package service

import (
	"context"
	"net/netip"
	"sync"
	"testing"
	"time"

	"github.com/Vineth-R/swiftdrop/apps/services/identity/internal/repository"
	"github.com/Vineth-R/swiftdrop/packages/go-common/auth"
)

// accountEmail is one request for an email, as the service made it.
type accountEmail struct {
	kind      string
	to        Recipient
	byReset   bool
	role      auth.Role
	lockedFor time.Duration
	isActive  bool
	at        time.Time
}

// recordingNotifier remembers what the service asked it to send.
type recordingNotifier struct {
	mu     sync.Mutex
	emails []accountEmail
}

func (r *recordingNotifier) add(e accountEmail) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.emails = append(r.emails, e)
}

func (r *recordingNotifier) all() []accountEmail {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]accountEmail(nil), r.emails...)
}

func (r *recordingNotifier) VerifyEmail(_ context.Context, to Recipient, link string, validFor time.Duration) {
	r.add(accountEmail{kind: "verify-email", to: to})
}

func (r *recordingNotifier) Welcome(_ context.Context, to Recipient) {
	r.add(accountEmail{kind: "welcome", to: to})
}

func (r *recordingNotifier) PasswordChanged(_ context.Context, to Recipient, byReset bool, at time.Time) {
	r.add(accountEmail{kind: "password-changed", to: to, byReset: byReset, at: at})
}

func (r *recordingNotifier) StaffAccountCreated(_ context.Context, to Recipient, role auth.Role) {
	r.add(accountEmail{kind: "staff-created", to: to, role: role})
}

func (r *recordingNotifier) AccountLocked(_ context.Context, to Recipient, lockedFor time.Duration) {
	r.add(accountEmail{kind: "locked", to: to, lockedFor: lockedFor})
}

func (r *recordingNotifier) AccountStatusChanged(_ context.Context, to Recipient, isActive bool) {
	r.add(accountEmail{kind: "status", to: to, isActive: isActive})
}

func kinds(emails []accountEmail) []string {
	out := make([]string, len(emails))
	for i, e := range emails {
		out[i] = e.kind
	}
	return out
}

func TestNotifier_sendsEmailVerificationAfterRegistration(t *testing.T) {
	f := newFixture(t)
	account := f.register(t)

	sent := f.notes.all()
	if len(sent) != 1 || sent[0].kind != "verify-email" {
		t.Fatalf("emails = %v, want one verification email", kinds(sent))
	}
	if sent[0].to != (Recipient{UserID: account.ID, Email: "nimal@example.com", FullName: "Nimal Perera"}) {
		t.Fatalf("recipient = %+v", sent[0].to)
	}
}

func TestNotifier_staysQuietWhenRegistrationFails(t *testing.T) {
	f := newFixture(t)
	f.register(t)

	if _, err := f.service.Register(context.Background(), validRegistration()); err == nil {
		t.Fatal("a duplicate registration succeeded")
	}
	bad := validRegistration()
	bad.Email, bad.Password = "other@example.com", "weak"
	if _, err := f.service.Register(context.Background(), bad); err == nil {
		t.Fatal("a weak password was accepted")
	}
	if got := len(f.notes.all()); got != 1 {
		t.Fatalf("%d emails, want only the first welcome", got)
	}
}

func TestNotifier_tellsThePersonWhenTheirAccountLocks(t *testing.T) {
	f := newFixture(t)
	account := f.register(t)
	locks := func() []accountEmail {
		var out []accountEmail
		for _, e := range f.notes.all() {
			if e.kind == "locked" {
				out = append(out, e)
			}
		}
		return out
	}

	for attempt := 1; attempt <= 4; attempt++ {
		_, _ = f.login("wrong-password")
		if len(locks()) != 0 {
			t.Fatalf("a lock notice after only %d failures", attempt)
		}
	}
	_, _ = f.login("wrong-password")
	notices := locks()
	if len(notices) != 1 || notices[0].to.UserID != account.ID || notices[0].lockedFor != 15*time.Minute {
		t.Fatalf("lock notices = %+v, want one for 15 minutes", notices)
	}

	// Attempts while locked do not repeat the email, even with the right password.
	_, _ = f.login("wrong-password")
	_, _ = f.login("Str0ngPass!")
	if got := len(locks()); got != 1 {
		t.Fatalf("%d lock notices, want exactly one", got)
	}
}

func TestNotifier_doesNotEmailAnUnknownAddressAfterFailedLogins(t *testing.T) {
	f := newFixture(t)
	for range 8 {
		_, _ = f.service.Login(context.Background(), LoginInput{Email: "nobody@example.com", Password: "x", IP: netip.MustParseAddr("203.0.113.1")})
	}
	if got := f.notes.all(); len(got) != 0 {
		t.Fatalf("emails = %v for an address with no account", kinds(got))
	}
}

