package config

import (
	"strings"
	"testing"
)

func TestLoadAccessSecret(t *testing.T) {
	valid := strings.Repeat("v", MinAccessSecretBytes)
	weak := strings.Repeat("w", MinAccessSecretBytes-1)
	placeholder := "change-me-access-secret-use-32-or-more-random-characters"

	tests := []struct {
		name     string
		value    string
		appEnv   string
		required bool
		want     string
		wantErr  bool
	}{
		{name: "development empty", appEnv: "development"},
		{name: "development weak", value: weak, appEnv: "development", want: weak},
		{name: "development placeholder", value: placeholder, appEnv: "development", want: placeholder},
		{name: "development required empty", appEnv: "development", required: true, wantErr: true},
		{name: "production optional empty", appEnv: "production"},
		{name: "production weak", value: weak, appEnv: "production", wantErr: true},
		{name: "production placeholder", value: placeholder, appEnv: "production", wantErr: true},
		{name: "staging case-insensitive placeholder", value: strings.ToUpper(placeholder), appEnv: "staging", wantErr: true},
		{name: "production valid", value: valid, appEnv: "production", want: valid},
		{name: "staging valid", value: valid, appEnv: "staging", want: valid},
		{name: "production required empty", appEnv: "production", required: true, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			loader := NewLoader(func(key string) (string, bool) {
				if key == "JWT_ACCESS_SECRET" {
					return test.value, true
				}
				return "", false
			})
			got, err := LoadAccessSecret(loader, test.appEnv, test.required)
			if test.wantErr {
				if err == nil {
					t.Fatal("LoadAccessSecret() should have failed")
				}
				if test.value != "" && strings.Contains(err.Error(), test.value) {
					t.Fatal("the error text contains the secret")
				}
				return
			}
			if err != nil {
				t.Fatalf("LoadAccessSecret() returned error: %v", err)
			}
			if got != test.want {
				t.Fatalf("LoadAccessSecret() = %q, want %q", got, test.want)
			}
		})
	}
}
