//go:build access

package access

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Vineth-R/swiftdrop/packages/go-common/auth"
)

// refreshReuseGrace mirrors repository.RefreshReuseGrace in the identity
// service and API v1.1 section 2.3. Change them together.
const refreshReuseGrace = 10 * time.Second

// withoutMeta drops the per-request member so two answers can be compared.
func withoutMeta(t *testing.T, body []byte) string {
	t.Helper()
	var envelope map[string]any
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatalf("not JSON: %s", body)
	}
	delete(envelope, "meta")
	out, _ := json.Marshal(envelope)
	return string(out)
}

func TestSessions_loginErrorsRevealNothing(t *testing.T) {
	s := useStack(t)
	user := s.newCustomer(t)

	wrongPassword, _ := s.login(t, user.email, "Wr0ngPass!x")
	unknownEmail, _ := s.login(t, "nobody."+randomHex(4)+"@example.test", "Wr0ngPass!x")

	for name, r := range map[string]reply{"wrong password": wrongPassword, "unknown email": unknownEmail} {
		if r.status != http.StatusUnauthorized || r.code() != "INVALID_CREDENTIALS" {
			t.Fatalf("%s answered %d %s, want 401 INVALID_CREDENTIALS", name, r.status, r.code())
		}
	}
	if withoutMeta(t, wrongPassword.body) != withoutMeta(t, unknownEmail.body) {
		t.Fatalf("a wrong password and an unknown email can be told apart:\n%s\n%s", wrongPassword.body, unknownEmail.body)
	}
	for _, leak := range []string{user.email, user.password, "$2a$", "$2b$", "bcrypt", "SELECT ", "goroutine", ".go:"} {
		if bytes.Contains(wrongPassword.body, []byte(leak)) {
			t.Fatalf("the error leaks %q: %s", leak, wrongPassword.body)
		}
	}
	for _, cookie := range wrongPassword.header.Values("Set-Cookie") {
		if strings.Contains(cookie, "sd_access") || strings.Contains(cookie, "sd_refresh") {
			t.Fatalf("a failed login set a session cookie: %s", cookie)
		}
	}
}

func TestSessions_fiveFailuresLockTheAccount(t *testing.T) {
	s := useStack(t)
	user := s.newCustomer(t)

	for attempt := 1; attempt <= 4; attempt++ {
		if r, _ := s.login(t, user.email, "Wr0ngPass!x"); r.status != http.StatusUnauthorized {
			t.Fatalf("failed attempt %d answered %d, want 401", attempt, r.status)
		}
	}
	if r, _ := s.login(t, user.email, "Wr0ngPass!x"); r.status != http.StatusUnauthorized && r.status != http.StatusLocked {
		t.Fatalf("the fifth failure answered %d, want 401 or 423", r.status)
	}

	r, got := s.login(t, user.email, user.password)
	if r.status != http.StatusLocked || r.code() != "ACCOUNT_LOCKED" {
		t.Fatalf("the right password on a locked account answered %d %s, want 423 ACCOUNT_LOCKED", r.status, r.code())
	}
	if got.AccessToken != "" || got.RefreshToken != "" {
		t.Fatal("a locked account was given tokens")
	}
}

func TestSessions_refreshRotationAndReuse(t *testing.T) {
	s := useStack(t)
	user := s.newCustomer(t)

	_, first := s.login(t, user.email, user.password)
	if first.RefreshToken == "" {
		t.Fatal("login returned no refresh token")
	}

	r, second := s.refresh(t, first.RefreshToken)
	if r.status != http.StatusOK || second.RefreshToken == "" || second.RefreshToken == first.RefreshToken {
		t.Fatalf("refresh answered %d without a new token: %s", r.status, r.body)
	}

	// A repeat right after the rotation (two tabs, a lost response) is refused...
	if r, _ := s.refresh(t, first.RefreshToken); r.status != http.StatusUnauthorized || r.code() != "REFRESH_TOKEN_INVALID" {
		t.Fatalf("the used token answered %d %s, want 401 REFRESH_TOKEN_INVALID", r.status, r.code())
	}
	// ...but does not end the session.
	r, third := s.refresh(t, second.RefreshToken)
	if r.status != http.StatusOK {
		t.Fatalf("a quick retry of the old token ended the session: the newest token answered %d", r.status)
	}

	// Later, the same old token is evidence of a copy: it is refused and the
	// whole chain, including the newest token, is revoked.
	time.Sleep(refreshReuseGrace + time.Second)
	if r, _ := s.refresh(t, first.RefreshToken); r.status != http.StatusUnauthorized || r.code() != "REFRESH_TOKEN_INVALID" {
		t.Fatalf("the replayed token answered %d %s, want 401 REFRESH_TOKEN_INVALID", r.status, r.code())
	}
	if r, _ := s.refresh(t, third.RefreshToken); r.status != http.StatusUnauthorized {
		t.Fatalf("the newest token still works after a replay: answered %d", r.status)
	}
}

