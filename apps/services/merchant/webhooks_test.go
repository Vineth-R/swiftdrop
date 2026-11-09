package main

import (
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func quietLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestRegisterWebhooksWithoutKeySkipsRoutes(t *testing.T) {
	router := gin.New()
	if err := registerWebhooks(router, nil, "", quietLogger()); err != nil {
		t.Fatalf("an unset key must not stop the service: %v", err)
	}
	if got := len(router.Routes()); got != 0 {
		t.Fatalf("routes registered without a key: %d", got)
	}
}

func TestRegisterWebhooksRejectsInvalidKey(t *testing.T) {
	for _, key := range []string{"not-hex", "abcd", strings.Repeat("zz", 32)} {
		if err := registerWebhooks(gin.New(), nil, key, quietLogger()); err == nil {
			t.Errorf("key %q was accepted", key)
		}
	}
}

func TestRegisterWebhooksWithValidKeyAddsFourRoutes(t *testing.T) {
	router := gin.New()
	if err := registerWebhooks(router, nil, strings.Repeat("ab", 32), quietLogger()); err != nil {
		t.Fatal(err)
	}
	if got := len(router.Routes()); got != 4 {
		t.Fatalf("routes = %d, want 4", got)
	}
}
