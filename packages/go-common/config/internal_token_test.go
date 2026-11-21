package config

import (
	"strings"
	"testing"
)

func TestLoadInternalToken(t *testing.T) {
	loader := func(value string) Loader {
		return NewLoader(func(key string) (string, bool) {
			if key == "INTERNAL_TOKEN" {
				return value, true
			}
			return "", false
		})
	}

	validDevelopment := strings.Repeat("d", MinInternalTokenLength)
	validNonDevelopment := strings.Repeat("n", MinInternalTokenLengthNonDevelopment)
	placeholder := "change-me-internal-token"

	tests := []struct {
		name        string
		value       string
		appEnv      string
		wantToken   string
		wantProblem string
	}{
		{name: "empty token", appEnv: "development", wantProblem: "INTERNAL_TOKEN is not set"},
		{name: "empty app environment", value: placeholder, appEnv: "", wantProblem: "INTERNAL_TOKEN must not be the example placeholder outside development"},
		{name: "whitespace token", value: "   ", appEnv: "production", wantProblem: "INTERNAL_TOKEN is not set"},
		{name: "valid development token", value: validDevelopment, appEnv: "development", wantToken: validDevelopment},
		{name: "development keeps example placeholder", value: placeholder, appEnv: "development", wantToken: placeholder},
		{name: "too short development token", value: strings.Repeat("d", MinInternalTokenLength-1), appEnv: "development", wantProblem: "INTERNAL_TOKEN is shorter than 16 characters"},
		{name: "valid non-development token", value: validNonDevelopment, appEnv: "production", wantToken: validNonDevelopment},
		{name: "known placeholder non-development", value: placeholder, appEnv: "production", wantProblem: "INTERNAL_TOKEN must not be the example placeholder outside development"},
		{name: "case-insensitive placeholder non-development", value: "Change-Me-Internal-Token", appEnv: "staging", wantProblem: "INTERNAL_TOKEN must not be the example placeholder outside development"},
		{name: "no-hyphen placeholder non-development", value: "changeme-internal-token", appEnv: "production", wantProblem: "INTERNAL_TOKEN must not be the example placeholder outside development"},
		{name: "too short non-development token", value: strings.Repeat("n", MinInternalTokenLengthNonDevelopment-1), appEnv: "production", wantProblem: "INTERNAL_TOKEN must be at least 32 bytes outside development"},
		{name: "empty non-development token", appEnv: "production", wantProblem: "INTERNAL_TOKEN is not set"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			token, problem := LoadInternalToken(loader(test.value), test.appEnv)

			if token != test.wantToken || problem != test.wantProblem {
				t.Fatalf("LoadInternalToken() = %q, %q, want %q, %q", token, problem, test.wantToken, test.wantProblem)
			}
			if problem != "" && test.value != "" && strings.Contains(problem, test.value) {
				t.Fatal("the problem text contains the token")
			}
		})
	}
}
