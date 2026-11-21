package config

import (
	"strings"
	"testing"
)

func TestLoadFromInternalToken(t *testing.T) {
	strong := strings.Repeat("a1", 16) // 32 bytes
	tests := []struct {
		name        string
		appEnv      string
		token       string
		set         bool
		wantToken   string
		wantClosed  bool // token empty and a problem reported
		wantStartup bool // LoadFrom must fail
	}{
		{name: "development without token is closed", appEnv: "development", wantClosed: true},
		{name: "development short token is closed", appEnv: "development", token: "abc12345", set: true, wantClosed: true},
		{name: "development 16 characters works", appEnv: "development", token: strings.Repeat("x", 16), set: true, wantToken: strings.Repeat("x", 16)},
		{name: "production without token is closed", appEnv: "production", wantClosed: true},
		{name: "production placeholder stops the service", appEnv: "production", token: "change-me-internal-token-change-me-0000", set: true, wantStartup: true},
		{name: "production short token stops the service", appEnv: "production", token: strings.Repeat("x", 20), set: true, wantStartup: true},
		{name: "production strong token works", appEnv: "production", token: strong, set: true, wantToken: strong},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			values := map[string]string{"APP_ENV": test.appEnv}
			if test.set {
				values["INTERNAL_TOKEN"] = test.token
			}
			config, err := LoadFrom(webhookLoader(values))
			if test.wantStartup {
				if err == nil {
					t.Fatal("LoadFrom accepted a weak internal token outside development")
				}
				if strings.Contains(err.Error(), test.token) {
					t.Fatalf("error leaks the token: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("LoadFrom returned error: %v", err)
			}
			if config.InternalToken != test.wantToken {
				t.Fatalf("InternalToken = %q, want %q", config.InternalToken, test.wantToken)
			}
			if test.wantClosed && config.InternalTokenProblem == "" {
				t.Fatal("a closed token must report why")
			}
			if !test.wantClosed && config.InternalTokenProblem != "" {
				t.Fatalf("unexpected problem %q", config.InternalTokenProblem)
			}
			if strings.Contains(config.InternalTokenProblem, test.token) && test.token != "" {
				t.Fatal("the problem text must never contain the token")
			}
		})
	}
}
