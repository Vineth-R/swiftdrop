package servicehttp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Vineth-R/swiftdrop/apps/services/merchant/internal/repository"
	"github.com/Vineth-R/swiftdrop/packages/go-common/httpx"
	"github.com/Vineth-R/swiftdrop/packages/go-common/webhookurl"
	"github.com/Vineth-R/swiftdrop/packages/webhookdelivery"
)

// fakeStore keeps one webhook in memory.
type fakeStore struct {
	status   repository.MerchantStatus
	webhook  *repository.Webhook
	saved    []repository.SaveWebhookParams
	filter   repository.DeliveryFilter
	attempts []repository.DeliveryAttempt
	total    int
	failWith error
}

func (s *fakeStore) MerchantByID(context.Context, string) (repository.Merchant, error) {
	return repository.Merchant{Status: s.status}, s.failWith
}

func (s *fakeStore) WebhookByMerchant(context.Context, string) (repository.Webhook, error) {
	if s.webhook == nil {
		return repository.Webhook{}, repository.ErrWebhookNotConfigured
	}
	return *s.webhook, nil
}

func (s *fakeStore) SaveWebhook(_ context.Context, p repository.SaveWebhookParams) (repository.Webhook, error) {
	s.saved = append(s.saved, p)
	w := repository.Webhook{ID: p.ID, URL: p.URL, EventTypes: p.EventTypes, IsActive: p.IsActive,
		SigningSecretEnc: p.SecretEnc, UpdatedAt: time.Date(2026, 11, 4, 9, 20, 0, 0, time.UTC)}
	s.webhook = &w
	return w, nil
}

func (s *fakeStore) ListDeliveryAttempts(_ context.Context, _ string, f repository.DeliveryFilter) ([]repository.DeliveryAttempt, int, error) {
	s.filter = f
	return s.attempts, s.total, nil
}

// fakeCipher is reversible and visibly different from the plain text.
type fakeCipher struct{}

func (fakeCipher) Encrypt(s string) (string, error) { return "v1:" + s, nil }
func (fakeCipher) Decrypt(s string) (string, error) {
	if !strings.HasPrefix(s, "v1:") {
		return "", errors.New("bad data")
	}
	return strings.TrimPrefix(s, "v1:"), nil
}

type fakeSender struct {
	result webhookdelivery.Result
	target webhookdelivery.Target
	event  webhookdelivery.Event
}

func (s *fakeSender) Send(_ context.Context, t webhookdelivery.Target, e webhookdelivery.Event) webhookdelivery.Result {
	s.target, s.event = t, e
	return s.result
}

type webhookFixture struct {
	router *gin.Engine
	store  *fakeStore
	sender *fakeSender
}

func newWebhookFixture(role string) *webhookFixture {
	gin.SetMode(gin.TestMode)
	store := &fakeStore{status: repository.MerchantStatusApproved}
	sender := &fakeSender{}
	router := httpx.NewRouter(slog.New(slog.NewTextHandler(io.Discard, nil)))
	router.Use(func(c *gin.Context) {
		WithIdentity(c, Identity{UserID: "u1", MerchantID: "m1", Role: role})
		c.Next()
	})
	h := registerWebhookRoutes(router, store, fakeCipher{}, sender, nil)
	// Replace the DNS check so tests never touch the network.
	h.checkURL = func(_ context.Context, raw string) error { return webhookurl.Validate(raw) }
	return &webhookFixture{router: router, store: store, sender: sender}
}

func (f *webhookFixture) call(method, path, body string) (*httptest.ResponseRecorder, map[string]any) {
	var reader io.Reader
	if body != "" {
		reader = bytes.NewBufferString(body)
	}
	request := httptest.NewRequest(method, path, reader)
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	recorder := httptest.NewRecorder()
	f.router.ServeHTTP(recorder, request)
	var decoded map[string]any
	_ = json.Unmarshal(recorder.Body.Bytes(), &decoded)
	return recorder, decoded
}

