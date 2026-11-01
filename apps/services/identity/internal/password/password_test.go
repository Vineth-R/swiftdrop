package password

import (
	"errors"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestValidate(t *testing.T) {
	tests := []struct {
		name      string
		password  string
		wantIssue string
	}{
		{name: "meets every rule", password: "Str0ngPass!"},
		{name: "exactly eight characters", password: "Abcdefg1"},
		{name: "exactly the byte limit", password: "A1" + strings.Repeat("a", MaxBytes-2)},
		{name: "unicode letters count", password: "Éclair99x"},
		{name: "too short", password: "Ab1defg", wantIssue: "must be at least 8 characters"},
		{name: "empty", password: "", wantIssue: "must be at least 8 characters"},
		{name: "no upper case", password: "lowercase1", wantIssue: "must contain"},
		{name: "no lower case", password: "UPPERCASE1", wantIssue: "must contain"},
		{name: "no digit", password: "NoDigitsHere", wantIssue: "must contain"},
		{name: "over the byte limit", password: "A1" + strings.Repeat("a", MaxBytes-1), wantIssue: "must be at most 72 bytes"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := Validate(test.password)
			if test.wantIssue == "" {
				if err != nil {
					t.Fatalf("Validate returned error: %v", err)
				}
				return
			}

			var policyErr *PolicyError
			if !errors.As(err, &policyErr) {
				t.Fatalf("Validate error = %v, want *PolicyError", err)
			}
			if !strings.Contains(policyErr.Issue, test.wantIssue) {
				t.Fatalf("Issue = %q, want it to contain %q", policyErr.Issue, test.wantIssue)
			}
			if strings.Contains(err.Error(), test.password) && test.password != "" {
				t.Fatalf("error text %q leaks the password", err.Error())
			}
		})
	}
}

func TestNewHasher(t *testing.T) {
	tests := []struct {
		name    string
		cost    int
		wantErr bool
	}{
		{name: "minimum cost", cost: bcrypt.MinCost},
		{name: "zero selects the default", cost: 0},
		{name: "negative", cost: -1, wantErr: true},
		{name: "too expensive", cost: bcrypt.MaxCost + 1, wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.cost == 0 && testing.Short() {
				t.Skip("default cost is slow")
			}
			_, err := NewHasher(test.cost)
			if (err != nil) != test.wantErr {
				t.Fatalf("NewHasher(%d) error = %v, wantErr %t", test.cost, err, test.wantErr)
			}
		})
	}
}

func TestHasher_Hash(t *testing.T) {
	hasher, err := NewHasher(bcrypt.MinCost)
	if err != nil {
		t.Fatalf("NewHasher returned error: %v", err)
	}

	t.Run("hashes are salted", func(t *testing.T) {
		first, err := hasher.Hash("Str0ngPass!")
		if err != nil {
			t.Fatalf("Hash returned error: %v", err)
		}
		second, _ := hasher.Hash("Str0ngPass!")
		if first == second {
			t.Fatal("two hashes of the same password are identical")
		}
		if strings.Contains(first, "Str0ngPass!") {
			t.Fatal("hash contains the plain password")
		}
	})

	t.Run("rejects input over the bcrypt limit", func(t *testing.T) {
		if _, err := hasher.Hash(strings.Repeat("a", MaxBytes+1)); err == nil {
			t.Fatal("Hash accepted an oversized password")
		}
	})
}

func TestHasher_Matches(t *testing.T) {
	hasher, err := NewHasher(bcrypt.MinCost)
	if err != nil {
		t.Fatalf("NewHasher returned error: %v", err)
	}
	hash, err := hasher.Hash("Str0ngPass!")
	if err != nil {
		t.Fatalf("Hash returned error: %v", err)
	}

	tests := []struct {
		name     string
		hash     string
		password string
		want     bool
	}{
		{name: "correct password", hash: hash, password: "Str0ngPass!", want: true},
		{name: "wrong password", hash: hash, password: "Str0ngPass?", want: false},
		{name: "empty password", hash: hash, password: "", want: false},
		{name: "malformed hash", hash: "not-a-bcrypt-hash", password: "Str0ngPass!", want: false},
		{name: "empty hash", hash: "", password: "Str0ngPass!", want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := hasher.Matches(test.hash, test.password); got != test.want {
				t.Fatalf("Matches() = %t, want %t", got, test.want)
			}
		})
	}
}

func TestHasher_Burn(t *testing.T) {
	hasher, err := NewHasher(bcrypt.MinCost)
	if err != nil {
		t.Fatalf("NewHasher returned error: %v", err)
	}
	// Burn has no result to assert; it must simply not panic on any input.
	hasher.Burn("")
	hasher.Burn(strings.Repeat("a", MaxBytes*2))
}
