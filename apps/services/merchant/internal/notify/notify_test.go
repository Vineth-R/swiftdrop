package notify

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/Vineth-R/swiftdrop/packages/go-common/emails"
	"github.com/Vineth-R/swiftdrop/packages/go-common/mailer"
)

func TestFailedSendIsLoggedWithoutRecipientOrErrorText(t *testing.T) {
	mock := &mailer.Mock{}
	mock.FailWith(errors.New("550 nimali@example.com rejected: password reset link"))
	var logs bytes.Buffer
	notifier := New(mock, emails.Site{BaseURL: "https://app.example"},
		slog.New(slog.NewTextHandler(&logs, nil)))

	notifier.Approved(context.Background(), "nimali@example.com", "Nimali", "SwiftDrop", time.Now())
	notifier.Wait()

	output := logs.String()
	if !strings.Contains(output, "merchant email was not sent") || !strings.Contains(output, "send_failed") {
		t.Fatalf("failure was not logged with a stable reason: %s", output)
	}
	for _, leak := range []string{"nimali@example.com", "550", "password reset link"} {
		if strings.Contains(output, leak) {
			t.Fatalf("log leaked %q: %s", leak, output)
		}
	}
}