func dataOf(t *testing.T, body map[string]any) map[string]any {
	t.Helper()
	data, ok := body["data"].(map[string]any)
	if !ok {
		t.Fatalf("response has no data object: %v", body)
	}
	return data
}

func webhookErrorCode(body map[string]any) string {
	detail, _ := body["error"].(map[string]any)
	code, _ := detail["code"].(string)
	return code
}

func TestWebhookRoutesRequireMerchantRole(t *testing.T) {
	f := newWebhookFixture("CUSTOMER")
	for _, route := range [][2]string{
		{http.MethodGet, "/merchants/me/webhook"},
		{http.MethodPut, "/merchants/me/webhook"},
		{http.MethodPost, "/merchants/me/webhook/test"},
		{http.MethodGet, "/merchants/me/webhook/deliveries"},
	} {
		recorder, _ := f.call(route[0], route[1], `{}`)
		if recorder.Code != http.StatusUnauthorized {
			t.Errorf("%s %s = %d, want 401", route[0], route[1], recorder.Code)
		}
	}
}

func TestGetWebhookNotConfigured(t *testing.T) {
	f := newWebhookFixture("MERCHANT")
	recorder, body := f.call(http.MethodGet, "/merchants/me/webhook", "")
	if recorder.Code != http.StatusNotFound || webhookErrorCode(body) != "WEBHOOK_NOT_CONFIGURED" {
		t.Fatalf("got %d %v", recorder.Code, body)
	}
}

func TestFirstSaveReturnsSecretOnceAndStoresItEncrypted(t *testing.T) {
	f := newWebhookFixture("MERCHANT")
	recorder, body := f.call(http.MethodPut, "/merchants/me/webhook",
		`{"url":"https://shopzone.lk/hooks/swiftdrop","eventTypes":["parcel.status_changed"],"isActive":true}`)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body %v", recorder.Code, body)
	}
	secret, _ := dataOf(t, body)["signingSecret"].(string)
	if !strings.HasPrefix(secret, "whsec_") {
		t.Fatalf("signingSecret = %q, want whsec_ prefix", secret)
	}
	saved := f.store.saved[0]
	if !saved.ReplaceSecret || len(saved.SecretEnc) == 0 {
		t.Fatalf("secret was not passed for storage: %+v", saved)
	}
	if !strings.HasPrefix(string(saved.SecretEnc), "v1:") {
		t.Fatalf("stored value is not the encrypted format: %q", saved.SecretEnc)
	}

	// Reading it back must not include the secret.
	_, got := f.call(http.MethodGet, "/merchants/me/webhook", "")
	if _, leaked := dataOf(t, got)["signingSecret"]; leaked {
		t.Fatal("GET returned the signing secret")
	}
}

func TestSecondSaveKeepsSecretUnlessRotated(t *testing.T) {
	f := newWebhookFixture("MERCHANT")
	f.call(http.MethodPut, "/merchants/me/webhook", `{"url":"https://shopzone.lk/a"}`)
	first := string(f.store.saved[0].SecretEnc)

	_, body := f.call(http.MethodPut, "/merchants/me/webhook", `{"url":"https://shopzone.lk/b","isActive":false}`)
	if _, shown := dataOf(t, body)["signingSecret"]; shown {
		t.Fatal("secret was shown again without rotation")
	}
	second := f.store.saved[1]
	if second.ReplaceSecret || string(second.SecretEnc) != first {
		t.Fatalf("secret changed without rotation: %+v", second)
	}
	if second.IsActive {
		t.Fatal("isActive=false was ignored")
	}

	_, rotated := f.call(http.MethodPut, "/merchants/me/webhook", `{"url":"https://shopzone.lk/b","rotateSecret":true}`)
	if secret, _ := dataOf(t, rotated)["signingSecret"].(string); !strings.HasPrefix(secret, "whsec_") {
		t.Fatalf("rotation did not return a new secret: %v", rotated)
	}
	if !f.store.saved[2].ReplaceSecret || string(f.store.saved[2].SecretEnc) == first {
		t.Fatal("rotation did not replace the stored secret")
	}
}

