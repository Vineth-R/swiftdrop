package notify

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/Vineth-R/swiftdrop/apps/services/identity/internal/service"
	"github.com/Vineth-R/swiftdrop/packages/go-common/auth"
	"github.com/Vineth-R/swiftdrop/packages/go-common/emails"
	"github.com/Vineth-R/swiftdrop/packages/go-common/mailer"
)

var (
	testSite = emails.Site{BaseURL: "https://app.swiftdrop.example", SupportEmail: "support@swiftdrop.example"}
	nimali   = service.Recipient{UserID: "user-1", Email: "nimali@example.com", FullName: "Nimali Perera"}
	moment   = time.Date(2026, time.November, 4, 8, 35, 0, 0, time.UTC)
)

func newSender(t *testing.T, opts ...Option) (*Sender, *mailer.Mock, *bytes.Buffer) {
	t.Helper()
	mock := &mailer.Mock{}
	logs := &bytes.Buffer{}
	return New(mock, testSite, slog.New(slog.NewTextHandler(logs, nil)), opts...), mock, logs
}

func TestSender_sendsTheRightEmailForEveryEvent(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name     string
		send     func(*Sender)
		subject  string
		contains []string
	}{
		{"welcome", func(s *Sender) { s.Welcome(ctx, nimali) }, "Welcome to SwiftDrop",
			[]string{"Hello Nimali Perera,", "https://app.swiftdrop.example/dashboard"}},
		{"password changed from the profile", func(s *Sender) { s.PasswordChanged(ctx, nimali, false, moment) }, "Your SwiftDrop password was changed",
			[]string{"4 Nov 2026, 14:05 (Sri Lanka time)", "From your profile", "https://app.swiftdrop.example/forgot-password"}},
		{"password changed by reset", func(s *Sender) { s.PasswordChanged(ctx, nimali, true, moment) }, "Your SwiftDrop password was changed",
			[]string{"With a password reset link"}},
		{"staff account", func(s *Sender) { s.StaffAccountCreated(ctx, nimali, auth.RoleCourier) }, "Your SwiftDrop account is ready",
			[]string{"Role: Courier", "Log in with: nimali@example.com", "https://app.swiftdrop.example/login"}},
		{"staff administrator", func(s *Sender) { s.StaffAccountCreated(ctx, nimali, auth.RoleAdmin) }, "Your SwiftDrop account is ready",
			[]string{"Role: Administrator"}},
		{"locked", func(s *Sender) { s.AccountLocked(ctx, nimali, 15*time.Minute) }, "Your SwiftDrop account is locked for 15 minutes",
			[]string{"15 minutes", "https://app.swiftdrop.example/forgot-password"}},
		{"deactivated", func(s *Sender) { s.AccountStatusChanged(ctx, nimali, false) }, "Your SwiftDrop account was deactivated",
			[]string{"can no longer log in"}},
		{"reactivated", func(s *Sender) { s.AccountStatusChanged(ctx, nimali, true) }, "Your SwiftDrop account is active again",
			[]string{"https://app.swiftdrop.example/login"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sender, mock, _ := newSender(t, Synchronous())
			test.send(sender)

			sent := mock.Sent()
			if len(sent) != 1 {
				t.Fatalf("%d emails sent, want 1", len(sent))
			}
			message := sent[0]
			if message.To != "nimali@example.com" || message.Subject != test.subject {
				t.Fatalf("to %q subject %q, want %q", message.To, message.Subject, test.subject)
			}
			// The text version wraps long lines, so compare it with its spaces flattened.
			flat := strings.Join(strings.Fields(message.Text), " ")
			for _, want := range test.contains {
				if !strings.Contains(flat, want) {
					t.Errorf("the text lacks %q:\n%s", want, message.Text)
				}
			}
			if message.HTML == "" || len(message.Inline) != 1 {
				t.Fatal("the email must be the branded HTML with its logo")
			}
		})
	}
}

func TestSender_deliversInTheBackgroundAndWaitFinishes(t *testing.T) {
	sender, mock, _ := newSender(t)

	sender.Welcome(context.Background(), nimali)
	sender.AccountLocked(context.Background(), nimali, time.Minute)
	sender.Wait()

	if got := len(mock.Sent()); got != 2 {
		t.Fatalf("%d emails after Wait, want 2", got)
	}
}

func TestSender_aFailedSendIsLoggedWithoutPersonalDetails(t *testing.T) {
	sender, mock, logs := newSender(t, Synchronous())
	mock.FailWith(errors.New("smtp down"))

	// Nothing to recover from and nothing returned: the caller never knows.
	sender.PasswordChanged(context.Background(), nimali, true, moment)

	log := logs.String()
	for _, want := range []string{"account email was not sent", "password_changed", "user-1", "send_failed"} {
		if !strings.Contains(log, want) {
			t.Errorf("the log lacks %q: %s", want, log)
		}
	}
	for _, leak := range []string{"nimali@example.com", "Nimali", "password reset link", "https://", "smtp down"} {
		if strings.Contains(log, leak) {
			t.Errorf("the log leaks %q: %s", leak, log)
		}
	}
}

func TestSender_anUnusableAddressIsLoggedNotSent(t *testing.T) {
	sender, mock, logs := newSender(t, Synchronous())
	sender.Welcome(context.Background(), service.Recipient{UserID: "user-2", Email: "not an address", FullName: "X"})

	if len(mock.Sent()) != 0 {
		t.Fatal("an email was sent to an address that is not valid")
	}
	if !strings.Contains(logs.String(), "account email was not sent") || strings.Contains(logs.String(), "not an address") {
		t.Fatalf("log = %s", logs.String())
	}
}

func TestSender_aSiteWithoutALinkBaseIsLoggedNotSent(t *testing.T) {
	mock := &mailer.Mock{}
	logs := &bytes.Buffer{}
	sender := New(mock, emails.Site{}, slog.New(slog.NewTextHandler(logs, nil)), Synchronous())

	sender.Welcome(context.Background(), nimali)

	if len(mock.Sent()) != 0 || !strings.Contains(logs.String(), "could not be built") {
		t.Fatalf("sent %d, log = %s", len(mock.Sent()), logs.String())
	}
}

func TestSender_aDeliveryOutlivesTheRequest(t *testing.T) {
	sender, mock, _ := newSender(t)
	ctx, cancel := context.WithCancel(context.Background())

	sender.Welcome(ctx, nimali)
	cancel() // the response was sent
	sender.Wait()

	if len(mock.Sent()) != 1 {
		t.Fatal("cancelling the request cancelled the email")
	}
}

func TestRoleLabel(t *testing.T) {
	for role, want := range map[auth.Role]string{
		auth.RoleCourier: "Courier", auth.RoleDispatcher: "Dispatcher", auth.RoleAdmin: "Administrator",
		auth.RoleMerchant: "Merchant", auth.RoleCustomer: "Customer",
	} {
		if got := roleLabel(role); got != want {
			t.Errorf("roleLabel(%s) = %q, want %q", role, got, want)
		}
	}
}
