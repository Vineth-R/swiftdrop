package logging

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

var customFormatterCalls atomic.Int32

type secretValuer struct{}

func (secretValuer) LogValue() slog.Value { return slog.StringValue("valuer-secret") }

type secretStringer struct{}

func (secretStringer) String() string { return "stringer-secret" }

type secretStringError string

func (e secretStringError) Error() string { return string(e) }

type nestedLogValuer string

func (v nestedLogValuer) LogValue() slog.Value {
	customFormatterCalls.Add(1)
	return slog.StringValue("method-log-valuer-" + string(v))
}

type nestedPointerLogValuer string

func (v *nestedPointerLogValuer) LogValue() slog.Value {
	customFormatterCalls.Add(1)
	return slog.StringValue("method-pointer-log-valuer-" + string(*v))
}

type nestedJSONMarshaler string

func (v nestedJSONMarshaler) MarshalJSON() ([]byte, error) {
	customFormatterCalls.Add(1)
	return []byte(`"method-json-` + string(v) + `"`), nil
}

type nestedPointerJSONMarshaler string

func (v *nestedPointerJSONMarshaler) MarshalJSON() ([]byte, error) {
	customFormatterCalls.Add(1)
	return []byte(`"method-pointer-json-` + string(*v) + `"`), nil
}

type nestedTextMarshaler string

func (v nestedTextMarshaler) MarshalText() ([]byte, error) {
	customFormatterCalls.Add(1)
	return []byte("method-text-" + string(v)), nil
}

type nestedPointerTextMarshaler string

func (v *nestedPointerTextMarshaler) MarshalText() ([]byte, error) {
	customFormatterCalls.Add(1)
	return []byte("method-pointer-text-" + string(*v)), nil
}

func pointerTo[T any](value T) *T {
	return &value
}

func TestRedactingHandlerMasksCredentialsAndPII(t *testing.T) {
	var buffer bytes.Buffer
	logger := slog.New(NewRedactingHandler(slog.NewJSONHandler(&buffer, nil)))

	logger.Info("login attempt",
		"password", "Str0ngPass!",
		"accessToken", "eyJhbGciOi-secret",
		"authorization", "Bearer abc123",
		"cookie", "sd_refresh=rt_9fA",
		"phone", "+94771234567",
		"pickup_address", "12 Temple Road, Nugegoda",
		"body", `{"password":"Str0ngPass!","refreshToken":"rt_9fA"}`,
		"tracking_number", "SDK7M2Q9XP4",
	)

	output := buffer.String()
	for _, secret := range []string{"Str0ngPass!", "eyJhbGciOi-secret", "Bearer abc123", "sd_refresh=rt_9fA", "+94771234567", "12 Temple Road"} {
		if strings.Contains(output, secret) {
			t.Fatalf("log output leaked %q: %s", secret, output)
		}
	}
	if !strings.Contains(output, "SDK7M2Q9XP4") {
		t.Fatalf("non-sensitive attribute was removed: %s", output)
	}
	if !strings.Contains(output, redacted) {
		t.Fatalf("expected redaction marker in %s", output)
	}
}

func TestRedactingHandlerMasksGroupsAndAttachedAttrs(t *testing.T) {
	var buffer bytes.Buffer
	base := slog.New(NewRedactingHandler(slog.NewJSONHandler(&buffer, nil)))
	logger := base.With("api_key", "sdk_live_secret").WithGroup("request")

	logger.Info("handled", "token", "rt_secret", "status", "ok")

	output := buffer.String()
	if strings.Contains(output, "sdk_live_secret") || strings.Contains(output, "rt_secret") {
		t.Fatalf("log output leaked a secret: %s", output)
	}
	if !strings.Contains(output, "status") {
		t.Fatalf("non-sensitive attribute was removed: %s", output)
	}
}

func TestRedactingHandlerMasksRefreshTokensAndAPIKeys(t *testing.T) {
	var buffer bytes.Buffer
	logger := slog.New(NewRedactingHandler(slog.NewJSONHandler(&buffer, nil)))

	logger.Info("session and credential fields",
		"refresh_token", "rt_refresh_secret",
		"refreshToken", "rt_camel_secret",
		"apiKey", "sdk_live_camel_secret",
		"api_key", "sdk_live_snake_secret",
		"phone_number", "+94771234567",
		"address", "12 Temple Road, Nugegoda",
		"tracking_number", "SDK7M2Q9XP4",
	)

	output := buffer.String()
	for _, secret := range []string{
		"rt_refresh_secret",
		"rt_camel_secret",
		"sdk_live_camel_secret",
		"sdk_live_snake_secret",
		"+94771234567",
		"12 Temple Road",
	} {
		if strings.Contains(output, secret) {
			t.Fatalf("log output leaked %q: %s", secret, output)
		}
	}
	if !strings.Contains(output, "SDK7M2Q9XP4") {
		t.Fatalf("non-sensitive tracking number was removed: %s", output)
	}
}

