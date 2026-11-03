package scripts_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const preflightCompose = `services:
  identity:
    image: example.invalid/identity
    environment:
      JWT_ACCESS_SECRET: ${JWT_ACCESS_SECRET:-}
`

func TestPreflightRejectsEmptyOrWhitespaceJWT(t *testing.T) {
	if !preflightToolsAvailable(t) {
		return
	}

	const sentinel = "sentinel-secret-must-not-appear"
	tests := []struct {
		name    string
		dotenv  string
		env     map[string]string
		wantErr bool
		secret  string
	}{
		{
			name:    "missing secret",
			wantErr: true,
		},
		{
			name:    "missing .env does not leak exported secret",
			env:     map[string]string{"JWT_ACCESS_SECRET": sentinel},
			wantErr: true,
			secret:  sentinel,
		},
		{
			name:    "empty exported secret",
			env:     map[string]string{"POSTGRES_USER": "swiftdrop", "POSTGRES_DB": "swiftdrop", "JWT_ACCESS_SECRET": ""},
			wantErr: true,
		},
		{
			name:    "empty quoted dotenv secret",
			dotenv:  "JWT_ACCESS_SECRET=\"\"\n",
			wantErr: true,
		},
		{
			name:    "whitespace-only exported secret",
			env:     map[string]string{"POSTGRES_USER": "swiftdrop", "POSTGRES_DB": "swiftdrop", "JWT_ACCESS_SECRET": "   "},
			wantErr: true,
		},
		{
			name:    "whitespace-only quoted dotenv secret",
			dotenv:  "JWT_ACCESS_SECRET=\"   \"\n",
			wantErr: true,
		},
		{
			name:    "valid exported secret",
			env:     map[string]string{"POSTGRES_USER": "swiftdrop", "POSTGRES_DB": "swiftdrop", "JWT_ACCESS_SECRET": "valid-secret"},
			wantErr: false,
		},
		{
			name:    "valid quoted dotenv secret",
			dotenv:  "JWT_ACCESS_SECRET=\"valid-secret\"\n",
			wantErr: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			output, err := runPreflight(t, test.dotenv, test.env)
			if test.wantErr && err == nil {
				t.Fatalf("preflight accepted invalid configuration:\n%s", output)
			}
			if !test.wantErr && err != nil {
				t.Fatalf("preflight rejected valid configuration: %v\n%s", err, output)
			}
			if test.secret != "" && strings.Contains(output, test.secret) {
				t.Fatalf("preflight output leaked the configured secret:\n%s", output)
			}
		})
	}
}

func preflightToolsAvailable(t *testing.T) bool {
	t.Helper()
	if _, err := exec.LookPath("make"); err != nil {
		t.Skip("make is not available; skipping preflight regression tests")
		return false
	}
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker is not available; skipping preflight regression tests")
		return false
	}
	if err := exec.Command("docker", "compose", "version").Run(); err != nil {
		t.Skip("docker compose is not available; skipping preflight regression tests")
		return false
	}
	return true
}

func runPreflight(t *testing.T, dotenv string, values map[string]string) (string, error) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte(preflightCompose), 0o600); err != nil {
		t.Fatalf("write test compose file: %v", err)
	}
	if dotenv != "" {
		if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(dotenv), 0o600); err != nil {
			t.Fatalf("write test .env: %v", err)
		}
	}

	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}
	cmd := exec.Command("make", "-C", dir, "-f", filepath.Join(repoRoot, "Makefile"), "preflight")
	cmd.Env = preflightEnvironment(values)
	output, err := cmd.CombinedOutput()
	return string(output), err
}

func preflightEnvironment(values map[string]string) []string {
	drop := map[string]bool{
		"COMPOSE":              true,
		"COMPOSE_FILE":         true,
		"COMPOSE_PROJECT_NAME": true,
		"JWT_ACCESS_SECRET":    true,
		"MAKEFLAGS":            true,
		"MFLAGS":               true,
		"POSTGRES_DB":          true,
		"POSTGRES_PASSWORD":    true,
		"POSTGRES_USER":        true,
	}
	env := []string{}
	for _, entry := range os.Environ() {
		key := strings.SplitN(entry, "=", 2)[0]
		if !drop[key] {
			env = append(env, entry)
		}
	}
	for key, value := range values {
		env = append(env, key+"="+value)
	}
	return env
}