func TestSessions_logoutEndsTheRefreshToken(t *testing.T) {
	s := useStack(t)
	user := s.newCustomer(t)
	_, in := s.login(t, user.email, user.password)

	if r := s.mustDo(t, http.MethodPost, "/api/v1/auth/logout", "", nil); r.status != http.StatusUnauthorized {
		t.Fatalf("logout without a token answered %d, want 401", r.status)
	}

	body, _ := json.Marshal(map[string]string{"refreshToken": in.RefreshToken})
	headers := bearer(in.AccessToken)
	headers["X-Token-Mode"] = "body"
	if r := s.mustDo(t, http.MethodPost, "/api/v1/auth/logout", string(body), headers); r.status != http.StatusNoContent && r.status != http.StatusOK {
		t.Fatalf("logout answered %d: %s", r.status, r.body)
	}
	if r, _ := s.refresh(t, in.RefreshToken); r.status != http.StatusUnauthorized {
		t.Fatalf("a logged out refresh token answered %d, want 401", r.status)
	}
}

func TestSessions_passwordChangeEndsEverySession(t *testing.T) {
	s := useStack(t)
	user := s.newCustomer(t)
	_, laptop := s.login(t, user.email, user.password)
	_, phone := s.login(t, user.email, user.password)

	body, _ := json.Marshal(map[string]string{"currentPassword": user.password, "newPassword": "N3wStrongPass!"})
	r := s.mustDo(t, http.MethodPut, "/api/v1/users/me/password", string(body), bearer(laptop.AccessToken))
	if r.status != http.StatusNoContent {
		t.Fatalf("password change answered %d: %s", r.status, r.body)
	}

	for name, token := range map[string]string{"laptop": laptop.RefreshToken, "phone": phone.RefreshToken} {
		if r, _ := s.refresh(t, token); r.status != http.StatusUnauthorized {
			t.Fatalf("the %s session survived a password change: refresh answered %d", name, r.status)
		}
	}
	if r, _ := s.login(t, user.email, user.password); r.status != http.StatusUnauthorized {
		t.Fatalf("the old password still works: answered %d", r.status)
	}
	if r, got := s.login(t, user.email, "N3wStrongPass!"); r.status != http.StatusOK || got.AccessToken == "" {
		t.Fatalf("the new password does not work: answered %d", r.status)
	}
}

func TestSessions_expiredAndForgedAccessTokens(t *testing.T) {
	s := useStack(t)
	id := s.userIDs[auth.RoleCustomer]

	issue := func(secret string, issuedAt time.Time) string {
		t.Helper()
		manager, err := auth.NewTokenManager(secret, 15*time.Minute, auth.WithClock(func() time.Time { return issuedAt }))
		if err != nil {
			t.Fatal(err)
		}
		token, _, err := manager.Issue(id, auth.RoleCustomer)
		if err != nil {
			t.Fatal(err)
		}
		return token
	}
	segment := func(value string) string { return base64.RawURLEncoding.EncodeToString([]byte(value)) }

	for _, test := range []struct {
		name     string
		token    string
		wantCode string
	}{
		{"an expired token", issue(s.secret, time.Now().Add(-time.Hour)), "TOKEN_EXPIRED"},
		{"a token signed with another secret", issue("another-secret-that-is-long-enough-0123456789", time.Now()), "UNAUTHENTICATED"},
		{
			"an unsigned token that claims to be ADMIN",
			segment(`{"alg":"none","typ":"JWT"}`) + "." + segment(`{"sub":"`+id+`","role":"ADMIN","iss":"swiftdrop-identity","exp":`+
				strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10)+`}`) + ".",
			"UNAUTHENTICATED",
		},
		{"garbage", "not.a.token", "UNAUTHENTICATED"},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := s.mustDo(t, http.MethodGet, "/api/v1/users/me", "", bearer(test.token))
			if r.status != http.StatusUnauthorized || r.code() != test.wantCode {
				t.Fatalf("answered %d %s, want 401 %s", r.status, r.code(), test.wantCode)
			}
		})
	}

	t.Run("a valid token still works", func(t *testing.T) {
		if r := s.mustDo(t, http.MethodGet, "/api/v1/users/me", "", bearer(issue(s.secret, time.Now()))); r.status != http.StatusOK {
			t.Fatalf("answered %d: %s", r.status, r.body)
		}
	})
}