func TestSaveDefaultsToStatusChangedAndActive(t *testing.T) {
	f := newWebhookFixture("MERCHANT")
	f.call(http.MethodPut, "/merchants/me/webhook", `{"url":"https://shopzone.lk/a"}`)
	saved := f.store.saved[0]
	if !saved.IsActive || len(saved.EventTypes) != 1 || saved.EventTypes[0] != "parcel.status_changed" {
		t.Fatalf("defaults not applied: %+v", saved)
	}
}

func TestSaveRejectsUnapprovedMerchant(t *testing.T) {
	f := newWebhookFixture("MERCHANT")
	f.store.status = repository.MerchantStatus("PENDING")
	recorder, body := f.call(http.MethodPut, "/merchants/me/webhook", `{"url":"https://shopzone.lk/a"}`)
	if recorder.Code != http.StatusForbidden || webhookErrorCode(body) != "MERCHANT_NOT_APPROVED" {
		t.Fatalf("got %d %v", recorder.Code, body)
	}
	if len(f.store.saved) != 0 {
		t.Fatal("an unapproved merchant saved a webhook")
	}
}

func TestSaveRejectsBadInput(t *testing.T) {
	cases := []struct {
		name, body, code string
	}{
		{"http url", `{"url":"http://shopzone.lk/a"}`, "WEBHOOK_URL_NOT_ALLOWED"},
		{"loopback", `{"url":"https://127.0.0.1/a"}`, "WEBHOOK_URL_NOT_ALLOWED"},
		{"private address", `{"url":"https://10.0.0.5/a"}`, "WEBHOOK_URL_NOT_ALLOWED"},
		{"localhost", `{"url":"https://localhost/a"}`, "WEBHOOK_URL_NOT_ALLOWED"},
		{"credentials", `{"url":"https://user:pw@shopzone.lk/a"}`, "WEBHOOK_URL_NOT_ALLOWED"},
		{"empty", `{"url":""}`, "VALIDATION_ERROR"},
		{"missing", `{}`, "VALIDATION_ERROR"},
		{"not json", `nope`, "VALIDATION_ERROR"},
		{"unknown event", `{"url":"https://shopzone.lk/a","eventTypes":["parcel.exploded"]}`, "VALIDATION_ERROR"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newWebhookFixture("MERCHANT")
			recorder, body := f.call(http.MethodPut, "/merchants/me/webhook", tc.body)
			if recorder.Code != http.StatusUnprocessableEntity || webhookErrorCode(body) != tc.code {
				t.Fatalf("got %d %v, want 422 %s", recorder.Code, body, tc.code)
			}
			if len(f.store.saved) != 0 {
				t.Fatal("a rejected request was saved")
			}
		})
	}
}

func TestTestSendReportsDelivery(t *testing.T) {
	f := newWebhookFixture("MERCHANT")
	f.call(http.MethodPut, "/merchants/me/webhook", `{"url":"https://shopzone.lk/a"}`)
	f.sender.result = webhookdelivery.Result{StatusCode: 200, Success: true, Duration: 182 * time.Millisecond}

	recorder, body := f.call(http.MethodPost, "/merchants/me/webhook/test", "")
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d", recorder.Code)
	}
	data := dataOf(t, body)
	if data["delivered"] != true || data["httpStatus"] != float64(200) || data["durationMs"] != float64(182) {
		t.Fatalf("unexpected result: %v", data)
	}
	if f.sender.target.URL != "https://shopzone.lk/a" || !strings.HasPrefix(f.sender.target.Secret, "whsec_") {
		t.Fatalf("sender got the wrong target: %+v", f.sender.target)
	}
	if f.sender.event.Type != webhookdelivery.EventTypeStatusChanged {
		t.Fatalf("sample event type = %q", f.sender.event.Type)
	}
}

