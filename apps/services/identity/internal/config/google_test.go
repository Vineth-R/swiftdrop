package config

import (
	"strings"
	"testing"
)

func TestConfig_GoogleProblems(t *testing.T) {
	complete := Config{
		GoogleClientID:     "client-id-example",
		GoogleClientSecret: "test-only-secret",
		GoogleRedirectURI:  "http://localhost:8080/api/v1/auth/google/callback",
	}

	tests := []struct {
		name   string
		change func(*Config)
		want   []string
	}{
		{name: "complete", change: func(*Config) {}, want: nil},
		{name: "https redirect", change: func(c *Config) { c.GoogleRedirectURI = "https://swiftdrop.example/cb" }, want: nil},
		{name: "nothing set", change: func(c *Config) { *c = Config{} }, want: []string{
			"GOOGLE_CLIENT_ID is not set", "GOOGLE_CLIENT_SECRET is not set", "GOOGLE_REDIRECT_URI is not set",
		}},
		{name: "no client ID", change: func(c *Config) { c.GoogleClientID = "" }, want: []string{"GOOGLE_CLIENT_ID is not set"}},
		{name: "no client secret", change: func(c *Config) { c.GoogleClientSecret = "" }, want: []string{"GOOGLE_CLIENT_SECRET is not set"}},
		{name: "no redirect URI", change: func(c *Config) { c.GoogleRedirectURI = "" }, want: []string{"GOOGLE_REDIRECT_URI is not set"}},
		{name: "relative redirect URI", change: func(c *Config) { c.GoogleRedirectURI = "/auth/google/callback" }, want: []string{
			"GOOGLE_REDIRECT_URI must be an absolute http or https URL",
		}},
		{name: "wrong scheme", change: func(c *Config) { c.GoogleRedirectURI = "ftp://example.com/cb" }, want: []string{
			"GOOGLE_REDIRECT_URI must be an absolute http or https URL",
		}},
		{name: "unparsable redirect URI", change: func(c *Config) { c.GoogleRedirectURI = "http://[::1" }, want: []string{
			"GOOGLE_REDIRECT_URI must be an absolute http or https URL",
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := complete
			test.change(&cfg)

			got := cfg.GoogleProblems()
			if strings.Join(got, "|") != strings.Join(test.want, "|") {
				t.Fatalf("GoogleProblems() = %q, want %q", got, test.want)
			}
			for _, problem := range got {
				if strings.Contains(problem, "test-only-secret") || strings.Contains(problem, "client-id-example") {
					t.Fatalf("problem %q leaks a setting value", problem)
				}
			}
		})
	}
}
