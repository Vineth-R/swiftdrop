package main

import (
	"io"
	"log/slog"
	"strings"
	"testing"
)

func TestInternalRouterOptionRejectsInvalidTokenOutsideDevelopment(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	for _, test := range []struct {
		name  string
		token string
	}{
		{name: "known placeholder", token: "change-me-internal-token"},
		{name: "empty", token: ""},
		{name: "too short", token: strings.Repeat("s", 16)},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("INTERNAL_TOKEN", test.token)

			_, err := internalRouterOption(logger, nil, nil, "production")
			if err == nil {
				t.Fatal("internalRouterOption() should fail")
			}
			if !strings.Contains(err.Error(), "load internal token") {
				t.Fatalf("error = %q, want an internal token startup error", err)
			}
			if test.token != "" && strings.Contains(err.Error(), test.token) {
				t.Fatal("the error text contains the token")
			}
		})
	}
}
