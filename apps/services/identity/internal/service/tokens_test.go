package service

import (
	"strings"
	"testing"
)

func TestNewRefreshToken(t *testing.T) {
	token, hash, err := newRefreshToken()
	if err != nil {
		t.Fatalf("newRefreshToken returned error: %v", err)
	}

	if !strings.HasPrefix(token, refreshTokenPrefix) {
		t.Fatalf("token %q lacks the %q prefix", token, refreshTokenPrefix)
	}
	if len(hash) != 64 || hash != hashRefreshToken(token) {
		t.Fatalf("hash %q is not the 64 character SHA-256 of the token", hash)
	}
	if strings.Contains(hash, token) {
		t.Fatal("the hash contains the raw token")
	}

	other, _, err := newRefreshToken()
	if err != nil {
		t.Fatalf("newRefreshToken returned error: %v", err)
	}
	if other == token {
		t.Fatal("two refresh tokens are identical")
	}
}

func TestPlausibleRefreshToken(t *testing.T) {
	tests := []struct {
		name  string
		token string
		want  bool
	}{
		{name: "generated shape", token: "rt_abcdefghijklmnopqrstuvwxyz0123456789ABCDEFG", want: true},
		{name: "missing prefix", token: "abcdefghijklmnopqrstuvwxyz", want: false},
		{name: "empty", token: "", want: false},
		{name: "at the length limit", token: "rt_" + strings.Repeat("a", maxRefreshTokenLength-3), want: true},
		{name: "over the length limit", token: "rt_" + strings.Repeat("a", maxRefreshTokenLength-2), want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := plausibleRefreshToken(test.token); got != test.want {
				t.Fatalf("plausibleRefreshToken() = %t, want %t", got, test.want)
			}
		})
	}
}
