package service

import (
	"context"
	"errors"
	"log/slog"
	"net/netip"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/Vineth-R/swiftdrop/apps/services/identity/internal/repository"
	"github.com/Vineth-R/swiftdrop/packages/go-common/auth"
	"github.com/Vineth-R/swiftdrop/packages/go-common/mailer"
)

type resetRecord struct {
	userID    string
	expiresAt time.Time
	used      bool
	ip        *netip.Addr
}

// resetMemoryStore is an in-memory ResetStore that applies the same rules as
// the repository: older tokens die when a new one is issued, a token works
// once, and a reset replaces the password.
type resetMemoryStore struct {
	users  map[string]*repository.User
	tokens map[string]*resetRecord // keyed by token hash
	failOn map[string]error

	revokedUsers []string
}

func newResetMemoryStore() *resetMemoryStore {
	return &resetMemoryStore{
		users:  map[string]*repository.User{},
		tokens: map[string]*resetRecord{},
		failOn: map[string]error{},
	}
}

func (s *resetMemoryStore) addUser(id, email string, active bool, hash *string) *repository.User {
	user := &repository.User{ID: id, Email: email, FullName: "Nimal <Perera>", Role: auth.RoleCustomer, IsActive: active, PasswordHash: hash}
	s.users[id] = user
	return user
}

func (s *resetMemoryStore) UserByEmail(_ context.Context, email string) (repository.User, error) {
	if err := s.failOn["UserByEmail"]; err != nil {
		return repository.User{}, err
	}
	for _, user := range s.users {
		if strings.EqualFold(user.Email, email) {
			return *user, nil
		}
	}
	return repository.User{}, repository.ErrNotFound
}

func (s *resetMemoryStore) IssuePasswordResetToken(_ context.Context, token repository.NewPasswordResetToken, _ time.Time) error {
	if err := s.failOn["IssuePasswordResetToken"]; err != nil {
		return err
	}
	for _, existing := range s.tokens {
		if existing.userID == token.UserID {
			existing.used = true
		}
	}
	s.tokens[token.TokenHash] = &resetRecord{userID: token.UserID, expiresAt: token.ExpiresAt, ip: token.RequestedIP}
	return nil
}

func (s *resetMemoryStore) PasswordResetTokenUsable(_ context.Context, tokenHash string, now time.Time) (bool, error) {
	if err := s.failOn["PasswordResetTokenUsable"]; err != nil {
		return false, err
	}
	record, ok := s.tokens[tokenHash]
	if !ok {
		return false, nil
	}
	return !record.used && now.Before(record.expiresAt) && s.users[record.userID].IsActive, nil
}

func (s *resetMemoryStore) ResetPasswordWithToken(_ context.Context, tokenHash, newHash string, now time.Time) (string, error) {
	if err := s.failOn["ResetPasswordWithToken"]; err != nil {
		return "", err
	}
	record, ok := s.tokens[tokenHash]
	if !ok || record.used || !now.Before(record.expiresAt) || !s.users[record.userID].IsActive {
		return "", repository.ErrResetTokenInvalid
	}
	record.used = true
	s.users[record.userID].PasswordHash = &newHash
	s.revokedUsers = append(s.revokedUsers, record.userID)
	return record.userID, nil
}

// testResetTokenTTL is deliberately not the production default, so the tests
// prove the lifetime comes from configuration.
const testResetTokenTTL = 45 * time.Minute

type resetFixture struct {
	*fixture
	reset  *PasswordReset
	store  *resetMemoryStore
	mailer *mailer.Mock
}

func newResetFixture(t *testing.T) *resetFixture {
	t.Helper()
	base := newFixture(t)
	store := newResetMemoryStore()
	hash := "existing-hash"
	store.addUser("user-1", "nimal@example.com", true, &hash)
	sender := &mailer.Mock{}

	reset, err := NewPasswordReset(base.service, store, sender, "https://swiftdrop.example/", testResetTokenTTL, WithSynchronousEmail())
	if err != nil {
		t.Fatalf("NewPasswordReset returned error: %v", err)
	}
	return &resetFixture{fixture: base, reset: reset, store: store, mailer: sender}
}

