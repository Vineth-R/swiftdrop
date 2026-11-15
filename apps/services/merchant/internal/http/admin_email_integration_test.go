//go:build integration

package servicehttp

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Vineth-R/swiftdrop/apps/services/merchant/internal/notify"
	"github.com/Vineth-R/swiftdrop/apps/services/merchant/internal/repository"
	"github.com/Vineth-R/swiftdrop/packages/go-common/auth"
	"github.com/Vineth-R/swiftdrop/packages/go-common/emails"
	"github.com/Vineth-R/swiftdrop/packages/go-common/mailer"
)

func TestAdminMerchantEmailSendsAndAudits(t *testing.T) {
	ctx, pool, repo := newWebhookTestPool(t)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	sender := &mailer.Mock{}
	notifier := notify.New(sender, emails.Site{BaseURL: "https://swiftdrop.test"}, logger)
	const secret = "admin-email-test-secret-0123456789abcdef"
	router := NewWithOptions(Options{
		Repository: repo, Notifier: notifier, AccessSecret: secret, Logger: logger,
	})
	manager, err := auth.NewTokenManager(secret, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	adminToken, _, err := manager.Issue(userID(t), auth.RoleAdmin)
	if err != nil {
		t.Fatal(err)
	}
	customerToken, _, err := manager.Issue(userID(t), auth.RoleCustomer)
	if err != nil {
		t.Fatal(err)
	}

	merchantID := userID(t)
	contact := "merchant-email@example.test"
	if err := repo.CreateMerchant(ctx, repository.CreateMerchantParams{
		ID: merchantID, OwnerUserID: userID(t), BusinessName: "Email Fixture",
		BusinessRegNo: fixtureBusinessRegNo("EML", merchantID), ContactEmail: contact,
		ContactPhone: "+94771234567", PickupAddress: "12 Galle Road", PickupZoneID: userID(t),
	}); err != nil {
		t.Fatal(err)
	}

	post := func(token, body string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost, "/admin/merchants/"+merchantID+"/email", strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Authorization", "Bearer "+token)
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		return recorder
	}

	body := `{"subject":"Missing documents","message":"Please send the registration document."}`
	if response := post(adminToken, body); response.Code != http.StatusOK {
		t.Fatalf("status = %d (%s)", response.Code, response.Body)
	}
	sent := sender.Sent()
	if len(sent) != 1 || sent[0].To != contact || sent[0].Subject != "Missing documents" {
		t.Fatalf("sent = %+v", sent)
	}

	var action string
	var details string
	if err := pool.QueryRow(ctx, `
		SELECT action, details::text FROM merchant.audit_logs
		WHERE entity_id = $1 AND action = 'ADMIN_EMAIL_SENT'
		ORDER BY created_at DESC LIMIT 1`, merchantID,
	).Scan(&action, &details); err != nil {
		t.Fatalf("audit row: %v", err)
	}
	if action != "ADMIN_EMAIL_SENT" || !strings.Contains(details, "Missing documents") {
		t.Fatalf("audit = %s %s", action, details)
	}
	for _, leaked := range []string{contact, "Please send the registration document."} {
		if strings.Contains(details, leaked) {
			t.Fatalf("audit details leaked %q: %s", leaked, details)
		}
	}

	if response := post(adminToken, `{"subject":"","message":""}`); response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("empty body status = %d (%s)", response.Code, response.Body)
	}
	if response := post(customerToken, body); response.Code != http.StatusForbidden {
		t.Fatalf("non-admin status = %d (%s)", response.Code, response.Body)
	}
}
