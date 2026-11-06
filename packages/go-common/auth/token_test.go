package auth

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const testSecret = "unit-test-secret-that-is-long-enough-0123456789"

var testNow = time.Date(2026, time.November, 2, 9, 30, 0, 0, time.UTC)

func newTestManager(t *testing.T, now time.Time) *TokenManager {
	t.Helper()
	manager, err := NewTokenManager(testSecret, 15*time.Minute, WithClock(func() time.Time { return now }))
	if err != nil {
		t.Fatalf("NewTokenManager returned error: %v", err)
	}
	return manager
}

func signForTest(t *testing.T, method jwt.SigningMethod, secret string, claims jwtClaims) string {
	t.Helper()
	signed, err := jwt.NewWithClaims(method, claims).SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("sign test token: %v", err)
	}
	return signed
}

// unsignedForTest builds an "alg: none" token, which verifiers must refuse.
func unsignedForTest(t *testing.T, claims jwtClaims) string {
	t.Helper()
	signed, err := jwt.NewWithClaims(jwt.SigningMethodNone, claims).SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatalf("build unsigned test token: %v", err)
	}
	return signed
}

func validClaims() jwtClaims {
	return jwtClaims{
		Role: string(RoleCustomer),
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    Issuer,
			Subject:   "3f1c0a52-8c0e-4e47-9d5e-1b6d7a9c2f10",
			ID:        "token-1",
			IssuedAt:  jwt.NewNumericDate(testNow),
			ExpiresAt: jwt.NewNumericDate(testNow.Add(15 * time.Minute)),
		},
	}
}

func TestCheckSecretStrength(t *testing.T) {
	tests := []struct {
		name    string
		secret  string
		wantErr bool
	}{
		{name: "long enough", secret: strings.Repeat("a", MinSecretBytes)},
		{name: "one byte short", secret: strings.Repeat("a", MinSecretBytes-1), wantErr: true},
		{name: "empty", secret: "", wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := CheckSecretStrength(test.secret)
			if (err != nil) != test.wantErr {
				t.Fatalf("CheckSecretStrength error = %v, wantErr %t", err, test.wantErr)
			}
		})
	}
}

func TestNewTokenManager(t *testing.T) {
	tests := []struct {
		name    string
		secret  string
		ttl     time.Duration
		wantErr bool
	}{
		{name: "valid", secret: testSecret, ttl: time.Minute},
		{name: "missing secret", secret: "", ttl: time.Minute, wantErr: true},
		{name: "zero lifetime", secret: testSecret, ttl: 0, wantErr: true},
		{name: "negative lifetime", secret: testSecret, ttl: -time.Minute, wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manager, err := NewTokenManager(test.secret, test.ttl)
			if (err != nil) != test.wantErr {
				t.Fatalf("NewTokenManager error = %v, wantErr %t", err, test.wantErr)
			}
			if err == nil && manager.TTL() != test.ttl {
				t.Fatalf("TTL() = %s, want %s", manager.TTL(), test.ttl)
			}
		})
	}
}

func TestTokenManager_Issue(t *testing.T) {
	manager := newTestManager(t, testNow)

	t.Run("issues claims with the configured lifetime", func(t *testing.T) {
		token, claims, err := manager.Issue("user-1", RoleCourier)
		if err != nil {
			t.Fatalf("Issue returned error: %v", err)
		}
		if token == "" || claims.TokenID == "" {
			t.Fatal("Issue returned an empty token or token id")
		}
		if claims.UserID != "user-1" || claims.Role != RoleCourier {
			t.Fatalf("claims = %+v, want user-1 COURIER", claims)
		}
		if got := claims.ExpiresAt.Sub(claims.IssuedAt); got != 15*time.Minute {
			t.Fatalf("lifetime = %s, want 15m", got)
		}
	})

	t.Run("token ids are unique", func(t *testing.T) {
		_, first, _ := manager.Issue("user-1", RoleCustomer)
		_, second, _ := manager.Issue("user-1", RoleCustomer)
		if first.TokenID == second.TokenID {
			t.Fatal("two tokens share the same token id")
		}
	})

	t.Run("rejects bad input", func(t *testing.T) {
		if _, _, err := manager.Issue("", RoleCustomer); err == nil {
			t.Fatal("Issue accepted an empty user id")
		}
		if _, _, err := manager.Issue("user-1", Role("ROOT")); err == nil {
			t.Fatal("Issue accepted an unknown role")
		}
	})
}