// forgot requests a link for email and returns the token found in the email.
func (f *resetFixture) forgot(t *testing.T, email string) string {
	t.Helper()
	before := len(f.mailer.Sent())
	if err := f.reset.ForgotPassword(context.Background(), ForgotPasswordInput{Email: email, IP: netip.MustParseAddr("203.0.113.7")}); err != nil {
		t.Fatalf("ForgotPassword returned error: %v", err)
	}
	sent := f.mailer.Sent()
	if len(sent) != before+1 {
		t.Fatalf("expected one new email, got %d", len(sent)-before)
	}
	return tokenFromEmail(t, sent[len(sent)-1])
}

func tokenFromEmail(t *testing.T, message mailer.Message) string {
	t.Helper()
	for _, field := range strings.Fields(message.Text) {
		if strings.HasPrefix(field, "https://") {
			parsed, err := url.Parse(field)
			if err != nil || parsed.Query().Get("token") == "" {
				t.Fatalf("reset link %q has no token", field)
			}
			return parsed.Query().Get("token")
		}
	}
	t.Fatal("email has no link")
	return ""
}

func TestNewPasswordReset(t *testing.T) {
	base := newFixture(t)
	store, sender := newResetMemoryStore(), &mailer.Mock{}

	if _, err := NewPasswordReset(nil, store, sender, "https://a.example", testResetTokenTTL); err == nil {
		t.Fatal("accepted a nil service")
	}
	if _, err := NewPasswordReset(base.service, nil, sender, "https://a.example", testResetTokenTTL); err == nil {
		t.Fatal("accepted a nil store")
	}
	if _, err := NewPasswordReset(base.service, store, nil, "https://a.example", testResetTokenTTL); err == nil {
		t.Fatal("accepted a nil mailer")
	}
	for _, bad := range []string{"", "/relative", "ftp://a.example", "not a url", "https://"} {
		if _, err := NewPasswordReset(base.service, store, sender, bad, testResetTokenTTL); err == nil {
			t.Fatalf("accepted frontend base URL %q", bad)
		}
	}
	if _, err := NewPasswordReset(base.service, store, sender, "https://a.example", 0); err == nil {
		t.Fatal("accepted a zero reset token lifetime")
	}
}

func TestPasswordReset_ForgotPassword_persistsUTCTimestamps(t *testing.T) {
	f := newResetFixture(t)
	local := time.Date(2026, time.November, 2, 12, 0, 0, 0, time.FixedZone("UTC+05:30", 5*3600+1800))
	f.service.now = func() time.Time { return local }

	token := f.forgot(t, "nimal@example.com")
	record := f.store.tokens[hashResetToken(token)]
	if record == nil {
		t.Fatal("reset token was not stored")
	}
	if record.expiresAt.Location() != time.UTC {
		t.Fatalf("expires_at location = %v, want UTC", record.expiresAt.Location())
	}
	if want := local.UTC().Add(testResetTokenTTL); !record.expiresAt.Equal(want) {
		t.Fatalf("expires_at = %v, want %v", record.expiresAt, want)
	}
}

func TestPasswordReset_ForgotPassword_sendsLinkAndStoresOnlyAHash(t *testing.T) {
	f := newResetFixture(t)

	token := f.forgot(t, "  Nimal@Example.com ")

	message := f.mailer.Sent()[0]
	if message.To != "nimal@example.com" || message.Subject != resetEmailSubject {
		t.Fatalf("message = %+v", message)
	}
	if !strings.Contains(message.Text, "https://swiftdrop.example/reset-password?token="+token) {
		t.Fatalf("link was not built from the configured frontend URL: %q", message.Text)
	}
	if strings.Contains(message.HTML, "<Perera>") || !strings.Contains(message.HTML, "&lt;Perera&gt;") {
		t.Fatalf("the user's name is not HTML-escaped: %q", message.HTML)
	}
	if len(token) < 43 {
		t.Fatalf("token has only %d characters", len(token))
	}

	if _, raw := f.store.tokens[token]; raw {
		t.Fatal("the raw token was stored")
	}
	record, ok := f.store.tokens[hashResetToken(token)]
	if !ok {
		t.Fatal("the token hash was not stored")
	}
	if want := f.now.Add(testResetTokenTTL); !record.expiresAt.Equal(want) {
		t.Fatalf("token expires at %v, want %v", record.expiresAt, want)
	}
	if record.ip == nil || record.ip.String() != "203.0.113.7" {
		t.Fatalf("requested IP = %v", record.ip)
	}
}

