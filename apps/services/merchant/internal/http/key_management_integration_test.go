//go:build integration

package servicehttp

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Vineth-R/swiftdrop/apps/services/merchant/internal/apikey"
	"github.com/Vineth-R/swiftdrop/apps/services/merchant/internal/notify"
	"github.com/Vineth-R/swiftdrop/packages/go-common/auth"
	"github.com/Vineth-R/swiftdrop/packages/go-common/emails"
	"github.com/Vineth-R/swiftdrop/packages/go-common/mailer"
)

func TestCreateKeyReturnsCreatedAtAndKeepsSecretOutOfEveryOtherSurface(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	sender := &mailer.Mock{}
	notifier := notify.New(sender, emails.Site{BaseURL: "https://swiftdrop.test", SupportEmail: "support@swiftdrop.test"}, logger)
	ctx, repo, pool, router, _ := newOrderHTTPFixtureWith(t, logger, notifier)

	merchantID, ownerID := createHTTPMerchant(t, ctx, repo)
	approve(t, ctx, repo, merchantID)
	merchant, err := repo.MerchantByID(ctx, merchantID)
	if err != nil {
		t.Fatalf("MerchantByID() error = %v", err)
	}
	token := merchantToken(t, ownerID)

	status, body := requestJSON(t, router, http.MethodPost, "/merchants/me/keys",
		map[string]any{"name": "Production"}, map[string]string{"Authorization": "Bearer " + token})
	if status != http.StatusCreated {
		t.Fatalf("create key status = %d (%s)", status, body)
	}
	var created struct {
		Data struct {
			ID        string    `json:"id"`
			Name      string    `json:"name"`
			APIKey    string    `json:"apiKey"`
			KeyPrefix string    `json:"keyPrefix"`
			CreatedAt time.Time `json:"createdAt"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &created); err != nil {
		t.Fatalf("decode create key: %v (%s)", err, body)
	}
	if created.Data.ID == "" || created.Data.APIKey == "" || created.Data.KeyPrefix == "" {
		t.Fatalf("create key body = %s", body)
	}
	if created.Data.CreatedAt.IsZero() {
		t.Fatalf("create key response omitted createdAt: %s", body)
	}
	if !strings.HasPrefix(created.Data.APIKey, apikey.Prefix) {
		t.Fatalf("created key = %q", created.Data.APIKey)
	}

	var storedHash string
	if err := pool.QueryRow(ctx,
		`SELECT key_hash FROM merchant.merchant_api_clients WHERE id = $1`, created.Data.ID,
	).Scan(&storedHash); err != nil {
		t.Fatalf("read stored key hash: %v", err)
	}
	if storedHash != apikey.Hash(created.Data.APIKey) {
		t.Fatalf("stored hash does not match the issued secret")
	}
	if storedHash == created.Data.APIKey {
		t.Fatal("the full secret was stored instead of its hash")
	}

	secret := created.Data.APIKey
	if strings.Contains(logs.String(), secret) {
		t.Fatalf("application log leaked the API key: %s", logs.String())
	}
	var auditMatches int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM merchant.audit_logs WHERE details::text LIKE '%' || $1 || '%'`, secret,
	).Scan(&auditMatches); err != nil {
		t.Fatalf("count audit matches: %v", err)
	}
	if auditMatches != 0 {
		t.Fatalf("audit records contain the API key")
	}

	status, body = requestJSON(t, router, http.MethodGet, "/merchants/me/keys", nil,
		map[string]string{"Authorization": "Bearer " + token})
	if status != http.StatusOK || bytes.Contains(body, []byte(secret)) {
		t.Fatalf("key list leaked the secret or failed: status %d body %s", status, body)
	}

	notifier.Wait()
	sent := sender.Sent()
	if len(sent) != 1 {
		t.Fatalf("key emails = %d, want 1", len(sent))
	}
	message := sent[0]
	if message.To != merchant.ContactEmail {
		t.Fatalf("key email recipient = %q, want %q", message.To, merchant.ContactEmail)
	}
	if !strings.Contains(message.Text, created.Data.KeyPrefix) {
		t.Fatalf("key email did not show the prefix:\n%s", message.Text)
	}
	if strings.Contains(message.Text, secret) {
		t.Fatalf("key email leaked the full secret:\n%s", message.Text)
	}

	status, body = requestJSON(t, router, http.MethodDelete, "/merchants/me/keys/"+created.Data.ID, nil,
		map[string]string{"Authorization": "Bearer " + token})
	if status != http.StatusNoContent {
		t.Fatalf("revoke status = %d (%s)", status, body)
	}
	status, body = requestJSON(t, router, http.MethodGet, "/merchant/orders", nil,
		map[string]string{"X-API-Key": secret})
	if status != http.StatusUnauthorized || !bytes.Contains(body, []byte("INVALID_API_KEY")) {
		t.Fatalf("revoked key status = %d (%s)", status, body)
	}
	if bytes.Contains(body, []byte(secret)) {
		t.Fatalf("revoked-key error leaked the secret: %s", body)
	}
}

func TestKeyLimitAndUnapprovedMerchantOverHTTP(t *testing.T) {
	ctx, repo, _, router, _ := newOrderHTTPFixture(t)

	_, pendingOwner := createHTTPMerchant(t, ctx, repo)
	status, body := requestJSON(t, router, http.MethodPost, "/merchants/me/keys",
		map[string]any{"name": "Pending"}, map[string]string{"Authorization": "Bearer " + merchantToken(t, pendingOwner)})
	if status != http.StatusForbidden || errorCode(body) != "MERCHANT_NOT_APPROVED" {
		t.Fatalf("pending key status = %d (%s)", status, body)
	}

	approvedID, ownerID := createHTTPMerchant(t, ctx, repo)
	approve(t, ctx, repo, approvedID)
	token := merchantToken(t, ownerID)
	secrets := make([]string, 0, 5)
	for i := 0; i < 5; i++ {
		status, body = requestJSON(t, router, http.MethodPost, "/merchants/me/keys",
			map[string]any{"name": "Key " + string(rune('A'+i))}, map[string]string{"Authorization": "Bearer " + token})
		if status != http.StatusCreated {
			t.Fatalf("key %d status = %d (%s)", i+1, status, body)
		}
		var created struct {
			Data struct {
				APIKey string `json:"apiKey"`
			} `json:"data"`
		}
		_ = json.Unmarshal(body, &created)
		secrets = append(secrets, created.Data.APIKey)
	}

	status, body = requestJSON(t, router, http.MethodPost, "/merchants/me/keys",
		map[string]any{"name": "Sixth"}, map[string]string{"Authorization": "Bearer " + token})
	if status != http.StatusConflict || errorCode(body) != "KEY_LIMIT_REACHED" {
		t.Fatalf("sixth key status = %d (%s)", status, body)
	}
	for _, secret := range secrets {
		if bytes.Contains(body, []byte(secret)) {
			t.Fatalf("limit error leaked a secret: %s", body)
		}
	}
}

func TestConcurrentKeyCreationCannotExceedTheLimitOverHTTP(t *testing.T) {
	ctx, repo, _, router, _ := newOrderHTTPFixture(t)
	merchantID, ownerID := createHTTPMerchant(t, ctx, repo)
	approve(t, ctx, repo, merchantID)
	token := merchantToken(t, ownerID)

	var wg sync.WaitGroup
	statuses := make(chan int, 10)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			status, _ := requestJSON(t, router, http.MethodPost, "/merchants/me/keys",
				map[string]any{"name": "Concurrent " + string(rune('A'+index))},
				map[string]string{"Authorization": "Bearer " + token})
			statuses <- status
		}(i)
	}
	wg.Wait()
	close(statuses)
	created, limited := 0, 0
	for status := range statuses {
		switch status {
		case http.StatusCreated:
			created++
		case http.StatusConflict:
			limited++
		default:
			t.Fatalf("concurrent key status = %d", status)
		}
	}
	if created != 5 || limited != 5 {
		t.Fatalf("created=%d limited=%d, want 5/5", created, limited)
	}
}

func merchantToken(t *testing.T, ownerID string) string {
	t.Helper()
	manager, err := auth.NewTokenManager(testJWTSecret, time.Minute)
	if err != nil {
		t.Fatalf("NewTokenManager() error = %v", err)
	}
	token, _, err := manager.IssueWithSession(ownerID, auth.RoleMerchant, 1)
	if err != nil {
		t.Fatalf("IssueWithSession() error = %v", err)
	}
	return token
}

func adminToken(t *testing.T, secret string) string {
	t.Helper()
	manager, err := auth.NewTokenManager(secret, time.Minute)
	if err != nil {
		t.Fatalf("NewTokenManager() error = %v", err)
	}
	token, _, err := manager.Issue(userID(t), auth.RoleAdmin)
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}
	return token
}
