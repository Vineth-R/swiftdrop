package notify

import (
	"context"
	"errors"
	"io"
	"log/slog"
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
