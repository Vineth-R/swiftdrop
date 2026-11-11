package businessreg

import (
	"errors"
	"strings"
	"testing"
)

func TestNormalize(t *testing.T) {
	for input, want := range map[string]string{
		"pv 123456":       "PV 123456",
		"  pv   123456  ": "PV 123456",
		"GV-2026/0457":    "GV-2026/0457",
		"":                "",
		" \t\n ":          "",
	} {
		if got := Normalize(input); got != want {
			t.Fatalf("Normalize(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestValid(t *testing.T) {
	for _, valid := range []string{"PV 123456", "GV-2026/0457", "HE.98", "123", "ÄÖÜ 123"} {
		if !Valid(valid) {
			t.Fatalf("Valid(%q) = false", valid)
		}
	}
	for _, invalid := range []string{"", "  ", "\t", "PV", "ABCDEF", "PV 123456 #", "PV_123456", strings.Repeat("1", 51)} {
		if Valid(invalid) {
			t.Fatalf("Valid(%q) = true", invalid)
		}
	}
}

func TestNormalizeValid(t *testing.T) {
	got, err := NormalizeValid("  pv   123456 ")
	if err != nil || got != "PV 123456" {
		t.Fatalf("NormalizeValid() = %q, %v", got, err)
	}
	for _, invalid := range []string{"", "   ", "\t", "PV", "ABCDEF", "PV_123456"} {
		if _, err := NormalizeValid(invalid); !errors.Is(err, ErrInvalid) {
			t.Fatalf("NormalizeValid(%q) error = %v", invalid, err)
		}
	}
}