func TestRedactingHandlerRecursivelyMasksNestedValues(t *testing.T) {
	var buffer bytes.Buffer
	logger := slog.New(NewRedactingHandler(slog.NewJSONHandler(&buffer, nil)))

	input := map[string]any{
		"password": "map-password",
		"nested": map[string]any{
			"api_key": "nested-api-key",
		},
		"items": []any{
			map[string]any{"token": "slice-token"},
			"plain",
		},
		"mixed": map[string]any{
			"visible": "keep-me",
			"phone":   "+94771234567",
		},
		"array": [2]any{
			map[string]any{"secret": "array-secret"},
			"safe",
		},
		"nil":     nil,
		"bytes":   []byte("byte-secret"),
		"error":   errors.New("error-secret"),
		"address": "12 Temple Road, Nugegoda",
	}

	logger.Info("nested values", "data", input)
	logger.Info("nested group",
		slog.Group("outer", slog.Group("inner", "password", "group-secret")))
	logger.Info("sensitive group", slog.Group("password", "value", "group-key-secret"))
	logger.Info("valuer", "value", secretValuer{})
	logger.Info("stringer", "value", secretStringer{})

	output := buffer.String()
	for _, secret := range []string{
		"map-password",
		"nested-api-key",
		"slice-token",
		"+94771234567",
		"array-secret",
		"byte-secret",
		"error-secret",
		"string-error-secret",
		"12 Temple Road",
		"group-secret",
		"group-key-secret",
		"valuer-secret",
		"stringer-secret",
	} {
		if strings.Contains(output, secret) {
			t.Fatalf("log output leaked %q: %s", secret, output)
		}
	}
	if !strings.Contains(output, "keep-me") || !strings.Contains(output, "plain") || !strings.Contains(output, "safe") {
		t.Fatalf("non-sensitive nested values were removed: %s", output)
	}
	if !strings.Contains(output, redacted) {
		t.Fatalf("expected redaction marker in %s", output)
	}

	if input["password"] != "map-password" {
		t.Fatal("redaction mutated the caller's map")
	}
	nested, ok := input["nested"].(map[string]any)
	if !ok || nested["api_key"] != "nested-api-key" {
		t.Fatal("redaction mutated a nested caller map")
	}
}

func TestRedactingHandlerRedactsNestedCustomFormatters(t *testing.T) {
	tests := []struct {
		name    string
		value   any
		secrets []string
	}{
		{
			name:    "LogValuer value receiver",
			value:   nestedLogValuer("underlying-log-valuer"),
			secrets: []string{"underlying-log-valuer", "method-log-valuer-"},
		},
		{
			name:    "LogValuer pointer receiver pointer",
			value:   pointerTo(nestedPointerLogValuer("underlying-pointer-log-valuer")),
			secrets: []string{"underlying-pointer-log-valuer", "method-pointer-log-valuer-"},
		},
		{
			name:    "LogValuer pointer receiver value",
			value:   nestedPointerLogValuer("underlying-pointer-value-log-valuer"),
			secrets: []string{"underlying-pointer-value-log-valuer", "method-pointer-log-valuer-"},
		},
		{
			name:    "json.Marshaler value receiver",
			value:   nestedJSONMarshaler("underlying-json"),
			secrets: []string{"underlying-json", "method-json-"},
		},
		{
			name:    "json.Marshaler pointer receiver pointer",
			value:   pointerTo(nestedPointerJSONMarshaler("underlying-pointer-json")),
			secrets: []string{"underlying-pointer-json", "method-pointer-json-"},
		},
		{
			name:    "json.Marshaler pointer receiver value",
			value:   nestedPointerJSONMarshaler("underlying-pointer-value-json"),
			secrets: []string{"underlying-pointer-value-json", "method-pointer-json-"},
		},
		{
			name:    "TextMarshaler value receiver",
			value:   nestedTextMarshaler("underlying-text"),
			secrets: []string{"underlying-text", "method-text-"},
		},
		{
			name:    "TextMarshaler pointer receiver pointer",
			value:   pointerTo(nestedPointerTextMarshaler("underlying-pointer-text")),
			secrets: []string{"underlying-pointer-text", "method-pointer-text-"},
		},
		{
			name:    "TextMarshaler pointer receiver value",
			value:   nestedPointerTextMarshaler("underlying-pointer-value-text"),
			secrets: []string{"underlying-pointer-value-text", "method-pointer-text-"},
		},
	}

	structures := []struct {
		name  string
		build func(any) any
	}{
		{
			name: "map",
			build: func(value any) any {
				return map[string]any{"value": value, "safe": "keep-map"}
			},
		},
		{
			name: "slice",
			build: func(value any) any {
				return []any{value, "keep-slice"}
			},
		},
		{
			name: "mixed",
			build: func(value any) any {
				return map[string]any{"items": []any{map[string]any{"value": value, "safe": "keep-mixed"}}}
			},
		},
	}

	for _, test := range tests {
		for _, structure := range structures {
			t.Run(test.name+"/"+structure.name, func(t *testing.T) {
				customFormatterCalls.Store(0)
				var buffer bytes.Buffer
				logger := slog.New(NewRedactingHandler(slog.NewJSONHandler(&buffer, nil)))
				logger.Info("custom formatter", "data", structure.build(test.value))

				output := buffer.String()
				for _, secret := range test.secrets {
					if strings.Contains(output, secret) {
						t.Fatalf("custom formatter leaked %q: %s", secret, output)
					}
				}
				if !strings.Contains(output, redacted) {
					t.Fatalf("expected redaction marker in %s", output)
				}
				if !strings.Contains(output, "keep-"+structure.name) {
					t.Fatalf("non-sensitive value was removed from %s", output)
				}
				if calls := customFormatterCalls.Load(); calls != 0 {
					t.Fatalf("custom formatter methods were invoked %d times", calls)
				}
			})
		}
	}
}

