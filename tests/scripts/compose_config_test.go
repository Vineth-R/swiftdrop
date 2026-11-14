package scripts_test

import (
	"os"
	"strings"
	"testing"
)

func TestComposeMerchantReceivesWebhookKeyringSettings(t *testing.T) {
	compose, err := os.ReadFile("../../docker-compose.yml")
	if err != nil {
		t.Fatalf("read docker-compose.yml: %v", err)
	}
	text := string(compose)
	merchant := text[strings.Index(text, "  merchant:"):]
	if end := strings.Index(merchant, "\n  realtime:"); end >= 0 {
		merchant = merchant[:end]
	}
	for _, want := range []string{
		"WEBHOOK_SECRET_KEY:",
		"WEBHOOK_SECRET_KEY_ID:",
		"WEBHOOK_SECRET_KEYS:",
	} {
		if !strings.Contains(merchant, want) {
			t.Fatalf("merchant service does not pass %s", want)
		}
	}

	env, err := os.ReadFile("../../.env.example")
	if err != nil {
		t.Fatalf("read .env.example: %v", err)
	}
	for _, want := range []string{"WEBHOOK_SECRET_KEY=", "WEBHOOK_SECRET_KEY_ID=k1", "WEBHOOK_SECRET_KEYS="} {
		if !strings.Contains(string(env), want) {
			t.Fatalf(".env.example does not document %s", want)
		}
	}
}
