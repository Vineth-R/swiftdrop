package service

import (
	"context"
	"errors"
	"net/netip"
	"testing"
	"time"

	"github.com/Vineth-R/swiftdrop/apps/services/identity/internal/repository"
)

type approvalChecker struct {
	status string
	err    error
}

func (c approvalChecker) Approval(context.Context, string) (string, error) {
	return c.status, c.err
}

func TestService_RegisterSendsVerificationAndDoesNotIssueSession(t *testing.T) {
	f := newFixture(t)
	account, err := f.service.Register(context.Background(), validRegistration())
	if err != nil {
		t.Fatalf("Register returned error: %v", err)
	}
	if account.EmailVerified {
		t.Fatal("a new customer was marked verified")
	}
	sent := f.notes.all()
	if len(sent) != 1 || sent[0].kind != "verify-email" {
		t.Fatalf("emails = %v, want one verification email", kinds(sent))
	}
	if _, err := f.login("Str0ngPass!"); !errors.Is(err, ErrEmailVerificationRequired) {
		t.Fatalf("unverified login error = %v, want ErrEmailVerificationRequired", err)
	}
}

func TestService_VerifyEmailConsumesTokenOnce(t *testing.T) {
	f := newFixture(t)
	account := f.register(t)
	// register() verifies in the fixture for ordinary use; reset to test the
	// token path itself.
	f.store.users[account.ID].EmailVerified = false
	for hash := range f.store.verifications {
		record := f.store.verifications[hash]
		record.userID = account.ID
		f.store.verifications[hash] = record
	}
	// Put a known token in the in-memory store.
	if err := f.store.IssueEmailVerificationToken(context.Background(), repository.NewEmailVerificationToken{
		UserID: account.ID, TokenHash: hashVerificationToken("known-token"), ExpiresAt: f.now.Add(time.Hour),
	}, f.now); err != nil {
		t.Fatal(err)
	}
	// The fixture's register() marked the user verified; verify still consumes
	// the token and sets the flag.
	if err := f.service.VerifyEmail(context.Background(), "known-token"); err != nil {
		t.Fatalf("VerifyEmail returned error: %v", err)
	}
	if !f.store.users[account.ID].EmailVerified {
		t.Fatal("email was not marked verified")
	}
	if err := f.service.VerifyEmail(context.Background(), "known-token"); !errors.Is(err, ErrVerificationTokenInvalid) {
		t.Fatalf("reused token error = %v, want ErrVerificationTokenInvalid", err)
	}
}

func TestService_VerifyEmailRejectsExpiredAndUnknown(t *testing.T) {
	f := newFixture(t)
	account := f.register(t)
	if err := f.store.IssueEmailVerificationToken(context.Background(), repository.NewEmailVerificationToken{
		UserID: account.ID, TokenHash: hashVerificationToken("expired"), ExpiresAt: f.now.Add(-time.Minute),
	}, f.now); err != nil {
		t.Fatal(err)
	}
	for _, token := range []string{"expired", "unknown", ""} {
		if err := f.service.VerifyEmail(context.Background(), token); !errors.Is(err, ErrVerificationTokenInvalid) {
			t.Fatalf("VerifyEmail(%q) = %v, want ErrVerificationTokenInvalid", token, err)
		}
	}
}

func TestService_ResendVerificationIsQuietAndInvalidatesOldToken(t *testing.T) {
	f := newFixture(t)
	account := f.register(t)
	f.store.users[account.ID].EmailVerified = false
	if err := f.store.IssueEmailVerificationToken(context.Background(), repository.NewEmailVerificationToken{
		UserID: account.ID, TokenHash: hashVerificationToken("old"), ExpiresAt: f.now.Add(time.Hour),
	}, f.now); err != nil {
		t.Fatal(err)
	}
	before := len(f.notes.all())
	if err := f.service.ResendVerification(context.Background(), "nimal@example.com", netip.Addr{}); err != nil {
		t.Fatalf("ResendVerification returned error: %v", err)
	}
	if got := len(f.notes.all()); got != before+1 {
		t.Fatalf("emails = %d, want one more", got)
	}
	if err := f.service.VerifyEmail(context.Background(), "old"); !errors.Is(err, ErrVerificationTokenInvalid) {
		t.Fatalf("old token remained valid: %v", err)
	}
	// Unknown and already verified addresses are quiet.
	if err := f.service.ResendVerification(context.Background(), "nobody@example.com", netip.Addr{}); err != nil {
		t.Fatalf("unknown email returned error: %v", err)
	}
}

func TestService_MerchantLoginRequiresApproval(t *testing.T) {
	f := newFixture(t)
	account := f.register(t)
	user := f.store.users[account.ID]
	user.Role = "MERCHANT"
	user.IsActive = true
	user.EmailVerified = false

	// No checker: fail closed.
	if _, err := f.login("Str0ngPass!"); !errors.Is(err, ErrServiceUnavailable) {
		t.Fatalf("login without checker = %v, want ErrServiceUnavailable", err)
	}
	f.service.merchantApproval = approvalChecker{status: "PENDING"}
	if _, err := f.login("Str0ngPass!"); !errors.Is(err, ErrMerchantAccountInactive) {
		t.Fatalf("pending merchant login = %v, want ErrMerchantAccountInactive", err)
	}
	f.service.merchantApproval = approvalChecker{status: "APPROVED"}
	if _, err := f.login("Str0ngPass!"); err != nil {
		t.Fatalf("approved merchant login returned error: %v", err)
	}
	user.IsActive = false
	if _, err := f.login("Str0ngPass!"); !errors.Is(err, ErrMerchantAccountInactive) {
		t.Fatalf("inactive merchant login = %v, want ErrMerchantAccountInactive", err)
	}
}

func TestService_IssuesSessionVersionClaim(t *testing.T) {
	f := newFixture(t)
	account := f.register(t)
	f.store.users[account.ID].SessionVersion = 3
	session, err := f.login("Str0ngPass!")
	if err != nil {
		t.Fatalf("login returned error: %v", err)
	}
	claims, err := f.tokens.Verify(session.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	if claims.SessionVersion != 3 {
		t.Fatalf("session version = %d, want 3", claims.SessionVersion)
	}
}
