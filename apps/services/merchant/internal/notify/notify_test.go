package notify

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Vineth-R/swiftdrop/packages/go-common/emails"
	"github.com/Vineth-R/swiftdrop/packages/go-common/mailer"
)

type fakeMailer struct {
	mu       sync.Mutex
	messages []mailer.Message
	err      error
}

func (m *fakeMailer) Send(_ context.Context, message mailer.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = append(m.messages, message)
	return m.err
}

func TestNotifierSendsEveryMerchantEmail(t *testing.T) {
	sender := &fakeMailer{}
	n := New(sender, emails.Site{BaseURL: "https://swiftdrop.test", SupportEmail: "support@swiftdrop.test"}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	n.dispatch = func(job func()) { job() }

	ctx := context.Background()
	now := time.Now()
	n.Received(ctx, "owner@example.test", "Owner", "Shop", "owner@example.test", now)
	n.Approved(ctx, "owner@example.test", "Owner", "Shop", now)
	n.Rejected(ctx, "owner@example.test", "Owner", "Shop", nil)
	n.Suspended(ctx, "owner@example.test", "Owner", "Shop", ptr("review"))
	n.Wait()

	if len(sender.messages) != 4 {
		t.Fatalf("messages = %d, want 4", len(sender.messages))
	}
	for _, message := range sender.messages {
		if message.To != "owner@example.test" || message.Subject == "" {
			t.Fatalf("message = %+v", message)
		}
	}
}

func TestNotifierRetriesAndLogsFailures(t *testing.T) {
	sender := &fakeMailer{err: errors.New("smtp down")}
	n := New(sender, emails.Site{BaseURL: "https://swiftdrop.test"}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	n.dispatch = func(job func()) { job() }

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	n.Approved(ctx, "owner@example.test", "Owner", "Shop", time.Now())
	n.Wait()
	if len(sender.messages) != 2 {
		t.Fatalf("messages = %d, want both failed attempts", len(sender.messages))
	}
}

func ptr(value string) *string { return &value }

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