func TestPasswordReset_ForgotPassword_sameOutcomeForEveryEmail(t *testing.T) {
	googleOnly := func(f *resetFixture) { f.store.addUser("user-g", "google@example.com", true, nil) }
	disabled := func(f *resetFixture) {
		hash := "h"
		f.store.addUser("user-d", "disabled@example.com", false, &hash)
	}

	tests := []struct {
		name  string
		setup func(*resetFixture)
		email string
	}{
		{name: "unknown email", setup: func(*resetFixture) {}, email: "nobody@example.com"},
		{name: "google-only account", setup: googleOnly, email: "google@example.com"},
		{name: "disabled account", setup: disabled, email: "disabled@example.com"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			f := newResetFixture(t)
			test.setup(f)

			if err := f.reset.ForgotPassword(context.Background(), ForgotPasswordInput{Email: test.email}); err != nil {
				t.Fatalf("ForgotPassword returned error %v, want nil so the caller sees the same reply", err)
			}
			if len(f.mailer.Sent()) != 0 {
				t.Fatal("an email was sent")
			}
			if len(f.store.tokens) != 0 {
				t.Fatal("a token was created")
			}
		})
	}
}

func TestPasswordReset_ForgotPassword_validation(t *testing.T) {
	f := newResetFixture(t)

	for _, email := range []string{"", "   ", "not-an-email", "Evil <a@example.com>"} {
		err := f.reset.ForgotPassword(context.Background(), ForgotPasswordInput{Email: email})
		var validation *ValidationError
		if !errors.As(err, &validation) || validation.Issues[0].Field != "email" {
			t.Fatalf("ForgotPassword(%q) error = %v, want an email validation error", email, err)
		}
	}
}

func TestPasswordReset_ForgotPassword_mailFailureIsNotReported(t *testing.T) {
	f := newResetFixture(t)
	f.mailer.FailWith(errors.New("smtp down"))

	if err := f.reset.ForgotPassword(context.Background(), ForgotPasswordInput{Email: "nimal@example.com"}); err != nil {
		t.Fatalf("ForgotPassword returned %v; a mail outage must not change the reply", err)
	}
}

func TestPasswordReset_ForgotPassword_storeFailures(t *testing.T) {
	for _, op := range []string{"UserByEmail", "IssuePasswordResetToken"} {
		t.Run(op, func(t *testing.T) {
			f := newResetFixture(t)
			f.store.failOn[op] = errStore

			err := f.reset.ForgotPassword(context.Background(), ForgotPasswordInput{Email: "nimal@example.com"})
			if !errors.Is(err, errStore) {
				t.Fatalf("error = %v, want the store error", err)
			}
			if len(f.mailer.Sent()) != 0 {
				t.Fatal("an email was sent although the token was not stored")
			}
		})
	}
}

func TestPasswordReset_ForgotPassword_asyncDeliveryCompletesBeforeWaitReturns(t *testing.T) {
	base := newFixture(t)
	store := newResetMemoryStore()
	hash := "h"
	store.addUser("user-1", "nimal@example.com", true, &hash)
	sender := &mailer.Mock{}
	reset, err := NewPasswordReset(base.service, store, sender, "https://swiftdrop.example", testResetTokenTTL)
	if err != nil {
		t.Fatalf("NewPasswordReset returned error: %v", err)
	}

	if err := reset.ForgotPassword(context.Background(), ForgotPasswordInput{Email: "nimal@example.com"}); err != nil {
		t.Fatalf("ForgotPassword returned error: %v", err)
	}
	reset.Wait()
	if len(sender.Sent()) != 1 {
		t.Fatalf("%d emails delivered after Wait, want 1", len(sender.Sent()))
	}
}

func TestPasswordReset_newestLinkOnly(t *testing.T) {
	f := newResetFixture(t)

	first := f.forgot(t, "nimal@example.com")
	second := f.forgot(t, "nimal@example.com")
	if first == second {
		t.Fatal("two requests produced the same token")
	}

	err := f.reset.ResetPassword(context.Background(), ResetPasswordInput{Token: first, NewPassword: "N3wStr0ngPass"})
	if !errors.Is(err, ErrResetTokenInvalid) {
		t.Fatalf("old link error = %v, want ErrResetTokenInvalid", err)
	}
	if err := f.reset.ResetPassword(context.Background(), ResetPasswordInput{Token: second, NewPassword: "N3wStr0ngPass"}); err != nil {
		t.Fatalf("newest link returned error: %v", err)
	}
}

