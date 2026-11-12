package logging

import (
	"context"
	"encoding"
	"encoding/json"
	"log/slog"
	"reflect"
	"regexp"
	"strings"
	"time"
)

const (
	redacted = "[REDACTED]"

	// maxRedactDepth bounds recursive inspection. It is a final guard against
	// cyclic or pathologically deep values that the visited-set check does not
	// catch.
	maxRedactDepth = 32
)

type visit struct {
	typ reflect.Type
	ptr uintptr
}

var (
	logValuerType     = reflect.TypeOf((*slog.LogValuer)(nil)).Elem()
	jsonMarshalerType = reflect.TypeOf((*json.Marshaler)(nil)).Elem()
	textMarshalerType = reflect.TypeOf((*encoding.TextMarshaler)(nil)).Elem()

	phonePattern = regexp.MustCompile(`^\+?[0-9][0-9()\s-]{7,}$`)
	emailPattern = regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`)
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
		"api-key",
		"authorization",
		"cookie",
		"credential",
		"dsn",
		"database_url",
		"rabbitmq_url",
		"amqp_url",
		"connection_string",
		"connectionstring",
		"smtp_user",
		"email",
		"recipient",
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
	if isSensitiveKey(attr.Key) {
		return slog.String(attr.Key, redacted)
	}

	switch attr.Value.Kind() {
	case slog.KindGroup:
		group := attr.Value.Group()
		clean := make([]any, 0, len(group))
		for _, item := range group {
			clean = append(clean, redactAttr(item))
		}
		return slog.Group(attr.Key, clean...)
	case slog.KindString:
		return slog.String(attr.Key, redactValue(attr.Value.String()))
	case slog.KindAny:
		return slog.Any(attr.Key, redactAny(attr.Value.Any(), 0, make(map[visit]struct{})))
	case slog.KindLogValuer:
		// Resolving an arbitrary LogValuer can call caller code and may expose a
		// sensitive value. Redact it instead of invoking it.
		return slog.String(attr.Key, redacted)
	default:
		return attr
	}
}

// redactAny recursively copies supported JSON-like values and redacts
// sensitive map keys and phone-like strings. It never mutates the input.
//
// Supported values are string-keyed maps, slices, arrays, groups (handled by
// redactAttr), primitives and time.Time. Arbitrary structs, errors, []byte,
// LogValuer values and maps with non-string keys are replaced with [REDACTED].
func redactAny(value any, depth int, seen map[visit]struct{}) any {
	if depth > maxRedactDepth {
		return redacted
	}
	if value == nil {
		return nil
	}
	if pointer, ok := value.(*time.Time); ok {
		if pointer == nil {
			return nil
		}
		return *pointer
	}
	if _, ok := value.(time.Time); ok {
		return value
	}
	if implementsCustomFormatting(value) {
		return redacted
	}
	if _, ok := value.(error); ok {
		// Do not call Error() or String() on arbitrary errors: their text may
		// contain secrets or infrastructure details.
		return redacted
	}

	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Interface:
		if reflected.IsNil() || !reflected.Elem().CanInterface() {
			return nil
		}
		return redactAny(reflected.Elem().Interface(), depth+1, seen)
	case reflect.Ptr:
		if reflected.IsNil() {
			return nil
		}
		key := visit{typ: reflected.Type(), ptr: reflected.Pointer()}
		if _, ok := seen[key]; ok {
			return redacted
		}
		seen[key] = struct{}{}
		defer delete(seen, key)
		if !reflected.Elem().CanInterface() {
			return redacted
		}
		return redactAny(reflected.Elem().Interface(), depth+1, seen)
	case reflect.Map:
		if reflected.IsNil() {
			return nil
		}
		if reflected.Type().Key().Kind() != reflect.String {
			return redacted
		}
		key := visit{typ: reflected.Type(), ptr: reflected.Pointer()}
		if _, ok := seen[key]; ok {
			return redacted
		}
		seen[key] = struct{}{}
		defer delete(seen, key)

		clean := make(map[string]any, reflected.Len())
		iterator := reflected.MapRange()
		for iterator.Next() {
			name := iterator.Key().String()
			if isSensitiveKey(name) {
				clean[name] = redacted
				continue
			}
			if !iterator.Value().CanInterface() {
				clean[name] = redacted
				continue
			}
			clean[name] = redactAny(iterator.Value().Interface(), depth+1, seen)
		}
		return clean
	case reflect.Slice, reflect.Array:
		if reflected.Kind() == reflect.Slice && reflected.IsNil() {
			return nil
		}
		if reflected.Type().Elem().Kind() == reflect.Uint8 {
			// Byte slices are opaque; do not try to inspect or format them.
			return redacted
		}
		if reflected.Kind() == reflect.Slice {
			key := visit{typ: reflected.Type(), ptr: reflected.Pointer()}
			if _, ok := seen[key]; ok {
				return redacted
			}
			seen[key] = struct{}{}
			defer delete(seen, key)
		}

		clean := make([]any, reflected.Len())
		for index := 0; index < reflected.Len(); index++ {
			item := reflected.Index(index)
			if !item.CanInterface() {
				clean[index] = redacted
				continue
			}
			clean[index] = redactAny(item.Interface(), depth+1, seen)
		}
		return clean
	case reflect.String:
		return redactValue(reflected.String())
	case reflect.Bool,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
		reflect.Float32, reflect.Float64, reflect.Complex64, reflect.Complex128:
		return value
	case reflect.Struct:
		// time.Time is handled above. Other structs can expose sensitive fields
		// through JSON marshalling or Stringer, so do not pass them through.
		return redacted
	default:
		return redacted
	}
}

func implementsCustomFormatting(value any) bool {
	if _, ok := value.(slog.LogValuer); ok {
		return true
	}
	if _, ok := value.(json.Marshaler); ok {
		return true
	}
	if _, ok := value.(encoding.TextMarshaler); ok {
		return true
	}

	// A value with a pointer-receiver method does not implement the interface
	// itself, but a downstream formatter may take its address. Check the pointer
	// type without dereferencing the value.
	typ := reflect.TypeOf(value)
	if typ == nil || typ.Kind() == reflect.Ptr {
		return false
	}
	pointer := reflect.PointerTo(typ)
	return pointer.Implements(logValuerType) ||
		pointer.Implements(jsonMarshalerType) ||
		pointer.Implements(textMarshalerType)
}

func isSensitiveKey(key string) bool {
	key = strings.ToLower(key)
	return containsAny(key, sensitiveKeys) || isAddressKey(key) || isPhoneKey(key) || isBodyKey(key)
}

func redactValue(value string) string {
	trimmed := strings.TrimSpace(value)
	if phonePattern.MatchString(trimmed) && digitCount(value) >= 9 {
		return redacted
	}
	lower := strings.ToLower(trimmed)
	for _, word := range []string{"password", "token", "secret", "apikey", "api_key", "authorization"} {
		if strings.Contains(lower, word) {
			return redacted
		}
	}
	if emailPattern.MatchString(value) {
		return emailPattern.ReplaceAllString(value, redacted)
	}
	return value
}

// RedactText applies the same string-level rules to free text, such as a panic
// value. It is exported for callers that cannot pass a struct or an error value
// to the redacting slog handler.
func RedactText(value string) string {
	return redactValue(value)
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
