package notificationlog

import (
	"strings"
	"unicode"
)

const maskFallback = "***"

// MaskRecipient returns a deterministic, privacy-preserving representation of
// a notification recipient. It never returns the original value.
func MaskRecipient(channel, recipient string) string {
	value := strings.TrimSpace(recipient)
	if value == "" || containsControl(value) {
		return maskFallback
	}

	switch strings.ToUpper(strings.TrimSpace(channel)) {
	case "EMAIL":
		return maskEmail(value)
	case "SMS":
		return maskPhone(value)
	default:
		return maskOpaque(value)
	}
}

func maskEmail(value string) string {
	at := strings.LastIndex(value, "@")
	if at <= 0 || at == len(value)-1 {
		return maskOpaque(value)
	}

	local := []rune(value[:at])
	domain := value[at+1:]
	if strings.ContainsAny(domain, " \t") {
		return maskOpaque(value)
	}
	if len(local) < 2 {
		return maskFallback + "@" + domain
	}
	return string(local[0]) + maskFallback + "@" + domain
}

func maskPhone(value string) string {
	digits := make([]rune, 0, len(value))
	for _, char := range value {
		if unicode.IsDigit(char) {
			digits = append(digits, char)
		}
	}
	if len(digits) < 4 {
		return maskFallback
	}

	prefix := ""
	if strings.HasPrefix(value, "+") {
		prefix = "+"
	}
	if len(digits) < 6 {
		return prefix + strings.Repeat("*", len(digits)-2) + string(digits[len(digits)-2:])
	}
	return prefix + string(digits[:2]) + strings.Repeat("*", len(digits)-4) + string(digits[len(digits)-2:])
}

func maskOpaque(value string) string {
	runes := []rune(value)
	if len(runes) < 2 {
		return maskFallback
	}
	return string(runes[0]) + maskFallback
}

func containsControl(value string) bool {
	for _, char := range value {
		if unicode.IsControl(char) {
			return true
		}
	}
	return false
}
