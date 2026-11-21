package apikey

import (
	"strings"
	"testing"
)

func TestGenerateParseAndHash(t *testing.T) {
	key, err := Generate()
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	lookup, err := Parse(key.Full)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if lookup != key.LookupID {
		t.Fatalf("lookup = %q, want %q", lookup, key.LookupID)
	}
	if key.Hash != Hash(key.Full) {
		t.Fatal("generated hash does not match full key")
	}
	if !EqualHash(key.Hash, strings.ToUpper(key.Hash)) {
		t.Fatal("EqualHash rejected equivalent hexadecimal hash")
	}
}

func TestParseRejectsMalformedKeys(t *testing.T) {
	for _, value := range []string{"", "sdk_test_abc_secret", Prefix + "short_secret", Prefix + strings.Repeat("a", LookupLength) + "_not-hex"} {
		if _, err := Parse(value); err == nil {
			t.Errorf("Parse(%q) error = nil", value)
		}
	}
}
