package scripts_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const pruneScript = "../../scripts/prune-retention.sh"

// capturePruneCompose writes a fake `docker compose` that records the SQL it
// receives on stdin. The real script then never touches a database.
func capturePruneCompose(t *testing.T) (compose, capture string) {
	t.Helper()
	dir := t.TempDir()
	compose = filepath.Join(dir, "fake-compose")
	capture = filepath.Join(dir, "captured.sql")
	script := "#!/bin/sh\ncat > \"$CAPTURE_FILE\"\n"
	if err := os.WriteFile(compose, []byte(script), 0o700); err != nil {
		t.Fatalf("write fake compose: %v", err)
	}
	return compose, capture
}

func runPrune(t *testing.T, values map[string]string) (string, string, error) {
	t.Helper()
	compose, capture := capturePruneCompose(t)
	cmd := exec.Command("sh", pruneScript)
	cmd.Env = pruneEnvironment(values, compose, capture)
	output, err := cmd.CombinedOutput()
	sql, _ := os.ReadFile(capture)
	return string(output), string(sql), err
}

func pruneEnvironment(values map[string]string, compose, capture string) []string {
	drop := map[string]bool{
		"APPLY":                          true,
		"AUDIT_RETENTION_DAYS":           true,
		"BACKUP_SOURCE":                  true,
		"BATCH_SIZE":                     true,
		"COMPOSE":                        true,
		"COMPOSE_FILE":                   true,
		"COMPOSE_PROJECT_NAME":           true,
		"CAPTURE_FILE":                   true,
		"DATABASE_URL":                   true,
		"MAX_BATCHES":                    true,
		"WEBHOOK_ATTEMPT_RETENTION_DAYS": true,
	}
	env := []string{"PATH=" + os.Getenv("PATH"), "COMPOSE=" + compose, "CAPTURE_FILE=" + capture}
	for _, entry := range os.Environ() {
		name := entry[:strings.IndexByte(entry, '=')]
		if !drop[name] {
			env = append(env, entry)
		}
	}
	for name, value := range values {
		env = append(env, name+"="+value)
	}
	return env
}

func TestPruneRetentionDryRunIsReadOnlyAndScoped(t *testing.T) {
	output, sql, err := runPrune(t, nil)
	if err != nil {
		t.Fatalf("dry run failed: %v\n%s", err, output)
	}
	for _, want := range []string{
		"'identity.audit_logs'",
		"'merchant.audit_logs'",
		"'pricing.audit_logs'",
		"'merchant.webhook_delivery_attempts'",
		"interval '365 days'",
		"interval '90 days'",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("dry-run SQL does not contain %q:\n%s", want, sql)
		}
	}
	if strings.Contains(sql, "DELETE") {
		t.Fatalf("dry run must not delete anything:\n%s", sql)
	}
	for _, forbidden := range []string{
		"notification.",
		"parcel.",
		"merchant.orders",
		"reservation",
		"delivery_proofs",
	} {
		if strings.Contains(sql, forbidden) {
			t.Fatalf("dry-run SQL touches %q, which the retention policy does not own:\n%s", forbidden, sql)
		}
	}
	if !strings.Contains(output, "dry run only; set APPLY=1") {
		t.Fatalf("dry run did not announce that it is read-only:\n%s", output)
	}
}

func TestPruneRetentionApplyUsesBoundedDeletesOnly(t *testing.T) {
	output, sql, err := runPrune(t, map[string]string{
		"APPLY":       "1",
		"BATCH_SIZE":  "250",
		"MAX_BATCHES": "3",
	})
	if err != nil {
		t.Fatalf("apply run failed: %v\n%s", err, output)
	}
	if got := strings.Count(sql, "DELETE FROM"); got != 4 {
		t.Fatalf("apply SQL has %d DELETE statements, want exactly the four owned tables:\n%s", got, sql)
	}
	for _, want := range []string{
		"DELETE FROM identity.audit_logs",
		"DELETE FROM merchant.audit_logs",
		"DELETE FROM pricing.audit_logs",
		"DELETE FROM merchant.webhook_delivery_attempts",
		"1..3 LOOP",
		"LIMIT 250",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("apply SQL does not contain %q:\n%s", want, sql)
		}
	}
	for _, forbidden := range []string{
		"notification.",
		"parcel.",
		"merchant.orders",
		"reservation",
		"delivery_proofs",
	} {
		if strings.Contains(sql, forbidden) {
			t.Fatalf("apply SQL touches %q, which the retention policy does not own:\n%s", forbidden, sql)
		}
	}
	if !strings.Contains(output, "batch_size=250 max_batches=3") {
		t.Fatalf("apply run did not report its batching bounds:\n%s", output)
	}
}

func TestPruneRetentionRejectsUnsafeWindowsAndLimits(t *testing.T) {
	tests := []struct {
		name   string
		values map[string]string
		want   string
	}{
		{
			name:   "audit window under 30 days",
			values: map[string]string{"AUDIT_RETENTION_DAYS": "29"},
			want:   "retention windows below 30 days are refused",
		},
		{
			name:   "webhook window under 30 days",
			values: map[string]string{"WEBHOOK_ATTEMPT_RETENTION_DAYS": "29"},
			want:   "retention windows below 30 days are refused",
		},
		{
			name:   "non-numeric audit window",
			values: map[string]string{"AUDIT_RETENTION_DAYS": "year"},
			want:   "AUDIT_RETENTION_DAYS must be a positive integer",
		},
		{
			name:   "apply must be zero or one",
			values: map[string]string{"APPLY": "2"},
			want:   "APPLY must be 0 or 1",
		},
		{
			name:   "zero batch size",
			values: map[string]string{"BATCH_SIZE": "0"},
			want:   "BATCH_SIZE and MAX_BATCHES must be positive",
		},
		{
			name:   "zero batch count",
			values: map[string]string{"MAX_BATCHES": "0"},
			want:   "BATCH_SIZE and MAX_BATCHES must be positive",
		},
		{
			name:   "non-numeric batch size",
			values: map[string]string{"BATCH_SIZE": "large"},
			want:   "BATCH_SIZE must be a positive integer",
		},
		{
			name:   "non-numeric batch count",
			values: map[string]string{"MAX_BATCHES": "many"},
			want:   "MAX_BATCHES must be a positive integer",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			output, sql, err := runPrune(t, test.values)
			if err == nil {
				t.Fatalf("unsafe input was accepted:\n%s", output)
			}
			if !strings.Contains(output, test.want) {
				t.Fatalf("error = %q, want it to contain %q", output, test.want)
			}
			if strings.Contains(sql, "DELETE") {
				t.Fatalf("rejected input still produced delete SQL:\n%s", sql)
			}
		})
	}
}

func TestPruneRetentionUsesCustomWindows(t *testing.T) {
	output, sql, err := runPrune(t, map[string]string{
		"AUDIT_RETENTION_DAYS":           "730",
		"WEBHOOK_ATTEMPT_RETENTION_DAYS": "120",
	})
	if err != nil {
		t.Fatalf("custom windows failed: %v\n%s", err, output)
	}
	if !strings.Contains(sql, "interval '730 days'") {
		t.Fatalf("audit window not applied:\n%s", sql)
	}
	if !strings.Contains(sql, "interval '120 days'") {
		t.Fatalf("webhook window not applied:\n%s", sql)
	}
}
