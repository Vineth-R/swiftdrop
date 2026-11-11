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
	reason string
	err    error
}

func (c approvalChecker) Approval(context.Context, string) (string, string, error) {
	return c.status, c.reason, c.err
}

func TestService_MerchantLoginRefusalCarriesTheDecision(t *testing.T) {
	const support = "support@swiftdrop.example"
	tests := []struct {
		name       string
		active     bool
		checker    approvalChecker
		wantDetail bool
		wantStatus string
		wantReason string
	}{
		{"pending application", true, approvalChecker{status: "PENDING"}, true, "PENDING", ""},
		{"rejected with a reason", true, approvalChecker{status: "REJECTED", reason: "The registration number could not be verified."}, true, "REJECTED", "The registration number could not be verified."},
		{"rejected without a reason", true, approvalChecker{status: "REJECTED"}, true, "REJECTED", ""},
		{"suspended with a reason", true, approvalChecker{status: "SUSPENDED", reason: "Repeated delivery complaints."}, true, "SUSPENDED", "Repeated delivery complaints."},
		// A rejected owner's Identity account is inactive from the start.
		{"inactive and rejected", false, approvalChecker{status: "REJECTED", reason: "Duplicate business."}, true, "REJECTED", "Duplicate business."},
		// The lookup only adds detail, so an outage is still the plain refusal.
		{"inactive and the merchant service is down", false, approvalChecker{err: errors.New("down")}, false, "", ""},
		// An account switched off by an administrator is not a merchant decision.
		{"inactive but approved", false, approvalChecker{status: "APPROVED"}, false, "", ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			f := newFixture(t)
			f.service.config.SupportEmail = support
			account := f.register(t)
			user := f.store.users[account.ID]
			user.Role = "MERCHANT"
			user.IsActive = test.active
			f.service.merchantApproval = test.checker

			_, err := f.login("Str0ngPass!")

			if !errors.Is(err, ErrMerchantAccountInactive) {
				t.Fatalf("error = %v, want ErrMerchantAccountInactive", err)
			}
			var inactive *MerchantInactiveError
			if got := errors.As(err, &inactive); got != test.wantDetail {
				t.Fatalf("carries details = %v, want %v (error %#v)", got, test.wantDetail, err)
			}
			if !test.wantDetail {
				return
			}
			if inactive.Status != test.wantStatus || inactive.Reason != test.wantReason || inactive.SupportEmail != support {
				t.Fatalf("details = %+v, want status %q reason %q support %q", *inactive, test.wantStatus, test.wantReason, support)
			}
		})
	}
}

func TestService_MerchantLoginShowsNothingBeforeTheCorrectPassword(t *testing.T) {
	f := newFixture(t)
	account := f.register(t)
	user := f.store.users[account.ID]
	user.Role = "MERCHANT"
	user.IsActive = false
	f.service.merchantApproval = approvalChecker{status: "REJECTED", reason: "Private note for the owner."}

	_, err := f.login("Wr0ngPassword!")

	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("wrong password error = %v, want ErrInvalidCredentials", err)
	}
	var inactive *MerchantInactiveError
	if errors.As(err, &inactive) {
		t.Fatal("a wrong password was told the merchant's status and reason")
	}
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