func TestTokenManager_Verify(t *testing.T) {
	manager := newTestManager(t, testNow)

	t.Run("round trip", func(t *testing.T) {
		token, issued, err := manager.Issue("user-1", RoleDispatcher)
		if err != nil {
			t.Fatalf("Issue returned error: %v", err)
		}

		got, err := manager.Verify(token)
		if err != nil {
			t.Fatalf("Verify returned error: %v", err)
		}
		if got != issued {
			t.Fatalf("Verify = %+v, want %+v", got, issued)
		}
	})

	badClaims := func(mutate func(*jwtClaims)) jwtClaims {
		claims := validClaims()
		mutate(&claims)
		return claims
	}

	tests := []struct {
		name  string
		token string
		want  error
	}{
		{
			name:  "valid token",
			token: signForTest(t, jwt.SigningMethodHS256, testSecret, validClaims()),
		},
		{
			name: "expired token",
			token: signForTest(t, jwt.SigningMethodHS256, testSecret, badClaims(func(c *jwtClaims) {
				c.ExpiresAt = jwt.NewNumericDate(testNow.Add(-time.Second))
			})),
			want: ErrTokenExpired,
		},
		{
			name:  "wrong secret",
			token: signForTest(t, jwt.SigningMethodHS256, "another-secret-another-secret-another-secret", validClaims()),
			want:  ErrTokenInvalid,
		},
		{
			name:  "other signing algorithm",
			token: signForTest(t, jwt.SigningMethodHS512, testSecret, validClaims()),
			want:  ErrTokenInvalid,
		},
		{
			name: "wrong issuer",
			token: signForTest(t, jwt.SigningMethodHS256, testSecret, badClaims(func(c *jwtClaims) {
				c.Issuer = "someone-else"
			})),
			want: ErrTokenInvalid,
		},
		{
			name: "missing expiry",
			token: signForTest(t, jwt.SigningMethodHS256, testSecret, badClaims(func(c *jwtClaims) {
				c.ExpiresAt = nil
			})),
			want: ErrTokenInvalid,
		},
		{
			name: "issued in the future",
			token: signForTest(t, jwt.SigningMethodHS256, testSecret, badClaims(func(c *jwtClaims) {
				c.IssuedAt = jwt.NewNumericDate(testNow.Add(time.Hour))
				c.ExpiresAt = jwt.NewNumericDate(testNow.Add(2 * time.Hour))
			})),
			want: ErrTokenInvalid,
		},
		{
			name: "unknown role",
			token: signForTest(t, jwt.SigningMethodHS256, testSecret, badClaims(func(c *jwtClaims) {
				c.Role = "ROOT"
			})),
			want: ErrTokenInvalid,
		},
		{
			name: "missing subject",
			token: signForTest(t, jwt.SigningMethodHS256, testSecret, badClaims(func(c *jwtClaims) {
				c.Subject = ""
			})),
			want: ErrTokenInvalid,
		},
		{
			name: "missing token id",
			token: signForTest(t, jwt.SigningMethodHS256, testSecret, badClaims(func(c *jwtClaims) {
				c.ID = ""
			})),
			want: ErrTokenInvalid,
		},
		{
			name:  "unsigned token",
			token: unsignedForTest(t, validClaims()),
			want:  ErrTokenInvalid,
		},
		{name: "garbage", token: "not-a-jwt", want: ErrTokenInvalid},
		{name: "empty", token: "", want: ErrTokenInvalid},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := manager.Verify(test.token)
			if !errors.Is(err, test.want) {
				t.Fatalf("Verify error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestTokenManager_Verify_expiresWithTheClock(t *testing.T) {
	issuer := newTestManager(t, testNow)
	token, _, err := issuer.Issue("user-1", RoleCustomer)
	if err != nil {
		t.Fatalf("Issue returned error: %v", err)
	}

	later := newTestManager(t, testNow.Add(16*time.Minute))
	if _, err := later.Verify(token); !errors.Is(err, ErrTokenExpired) {
		t.Fatalf("Verify after the lifetime error = %v, want %v", err, ErrTokenExpired)
	}
}