func TestPasswordReset_ResetPassword_worksOnce(t *testing.T) {
	f := newResetFixture(t)
	token := f.forgot(t, "nimal@example.com")

	if err := f.reset.ResetPassword(context.Background(), ResetPasswordInput{Token: token, NewPassword: "N3wStr0ngPass"}); err != nil {
		t.Fatalf("ResetPassword returned error: %v", err)
	}

	stored := f.store.users["user-1"].PasswordHash
	if stored == nil || *stored == "existing-hash" || *stored == "N3wStr0ngPass" || !f.hasher.Matches(*stored, "N3wStr0ngPass") {
		t.Fatal("the new password was not stored as a hash")
	}
	if len(f.store.revokedUsers) != 1 || f.store.revokedUsers[0] != "user-1" {
		t.Fatal("the user's sessions were not revoked")
	}

	err := f.reset.ResetPassword(context.Background(), ResetPasswordInput{Token: token, NewPassword: "An0therStrongPass"})
	if !errors.Is(err, ErrResetTokenInvalid) {
		t.Fatalf("second use error = %v, want ErrResetTokenInvalid", err)
	}
}

func TestPasswordReset_ResetPassword_expired(t *testing.T) {
	f := newResetFixture(t)
	token := f.forgot(t, "nimal@example.com")

	f.advance(testResetTokenTTL)

	err := f.reset.ResetPassword(context.Background(), ResetPasswordInput{Token: token, NewPassword: "N3wStr0ngPass"})
	if !errors.Is(err, ErrResetTokenInvalid) {
		t.Fatalf("error = %v, want ErrResetTokenInvalid", err)
	}
	if *f.store.users["user-1"].PasswordHash != "existing-hash" {
		t.Fatal("an expired link changed the password")
	}
}

func TestPasswordReset_ResetPassword_stillValidJustBeforeExpiry(t *testing.T) {
	f := newResetFixture(t)
	token := f.forgot(t, "nimal@example.com")

	f.advance(testResetTokenTTL - time.Second)

	if err := f.reset.ResetPassword(context.Background(), ResetPasswordInput{Token: token, NewPassword: "N3wStr0ngPass"}); err != nil {
		t.Fatalf("ResetPassword returned error: %v", err)
	}
}

func TestPasswordReset_ResetPassword_unknownAndOversizedTokens(t *testing.T) {
	f := newResetFixture(t)

	for _, token := range []string{"never-issued", "", "   ", strings.Repeat("a", maxResetTokenLength+1)} {
		err := f.reset.ResetPassword(context.Background(), ResetPasswordInput{Token: token, NewPassword: "N3wStr0ngPass"})
		if !errors.Is(err, ErrResetTokenInvalid) {
			t.Fatalf("token %.10q error = %v, want ErrResetTokenInvalid", token, err)
		}
	}
}

func TestPasswordReset_ResetPassword_validation(t *testing.T) {
	f := newResetFixture(t)
	token := f.forgot(t, "nimal@example.com")

	for name, weak := range map[string]string{
		"too short":        "short",
		"no digit":         "NoDigitsHere",
		"too long":         strings.Repeat("aA1", 30),
		"missing password": "",
	} {
		t.Run(name, func(t *testing.T) {
			err := f.reset.ResetPassword(context.Background(), ResetPasswordInput{Token: token, NewPassword: weak})

			var validation *ValidationError
			if !errors.As(err, &validation) || len(validation.Issues) != 1 || validation.Issues[0].Field != "newPassword" {
				t.Fatalf("error = %v, want a newPassword validation error", err)
			}
		})
	}

	// A rejected password must not spend the token.
	if err := f.reset.ResetPassword(context.Background(), ResetPasswordInput{Token: token, NewPassword: "N3wStr0ngPass"}); err != nil {
		t.Fatalf("token was spent by a failed validation: %v", err)
	}
}

