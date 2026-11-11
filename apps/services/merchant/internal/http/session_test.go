package servicehttp

import (
	"testing"

	"github.com/Vineth-R/swiftdrop/apps/services/merchant/internal/identityclient"
	"github.com/Vineth-R/swiftdrop/packages/go-common/auth"
)

func TestSessionIsCurrentRejectsStaleVersion(t *testing.T) {
	claims := auth.Claims{UserID: "user-1", Role: auth.RoleMerchant, SessionVersion: 1}
	if !sessionIsCurrent(claims, identityclient.SessionState{Role: "MERCHANT", IsActive: true, SessionVersion: 1}) {
		t.Fatal("current session was rejected")
	}
	if sessionIsCurrent(claims, identityclient.SessionState{Role: "MERCHANT", IsActive: true, SessionVersion: 2}) {
		t.Fatal("a stale session version was accepted after reapproval")
	}
	if sessionIsCurrent(claims, identityclient.SessionState{Role: "MERCHANT", IsActive: false, SessionVersion: 1}) {
		t.Fatal("an inactive account session was accepted")
	}
	if sessionIsCurrent(claims, identityclient.SessionState{Role: "CUSTOMER", IsActive: true, SessionVersion: 1}) {
		t.Fatal("a role mismatch was accepted")
	}
}
