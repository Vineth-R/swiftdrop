package config

import "testing"

func TestLoadAppEnv(t *testing.T) {
	tests := []struct {
		name    string
		values  map[string]string
		want    string
		wantErr bool
	}{
		{name: "unset", wantErr: true},
		{name: "empty", values: map[string]string{"APP_ENV": ""}, wantErr: true},
		{name: "whitespace", values: map[string]string{"APP_ENV": "   "}, wantErr: true},
		{name: "unknown", values: map[string]string{"APP_ENV": "local"}, wantErr: true},
		{name: "development", values: map[string]string{"APP_ENV": "development"}, want: "development"},
		{name: "staging", values: map[string]string{"APP_ENV": "staging"}, want: "staging"},
		{name: "production", values: map[string]string{"APP_ENV": "production"}, want: "production"},
		{name: "case and whitespace", values: map[string]string{"APP_ENV": "  PRODUCTION "}, want: "production"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			loader := NewLoader(func(key string) (string, bool) {
				value, ok := test.values[key]
				return value, ok
			})
			got, err := LoadAppEnv(loader)
			if test.wantErr {
				if err == nil {
					t.Fatalf("LoadAppEnv() accepted APP_ENV=%q", test.values["APP_ENV"])
				}
				return
			}
			if err != nil {
				t.Fatalf("LoadAppEnv() returned error: %v", err)
			}
			if got != test.want {
				t.Fatalf("LoadAppEnv() = %q, want %q", got, test.want)
			}
		})
	}
}