func TestNotifier_confirmsAPasswordChangeFromTheProfile(t *testing.T) {
	f := newFixture(t)
	account := f.register(t)

	if err := f.service.ChangePassword(context.Background(), account.ID, ChangePasswordInput{CurrentPassword: "wrong", NewPassword: "N3wStrongPass!"}); err == nil {
		t.Fatal("a wrong current password was accepted")
	}
	if got := len(f.notes.all()); got != 1 {
		t.Fatalf("%d emails after a refused change, want only the welcome", got)
	}

	if err := f.service.ChangePassword(context.Background(), account.ID, ChangePasswordInput{CurrentPassword: "Str0ngPass!", NewPassword: "N3wStrongPass!"}); err != nil {
		t.Fatalf("ChangePassword returned error: %v", err)
	}
	sent := f.notes.all()
	last := sent[len(sent)-1]
	if last.kind != "password-changed" || last.byReset || last.to.Email != "nimal@example.com" || !last.at.Equal(f.now) {
		t.Fatalf("last email = %+v, want a password-changed notice for a profile change", last)
	}
}

func TestNotifier_confirmsAPasswordReset(t *testing.T) {
	f := newResetFixture(t)
	// The reset fixture's own store knows user-1; the notice looks the person up in the service's store.
	hash := "existing-hash"
	f.fixture.store.users["user-1"] = &repository.User{ID: "user-1", Email: "nimal@example.com", FullName: "Nimal Perera", PasswordHash: &hash, IsActive: true}

	token := f.forgot(t, "nimal@example.com")
	if err := f.reset.ResetPassword(context.Background(), ResetPasswordInput{Token: "not-a-token", NewPassword: "N3wStr0ngPass"}); err == nil {
		t.Fatal("a bad token was accepted")
	}
	if got := len(f.notes.all()); got != 0 {
		t.Fatalf("%d emails after a refused reset, want none", got)
	}

	if err := f.reset.ResetPassword(context.Background(), ResetPasswordInput{Token: token, NewPassword: "N3wStr0ngPass"}); err != nil {
		t.Fatalf("ResetPassword returned error: %v", err)
	}
	sent := f.notes.all()
	if len(sent) != 1 || sent[0].kind != "password-changed" || !sent[0].byReset || sent[0].to.UserID != "user-1" {
		t.Fatalf("emails = %+v, want one password-changed notice for a reset", sent)
	}
}

func TestNotifier_welcomesANewGoogleAccountButNotALinkedOne(t *testing.T) {
	f := newGoogleFixture(t)
	if _, err := f.callback(f.begin(t, "/dashboard")); err != nil {
		t.Fatalf("CompleteGoogleLogin returned error: %v", err)
	}
	if sent := f.notes.all(); len(sent) != 1 || sent[0].kind != "welcome" || sent[0].to.Email != "nimal@example.com" {
		t.Fatalf("emails = %+v, want one welcome for the new account", sent)
	}

	// The same person signing in again is not a new account.
	if _, err := f.callback(f.begin(t, "/dashboard")); err != nil {
		t.Fatalf("second CompleteGoogleLogin returned error: %v", err)
	}
	if got := len(f.notes.all()); got != 1 {
		t.Fatalf("%d emails after a second sign-in, want still one", got)
	}
}

func TestNotifier_tellsAStaffMemberTheirAccountExistsWithoutThePassword(t *testing.T) {
	admin, _, f := newAdminFixture(t)

	created, err := admin.CreateStaffUser(context.Background(), adminID, validStaff())
	if err != nil {
		t.Fatalf("CreateStaffUser returned error: %v", err)
	}
	sent := f.notes.all()
	if len(sent) != 1 || sent[0].kind != "staff-created" || sent[0].role != auth.RoleCourier || sent[0].to.UserID != created.ID {
		t.Fatalf("emails = %+v, want one staff-created notice for the courier", sent)
	}

	// A refused creation sends nothing.
	if _, err := admin.CreateStaffUser(context.Background(), adminID, CreateStaffInput{Email: "bad"}); err == nil {
		t.Fatal("an invalid staff account was created")
	}
	if got := len(f.notes.all()); got != 1 {
		t.Fatalf("%d emails after a refused creation, want still one", got)
	}
}

func TestNotifier_tellsAPersonTheirAccountWasTurnedOffOrOn(t *testing.T) {
	admin, _, f := newAdminFixture(t)
	f.store.users[targetID] = &repository.User{ID: targetID, Email: "nimal@example.com", FullName: "Nimal Perera", IsActive: true}

	if err := admin.SetUserActive(context.Background(), adminID, targetID, false); err != nil {
		t.Fatalf("deactivate returned error: %v", err)
	}
	if err := admin.SetUserActive(context.Background(), adminID, targetID, true); err != nil {
		t.Fatalf("activate returned error: %v", err)
	}
	sent := f.notes.all()
	if len(sent) != 2 || sent[0].kind != "status" || sent[0].isActive || sent[1].kind != "status" || !sent[1].isActive {
		t.Fatalf("emails = %+v, want deactivated then activated", sent)
	}
	if sent[0].to.Email != "nimal@example.com" {
		t.Fatalf("recipient = %+v", sent[0].to)
	}

	// Nothing is sent when the change is refused.
	before := len(f.notes.all())
	_ = admin.SetUserActive(context.Background(), adminID, adminID, false)
	if got := len(f.notes.all()); got != before {
		t.Fatal("a refused self-deactivation still sent an email")
	}
}

func TestWithNotifier_nilKeepsTheSilentDefault(t *testing.T) {
	f := newFixture(t)
	var n Notifier
	WithNotifier(n)(f.service)
	if f.service.notifier == nil {
		t.Fatal("a nil notifier replaced the default")
	}
	f.register(t) // must not panic
}
