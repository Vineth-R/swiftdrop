package repository

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestTruncateUserAgent(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
		nil  bool
	}{
		{name: "empty becomes null", in: "", nil: true},
		{name: "short value is kept", in: "Mozilla/5.0", want: "Mozilla/5.0"},
		{name: "exact limit is kept", in: strings.Repeat("a", userAgentMaxLength), want: strings.Repeat("a", userAgentMaxLength)},
		{name: "long value is cut", in: strings.Repeat("a", userAgentMaxLength+40), want: strings.Repeat("a", userAgentMaxLength)},
		{name: "invalid bytes are dropped", in: "agent\xff\xfe-1", want: "agent-1"},
		{name: "only invalid bytes becomes null", in: "\xff\xfe", nil: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := truncateUserAgent(test.in)
			if test.nil {
				if got != nil {
					t.Fatalf("truncateUserAgent() = %q, want nil", *got)
				}
				return
			}
			if got == nil || *got != test.want {
				t.Fatalf("truncateUserAgent() = %v, want %q", got, test.want)
			}
		})
	}

	t.Run("never splits a multi-byte character", func(t *testing.T) {
		// Each é is two bytes, so the byte limit falls inside a character.
		got := truncateUserAgent(strings.Repeat("é", userAgentMaxLength))
		if got == nil {
			t.Fatal("truncateUserAgent() = nil, want a truncated value")
		}
		if !utf8.ValidString(*got) || len(*got) > userAgentMaxLength {
			t.Fatalf("truncateUserAgent() produced invalid or oversized output (%d bytes)", len(*got))
		}
	})
}