func TestTestSendFailureIsStill200WithNullStatus(t *testing.T) {
	f := newWebhookFixture("MERCHANT")
	f.call(http.MethodPut, "/merchants/me/webhook", `{"url":"https://shopzone.lk/a"}`)
	f.sender.result = webhookdelivery.Result{StatusCode: 0, Success: false, Duration: 10 * time.Second, ErrorMessage: "request timed out"}

	recorder, body := f.call(http.MethodPost, "/merchants/me/webhook/test", "")
	data := dataOf(t, body)
	if recorder.Code != http.StatusOK || data["delivered"] != false {
		t.Fatalf("got %d %v", recorder.Code, data)
	}
	if status, present := data["httpStatus"]; !present || status != nil {
		t.Fatalf("httpStatus = %v, want explicit null", status)
	}
	if strings.Contains(recorder.Body.String(), "timed out") {
		t.Fatal("the internal error message leaked into the response")
	}
}

func TestTestSendNotConfigured(t *testing.T) {
	f := newWebhookFixture("MERCHANT")
	recorder, body := f.call(http.MethodPost, "/merchants/me/webhook/test", "")
	if recorder.Code != http.StatusNotFound || webhookErrorCode(body) != "WEBHOOK_NOT_CONFIGURED" {
		t.Fatalf("got %d %v", recorder.Code, body)
	}
}

func TestDeliveriesFiltersAndPaging(t *testing.T) {
	f := newWebhookFixture("MERCHANT")
	status := 503
	retry := time.Date(2026, 11, 4, 12, 35, 0, 0, time.UTC)
	f.store.total = 41
	f.store.attempts = []repository.DeliveryAttempt{{
		EventID: "ev12", TrackingNumber: "SDK7M2Q9XP4", AttemptNumber: 2, HTTPStatus: &status,
		NextRetryAt: &retry, CreatedAt: time.Date(2026, 11, 4, 12, 30, 0, 0, time.UTC),
	}}

	recorder, body := f.call(http.MethodGet,
		"/merchants/me/webhook/deliveries?success=false&trackingNumber=SDK7M2Q9XP4&page=2&pageSize=20", "")
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d %v", recorder.Code, body)
	}
	got := f.store.filter
	if got.Success == nil || *got.Success || got.TrackingNumber != "SDK7M2Q9XP4" || got.Limit != 20 || got.Offset != 20 {
		t.Fatalf("filter = %+v", got)
	}
	rows, _ := body["data"].([]any)
	if len(rows) != 1 {
		t.Fatalf("rows = %v", body["data"])
	}
	row := rows[0].(map[string]any)
	if row["eventId"] != "ev12" || row["attemptNumber"] != float64(2) || row["httpStatus"] != float64(503) || row["deadLettered"] != false {
		t.Fatalf("row = %v", row)
	}
	pagination := body["meta"].(map[string]any)["pagination"].(map[string]any)
	if pagination["totalItems"] != float64(41) || pagination["totalPages"] != float64(3) {
		t.Fatalf("pagination = %v", pagination)
	}
}

func TestDeliveriesDefaultsAndLimits(t *testing.T) {
	f := newWebhookFixture("MERCHANT")
	f.call(http.MethodGet, "/merchants/me/webhook/deliveries", "")
	if got := f.store.filter; got.Success != nil || got.Limit != 20 || got.Offset != 0 {
		t.Fatalf("defaults = %+v", got)
	}
	f.call(http.MethodGet, "/merchants/me/webhook/deliveries?pageSize=500", "")
	if f.store.filter.Limit != 100 {
		t.Fatalf("pageSize was not capped: %d", f.store.filter.Limit)
	}
}

func TestDeliveriesRejectsBadQuery(t *testing.T) {
	for _, query := range []string{"success=maybe", "page=0", "page=abc", "pageSize=-1"} {
		f := newWebhookFixture("MERCHANT")
		recorder, body := f.call(http.MethodGet, "/merchants/me/webhook/deliveries?"+query, "")
		if recorder.Code != http.StatusUnprocessableEntity || webhookErrorCode(body) != "VALIDATION_ERROR" {
			t.Errorf("%s: got %d %v", query, recorder.Code, body)
		}
	}
}
