package logging

import (
	"context"
	"log/slog"
	"regexp"
	"strings"
)

const redacted = "[REDACTED]"

var (
	phonePattern = regexp.MustCompile(`^\+?[0-9][0-9()\s-]{7,}$`)
	authPaths    = []string{
		"/auth/",
		"/api/v1/auth/",
		"/login",
		"/register",
		"/forgot-password",
		"/reset-password",
		"/refresh",
		"/token",
		"/users/me/password",
	}
	sensitiveKeys = []string{
		"password",
		"token",
		"secret",
		"apikey",
		"api_key",
		"authorization",
		"cookie",
		"credential",
	}
)

type RedactingHandler struct {
	next slog.Handler
}

func NewRedactingHandler(next slog.Handler) slog.Handler {
	return &RedactingHandler{next: next}
}

func (h *RedactingHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h *RedactingHandler) Handle(ctx context.Context, record slog.Record) error {
	clean := slog.NewRecord(record.Time, record.Level, record.Message, record.PC)
	record.Attrs(func(attr slog.Attr) bool {
		clean.AddAttrs(redactAttr(attr))
		return true
	})
	return h.next.Handle(ctx, clean)
}

func (h *RedactingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	clean := make([]slog.Attr, 0, len(attrs))
	for _, attr := range attrs {
		clean = append(clean, redactAttr(attr))
	}
	return &RedactingHandler{next: h.next.WithAttrs(clean)}
}

func (h *RedactingHandler) WithGroup(name string) slog.Handler {
	return &RedactingHandler{next: h.next.WithGroup(name)}
}

func IsAuthPath(path string) bool {
	lower := strings.ToLower(path)
	for _, prefix := range authPaths {
		if strings.Contains(lower, prefix) {
			return true
		}
	}
	return false
}

func BodyForLog(path string, body []byte) string {
	if len(body) == 0 {
		return ""
	}
	if IsAuthPath(path) {
		return redacted
	}
	// Request bodies can contain personal data; never log them in full.
	return redacted
}

func redactAttr(attr slog.Attr) slog.Attr {
	if attr.Value.Kind() == slog.KindGroup {
		group := attr.Value.Group()
		clean := make([]any, 0, len(group))
		for _, item := range group {
			clean = append(clean, redactAttr(item))
		}
		return slog.Group(attr.Key, clean...)
	}

	key := strings.ToLower(attr.Key)
	if containsAny(key, sensitiveKeys) || isAddressKey(key) || isPhoneKey(key) || isBodyKey(key) {
		return slog.String(attr.Key, redacted)
	}

	if attr.Value.Kind() == slog.KindString {
		return slog.String(attr.Key, redactValue(attr.Value.String()))
	}

	return attr
}

func redactValue(value string) string {
	if phonePattern.MatchString(strings.TrimSpace(value)) && digitCount(value) >= 9 {
		return redacted
	}
	return value
}

func digitCount(value string) int {
	count := 0
	for _, char := range value {
		if char >= '0' && char <= '9' {
			count++
		}
	}
	return count
}

func containsAny(value string, needles []string) bool {
	for _, needle := range needles {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}

func isAddressKey(key string) bool {
	return strings.Contains(key, "address")
}

func isPhoneKey(key string) bool {
	return strings.Contains(key, "phone") || strings.Contains(key, "msisdn")
}

func isBodyKey(key string) bool {
	switch key {
	case "body", "request_body", "requestbody", "payload":
		return true
	default:
		return false
	}
}
