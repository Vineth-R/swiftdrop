package notificationlog

import (
	"strings"
	"testing"
)

func TestMaskRecipient(t *testing.T) {
	tests := []struct {
		name      string
		channel   string
		recipient string
		want      string
	}{
		{name: "email", channel: "EMAIL", recipient: "john.doe@example.com", want: "j***@example.com"},
		{name: "email is case insensitive", channel: "email", recipient: "john@example.com", want: "j***@example.com"},
		{name: "one character local part", channel: "EMAIL", recipient: "a@example.com", want: "***@example.com"},
		{name: "unicode local part", channel: "EMAIL", recipient: "élamide@example.com", want: "é***@example.com"},
		{name: "malformed email", channel: "EMAIL", recipient: "not-an-email", want: "n***"},
		{name: "missing local part", channel: "EMAIL", recipient: "@example.com", want: "@***"},
		{name: "missing domain", channel: "EMAIL", recipient: "john@", want: "j***"},
		{name: "empty", channel: "EMAIL", recipient: "  ", want: "***"},
		{name: "control character", channel: "EMAIL", recipient: "john\n@example.com", want: "***"},
		{name: "sms with country code", channel: "SMS", recipient: "+94771234567", want: "+94*******67"},
		{name: "sms with separators", channel: "SMS", recipient: "+94 77 123 4567", want: "+94*******67"},
		{name: "sms local format", channel: "SMS", recipient: "0771234567", want: "07******67"},
		{name: "short sms", channel: "SMS", recipient: "1234", want: "**34"},
		{name: "too short sms", channel: "SMS", recipient: "123", want: "***"},
		{name: "non-numeric sms", channel: "SMS", recipient: "+abc", want: "***"},
		{name: "unexpected channel", channel: "PUSH", recipient: "john@example.com", want: "j***"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := MaskRecipient(test.channel, test.recipient)
			if got != test.want {
				t.Fatalf("MaskRecipient(%q, %q) = %q, want %q", test.channel, test.recipient, got, test.want)
			}
			if recipient := strings.TrimSpace(test.recipient); recipient != "" && strings.Contains(got, recipient) {
				t.Fatalf("masked value %q contains the full recipient", got)
			}
			if again := MaskRecipient(test.channel, test.recipient); again != got {
				t.Fatalf("masking is not deterministic: %q then %q", got, again)
			}
		})
	}
}
