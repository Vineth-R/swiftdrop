package config

import (
	"strings"
	"testing"

	sharedconfig "github.com/Vineth-R/swiftdrop/packages/go-common/config"
)

func TestLoadInternalToken(t *testing.T) {
	loader := func(values map[string]string) sharedconfig.Loader {
		return sharedconfig.NewLoader(func(key string) (string, bool) {
			value, ok := values[key]
			return value, ok
		})
	}
	valid := strings.Repeat("a", MinInternalTokenLength)
	validNonDevelopment := strings.Repeat("a", sharedconfig.MinInternalTokenLengthNonDevelopment)
	placeholder := "change-me-internal-token"

	tests := []struct {
		name        string
		values      map[string]string
		appEnv      string
		wantToken   string
		wantProblem string
	}{
		{name: "valid development", values: map[string]string{"INTERNAL_TOKEN": valid}, appEnv: "development", wantToken: valid},
		{name: "exactly the development minimum", values: map[string]string{"INTERNAL_TOKEN": valid}, appEnv: "development", wantToken: valid},
		{name: "development placeholder remains usable", values: map[string]string{"INTERNAL_TOKEN": placeholder}, appEnv: "development", wantToken: placeholder},
		{name: "missing", values: nil, appEnv: "development", wantProblem: "INTERNAL_TOKEN is not set"},
		{name: "empty", values: map[string]string{"INTERNAL_TOKEN": ""}, appEnv: "production", wantProblem: "INTERNAL_TOKEN is not set"},
		{name: "only spaces", values: map[string]string{"INTERNAL_TOKEN": "     "}, appEnv: "production", wantProblem: "INTERNAL_TOKEN is not set"},
		{name: "too short development", values: map[string]string{"INTERNAL_TOKEN": "short-secret"}, appEnv: "development", wantProblem: "INTERNAL_TOKEN is shorter than 16 characters"},
		{name: "too short non-development", values: map[string]string{"INTERNAL_TOKEN": valid}, appEnv: "production", wantProblem: "INTERNAL_TOKEN must be at least 32 bytes outside development"},
		{name: "placeholder staging", values: map[string]string{"INTERNAL_TOKEN": placeholder}, appEnv: "staging", wantProblem: "INTERNAL_TOKEN must not be the example placeholder outside development"},
		{name: "placeholder production", values: map[string]string{"INTERNAL_TOKEN": placeholder}, appEnv: "production", wantProblem: "INTERNAL_TOKEN must not be the example placeholder outside development"},
		{name: "valid staging", values: map[string]string{"INTERNAL_TOKEN": validNonDevelopment}, appEnv: "staging", wantToken: validNonDevelopment},
		{name: "valid production", values: map[string]string{"INTERNAL_TOKEN": validNonDevelopment}, appEnv: "production", wantToken: validNonDevelopment},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			token, problem := LoadInternalToken(loader(test.values), test.appEnv)

			if token != test.wantToken || problem != test.wantProblem {
				t.Fatalf("LoadInternalToken() = %q, %q, want %q, %q", token, problem, test.wantToken, test.wantProblem)
			}
			if problem != "" && test.values["INTERNAL_TOKEN"] != "" && strings.Contains(problem, test.values["INTERNAL_TOKEN"]) {
				t.Fatal("the problem text contains the token")
			}
		})
	}
}
