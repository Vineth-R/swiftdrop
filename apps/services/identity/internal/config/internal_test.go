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
	good := "a-sufficiently-long-token"

	tests := []struct {
		name        string
		values      map[string]string
		wantToken   string
		wantProblem string
	}{
		{name: "valid", values: map[string]string{"INTERNAL_TOKEN": good}, wantToken: good},
		{name: "exactly the minimum", values: map[string]string{"INTERNAL_TOKEN": strings.Repeat("a", MinInternalTokenLength)}, wantToken: strings.Repeat("a", MinInternalTokenLength)},
		{name: "missing", values: nil, wantProblem: "INTERNAL_TOKEN is not set"},
		{name: "empty", values: map[string]string{"INTERNAL_TOKEN": ""}, wantProblem: "INTERNAL_TOKEN is not set"},
		{name: "only spaces", values: map[string]string{"INTERNAL_TOKEN": "     "}, wantProblem: "INTERNAL_TOKEN is not set"},
		{name: "too short", values: map[string]string{"INTERNAL_TOKEN": "short-secret"}, wantProblem: "INTERNAL_TOKEN is shorter than 16 characters"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			token, problem := LoadInternalToken(loader(test.values))

			if token != test.wantToken || problem != test.wantProblem {
				t.Fatalf("LoadInternalToken() = %q, %q, want %q, %q", token, problem, test.wantToken, test.wantProblem)
			}
			if problem != "" && strings.Contains(problem, test.values["INTERNAL_TOKEN"]) && test.values["INTERNAL_TOKEN"] != "" {
				t.Fatal("the problem text contains the token")
			}
		})
	}
}