// API v1.1 section 2.17 checks the token first and the password second, so a
// request with both wrong is a 400 (invalid token), never a 422.
func TestPasswordReset_ResetPassword_tokenIsCheckedBeforeThePassword(t *testing.T) {
	f := newResetFixture(t)
	used := f.forgot(t, "nimal@example.com")
	if err := f.reset.ResetPassword(context.Background(), ResetPasswordInput{Token: used, NewPassword: "N3wStr0ngPass"}); err != nil {
		t.Fatalf("setup reset returned error: %v", err)
	}
	expired := f.forgot(t, "nimal@example.com")
	f.advance(testResetTokenTTL)

	for name, token := range map[string]string{"unknown": "never-issued", "missing": "", "already used": used, "expired": expired} {
		t.Run(name, func(t *testing.T) {
			err := f.reset.ResetPassword(context.Background(), ResetPasswordInput{Token: token, NewPassword: "weak"})
			if !errors.Is(err, ErrResetTokenInvalid) {
				t.Fatalf("error = %v, want ErrResetTokenInvalid before any password check", err)
			}
		})
	}
}

func TestPasswordReset_ResetPassword_neverHashesForABadToken(t *testing.T) {
	// A bad token must be refused before the expensive bcrypt hash is computed,
	// so garbage requests cannot be used to burn CPU.
	f := newResetFixture(t)
	counter := &countingHasher{Hasher: f.service.hasher}
	f.service.hasher = counter

	_ = f.reset.ResetPassword(context.Background(), ResetPasswordInput{Token: "never-issued", NewPassword: "N3wStr0ngPass"})

	if counter.hashes != 0 {
		t.Fatal("the password was hashed although the token was invalid")
	}
}

func TestPasswordReset_ResetPassword_checkFailureIsNotATokenError(t *testing.T) {
	f := newResetFixture(t)
	token := f.forgot(t, "nimal@example.com")
	f.store.failOn["PasswordResetTokenUsable"] = errStore

	err := f.reset.ResetPassword(context.Background(), ResetPasswordInput{Token: token, NewPassword: "N3wStr0ngPass"})
	if !errors.Is(err, errStore) || errors.Is(err, ErrResetTokenInvalid) {
		t.Fatalf("error = %v, want the store error", err)
	}
}

func TestPasswordReset_ResetPassword_disabledAccount(t *testing.T) {
	f := newResetFixture(t)
	token := f.forgot(t, "nimal@example.com")
	f.store.users["user-1"].IsActive = false

	err := f.reset.ResetPassword(context.Background(), ResetPasswordInput{Token: token, NewPassword: "N3wStr0ngPass"})
	if !errors.Is(err, ErrResetTokenInvalid) {
		t.Fatalf("error = %v, want ErrResetTokenInvalid", err)
	}
}

func TestPasswordReset_ResetPassword_storeFailureIsNotATokenError(t *testing.T) {
	f := newResetFixture(t)
	token := f.forgot(t, "nimal@example.com")
	f.store.failOn["ResetPasswordWithToken"] = errStore

	err := f.reset.ResetPassword(context.Background(), ResetPasswordInput{Token: token, NewPassword: "N3wStr0ngPass"})
	if !errors.Is(err, errStore) || errors.Is(err, ErrResetTokenInvalid) {
		t.Fatalf("error = %v, want the store error", err)
	}
}

func TestPasswordReset_logsNeverContainTokensOrEmails(t *testing.T) {
	f := newResetFixture(t)
	f.mailer.FailWith(errors.New("smtp down"))

	// The fixture's logger discards output; swap in one we can read.
	var logs strings.Builder
	f.service.logger = newCapturingLogger(&logs)

	f.reset.ForgotPassword(context.Background(), ForgotPasswordInput{Email: "nimal@example.com"})

	text := logs.String()
	if !strings.Contains(text, "password reset email was not sent") || !strings.Contains(text, "user-1") {
		t.Fatalf("failure was not logged with the user ID: %q", text)
	}
	if strings.Contains(text, "nimal@example.com") || strings.Contains(text, "token=") {
		t.Fatalf("log leaks an email address or reset link: %q", text)
	}
}

func newCapturingLogger(out *strings.Builder) *slog.Logger {
	return slog.New(slog.NewTextHandler(out, nil))
}

// countingHasher counts calls to Hash so a test can prove none happened.
type countingHasher struct {
	Hasher
	hashes int
}

func (h *countingHasher) Hash(plain string) (string, error) {
	h.hashes++
	return h.Hasher.Hash(plain)
}