func TestRedactingHandlerHandlesNilCustomFormatters(t *testing.T) {
	customFormatterCalls.Store(0)
	var valuer *nestedPointerLogValuer
	var jsonMarshaler *nestedPointerJSONMarshaler
	var textMarshaler *nestedPointerTextMarshaler

	var buffer bytes.Buffer
	logger := slog.New(NewRedactingHandler(slog.NewJSONHandler(&buffer, nil)))
	logger.Info("nil custom formatters",
		"valuer", valuer,
		"json", jsonMarshaler,
		"text", textMarshaler,
	)

	output := buffer.String()
	if !strings.Contains(output, redacted) {
		t.Fatalf("nil custom formatters were not redacted: %s", output)
	}
	if calls := customFormatterCalls.Load(); calls != 0 {
		t.Fatalf("nil custom formatter methods were invoked %d times", calls)
	}
}

func TestRedactingHandlerPreservesTimeValuesAndRedactsCustomFormatters(t *testing.T) {
	customFormatterCalls.Store(0)
	now := time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)
	var nilTime *time.Time

	var buffer bytes.Buffer
	logger := slog.New(NewRedactingHandler(slog.NewJSONHandler(&buffer, nil)))
	logger.Info("time and custom values",
		"value", now,
		"pointer", &now,
		"nil", nilTime,
		"json", nestedJSONMarshaler("time-json-secret"),
		"text", nestedTextMarshaler("time-text-secret"),
		"valuer", nestedLogValuer("time-valuer-secret"),
	)

	var record map[string]any
	if err := json.Unmarshal(buffer.Bytes(), &record); err != nil {
		t.Fatalf("decode log output: %v: %s", err, buffer.String())
	}

	const wantTime = "2026-11-03T12:00:00Z"
	if record["value"] != wantTime {
		t.Fatalf("time value = %v, want %s", record["value"], wantTime)
	}
	if record["pointer"] != wantTime {
		t.Fatalf("time pointer = %v, want %s", record["pointer"], wantTime)
	}
	if record["nil"] != nil {
		t.Fatalf("nil time pointer = %v, want null", record["nil"])
	}
	for _, field := range []string{"json", "text", "valuer"} {
		if record[field] != redacted {
			t.Fatalf("%s = %v, want %s", field, record[field], redacted)
		}
	}
	output := buffer.String()
	for _, secret := range []string{"time-json-secret", "time-text-secret", "time-valuer-secret", "method-json-", "method-text-", "method-log-valuer-"} {
		if strings.Contains(output, secret) {
			t.Fatalf("log output leaked %q: %s", secret, output)
		}
	}
	if calls := customFormatterCalls.Load(); calls != 0 {
		t.Fatalf("custom formatter methods were invoked %d times", calls)
	}
}

func TestRedactingHandlerHandlesCyclicContainers(t *testing.T) {
	var buffer bytes.Buffer
	logger := slog.New(NewRedactingHandler(slog.NewJSONHandler(&buffer, nil)))

	cyclicMap := map[string]any{"name": "cycle-map"}
	cyclicMap["self"] = cyclicMap
	cyclicSlice := []any{"cycle-slice"}
	cyclicSlice[0] = cyclicSlice

	logger.Info("cycles", "map", cyclicMap, "slice", cyclicSlice)

	output := buffer.String()
	if !strings.Contains(output, "cycle-map") || !strings.Contains(output, redacted) {
		t.Fatalf("cyclic map was not handled safely: %s", output)
	}
	if strings.Contains(output, "cycle-slice") {
		t.Fatalf("cyclic slice leaked its value: %s", output)
	}
}

func TestBodyForLogNeverReturnsRequestBodies(t *testing.T) {
	body := []byte(`{"email":"nimal@example.com","password":"Str0ngPass!","refreshToken":"rt_9fA"}`)

	for _, path := range []string{"/auth/login", "/api/v1/auth/login", "/users/me/password", "/parcels"} {
		got := BodyForLog(path, body)
		if got != redacted {
			t.Fatalf("BodyForLog(%q) = %q, want %q", path, got, redacted)
		}
	}
}

func TestIsAuthPath(t *testing.T) {
	auth := []string{"/auth/login", "/api/v1/auth/refresh", "/users/me/password", "/auth/reset-password"}
	for _, path := range auth {
		if !IsAuthPath(path) {
			t.Fatalf("IsAuthPath(%q) = false, want true", path)
		}
	}
	if IsAuthPath("/parcels") {
		t.Fatal("IsAuthPath() should be false for a business path")
	}
}
