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
	"github.com/Vineth-R/swiftdrop/packages/go-common/auth"
	"github.com/Vineth-R/swiftdrop/packages/go-common/httpx"
	"github.com/Vineth-R/swiftdrop/packages/go-common/webhookurl"
	"github.com/Vineth-R/swiftdrop/packages/webhookdelivery"
)

const (
	webhookTestSecret = "webhook-test-access-secret-32-bytes!!"
	webhookTestCookie = "sd_access"
	webhookOwnerID    = "u1"
	webhookMerchantID = "m1"
)

// fakeStore keeps one webhook in memory and mimics the repository's atomic
// save result: a secret is reported as stored only on insert or rotation.
type fakeStore struct {
	status       repository.MerchantStatus
	merchantOK   bool
	webhook      *repository.Webhook
	saved        []repository.SaveWebhookParams
	filter       repository.DeliveryFilter
	attempts     []repository.DeliveryAttempt
	total        int
	merchantErr  error
	saveErr      error
	lookupCalled bool
}

func (s *fakeStore) MerchantByOwnerUserID(_ context.Context, ownerUserID string) (repository.Merchant, error) {
	s.lookupCalled = true
	if s.merchantErr != nil {
		return repository.Merchant{}, s.merchantErr
	}
	if !s.merchantOK || ownerUserID != webhookOwnerID {
		return repository.Merchant{}, repository.ErrNotFound
	}
	return repository.Merchant{ID: webhookMerchantID, OwnerUserID: ownerUserID, Status: s.status}, nil
}

func (s *fakeStore) WebhookByMerchant(context.Context, string) (repository.Webhook, error) {
	if s.webhook == nil {
		return repository.Webhook{}, repository.ErrWebhookNotConfigured
	}
	return *s.webhook, nil
}

func (s *fakeStore) SaveWebhook(_ context.Context, p repository.SaveWebhookParams) (repository.WebhookSaveResult, error) {
	s.saved = append(s.saved, p)
	if s.saveErr != nil {
		return repository.WebhookSaveResult{}, s.saveErr
	}
	updatedAt := time.Date(2026, 11, 4, 9, 20, 0, 0, time.UTC)
	if s.webhook == nil {
		w := repository.Webhook{
			ID: p.ID, MerchantID: p.MerchantID, URL: p.URL, EventTypes: p.EventTypes,
			IsActive: p.IsActive, SigningSecretEnc: p.CandidateSecretEnc, CreatedAt: updatedAt, UpdatedAt: updatedAt,
		}
		s.webhook = &w
		return repository.WebhookSaveResult{Webhook: w, SecretStored: true}, nil
	}
	s.webhook.URL = p.URL
	s.webhook.EventTypes = p.EventTypes
	s.webhook.IsActive = p.IsActive
	s.webhook.UpdatedAt = updatedAt
	if p.RotateSecret {
		s.webhook.SigningSecretEnc = p.CandidateSecretEnc
		return repository.WebhookSaveResult{Webhook: *s.webhook, SecretStored: true}, nil
	}
	return repository.WebhookSaveResult{Webhook: *s.webhook, SecretStored: false}, nil
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
	token  string
}

func newWebhookFixture(role string) *webhookFixture {
	return newWebhookFixtureWithOptions(role, webhookTestSecret, fakeCipher{})
}

func newWebhookFixtureWithOptions(role, accessSecret string, cipher secretCipher) *webhookFixture {
	gin.SetMode(gin.TestMode)
	store := &fakeStore{status: repository.MerchantStatusApproved, merchantOK: true}
	sender := &fakeSender{}
	router := httpx.NewRouter(slog.New(slog.NewTextHandler(io.Discard, nil)))

	token := ""
	if accessSecret != "" {
		manager, err := auth.NewTokenManager(accessSecret, time.Hour)
		if err != nil {
			panic(err)
		}
		parsed, ok := auth.ParseRole(role)
		if !ok {
			panic("unknown role " + role)
		}
		token, _, err = manager.Issue(webhookOwnerID, parsed)
		if err != nil {
			panic(err)
		}
	}

	h, err := registerWebhookRoutes(router, store, cipher, sender, accessSecret, webhookTestCookie, nil, nil)
	if err != nil {
		panic(err)
	}
	// Replace the DNS check so tests never touch the network.
	h.checkURL = func(_ context.Context, raw string) error { return webhookurl.Validate(raw) }
	return &webhookFixture{router: router, store: store, sender: sender, token: token}
}

func (f *webhookFixture) call(method, path, body string) (*httptest.ResponseRecorder, map[string]any) {
	request := f.request(method, path, body)
	if f.token != "" {
		request.Header.Set("Authorization", "Bearer "+f.token)
	}
	return f.serve(request)
}

func (f *webhookFixture) callWithoutToken(method, path, body string) (*httptest.ResponseRecorder, map[string]any) {
	return f.serve(f.request(method, path, body))
}

func (f *webhookFixture) request(method, path, body string) *http.Request {
	var reader io.Reader
	if body != "" {
		reader = bytes.NewBufferString(body)
	}
	request := httptest.NewRequest(method, path, reader)
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	return request
}

func (f *webhookFixture) serve(request *http.Request) (*httptest.ResponseRecorder, map[string]any) {
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

func TestWebhookRoutesRequireAuthentication(t *testing.T) {
	f := newWebhookFixture("MERCHANT")
	for _, route := range [][2]string{
		{http.MethodGet, "/merchants/me/webhook"},
		{http.MethodPut, "/merchants/me/webhook"},
		{http.MethodPost, "/merchants/me/webhook/test"},
		{http.MethodGet, "/merchants/me/webhook/deliveries"},
	} {
		recorder, body := f.callWithoutToken(route[0], route[1], `{}`)
		if recorder.Code != http.StatusUnauthorized || webhookErrorCode(body) != "UNAUTHENTICATED" {
			t.Errorf("%s %s = %d %v, want 401 UNAUTHENTICATED", route[0], route[1], recorder.Code, body)
		}
	}
	if f.store.lookupCalled {
		t.Fatal("an unauthenticated request reached the merchant lookup")
	}
}

func TestWebhookRoutesRequireMerchantRole(t *testing.T) {
	f := newWebhookFixture("CUSTOMER")
	for _, route := range [][2]string{
		{http.MethodGet, "/merchants/me/webhook"},
		{http.MethodPut, "/merchants/me/webhook"},
		{http.MethodPost, "/merchants/me/webhook/test"},
		{http.MethodGet, "/merchants/me/webhook/deliveries"},
	} {
		recorder, body := f.call(route[0], route[1], `{}`)
		if recorder.Code != http.StatusForbidden || webhookErrorCode(body) != "FORBIDDEN" {
			t.Errorf("%s %s = %d %v, want 403 FORBIDDEN", route[0], route[1], recorder.Code, body)
		}
	}
	if f.store.lookupCalled {
		t.Fatal("an unauthorized request reached the merchant lookup")
	}
}

func TestWebhookRoutesRejectUnprovisionedMerchant(t *testing.T) {
	f := newWebhookFixture("MERCHANT")
	f.store.merchantOK = false
	recorder, body := f.call(http.MethodGet, "/merchants/me/webhook", "")
	if recorder.Code != http.StatusForbidden || webhookErrorCode(body) != "FORBIDDEN" {
		t.Fatalf("got %d %v, want 403 FORBIDDEN", recorder.Code, body)
	}
}

func TestWebhookRoutesFailClosedWithoutAuthConfig(t *testing.T) {
	f := newWebhookFixtureWithOptions("MERCHANT", "", fakeCipher{})
	for _, route := range [][2]string{
		{http.MethodGet, "/merchants/me/webhook"},
		{http.MethodPut, "/merchants/me/webhook"},
		{http.MethodPost, "/merchants/me/webhook/test"},
		{http.MethodGet, "/merchants/me/webhook/deliveries"},
	} {
		recorder, body := f.callWithoutToken(route[0], route[1], `{}`)
		if recorder.Code != http.StatusServiceUnavailable || webhookErrorCode(body) != "SERVICE_UNAVAILABLE" {
			t.Errorf("%s %s = %d %v, want 503 SERVICE_UNAVAILABLE", route[0], route[1], recorder.Code, body)
		}
	}
}

func TestWebhookRoutesFailClosedWithoutSecretKey(t *testing.T) {
	f := newWebhookFixtureWithOptions("MERCHANT", webhookTestSecret, nil)
	recorder, body := f.call(http.MethodGet, "/merchants/me/webhook", "")
	if recorder.Code != http.StatusServiceUnavailable || webhookErrorCode(body) != "SERVICE_UNAVAILABLE" {
		t.Fatalf("got %d %v, want 503 SERVICE_UNAVAILABLE", recorder.Code, body)
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
	if saved.RotateSecret || len(saved.CandidateSecretEnc) == 0 {
		t.Fatalf("candidate secret was not prepared: %+v", saved)
	}
	if !strings.HasPrefix(string(saved.CandidateSecretEnc), "v1:") {
		t.Fatalf("stored value is not the encrypted format: %q", saved.CandidateSecretEnc)
	}
	if f.store.webhook == nil || string(f.store.webhook.SigningSecretEnc) != string(saved.CandidateSecretEnc) {
		t.Fatal("the content returned was not the secret that was persisted")
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
	first := string(f.store.webhook.SigningSecretEnc)

	_, body := f.call(http.MethodPut, "/merchants/me/webhook", `{"url":"https://shopzone.lk/b","isActive":false}`)
	if _, shown := dataOf(t, body)["signingSecret"]; shown {
		t.Fatal("secret was shown again without rotation")
	}
	if string(f.store.webhook.SigningSecretEnc) != first {
		t.Fatal("secret changed without rotation")
	}
	if f.store.webhook.IsActive {
		t.Fatal("isActive=false was ignored")
	}

	_, rotated := f.call(http.MethodPut, "/merchants/me/webhook", `{"url":"https://shopzone.lk/b","rotateSecret":true}`)
	secret, _ := dataOf(t, rotated)["signingSecret"].(string)
	if !strings.HasPrefix(secret, "whsec_") {
		t.Fatalf("rotation did not return a new secret: %v", rotated)
	}
	if string(f.store.webhook.SigningSecretEnc) == first {
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

func TestSaveReturnsNoSecretWhenStoredSecretIsKept(t *testing.T) {
	f := newWebhookFixture("MERCHANT")
	f.call(http.MethodPut, "/merchants/me/webhook", `{"url":"https://shopzone.lk/a"}`)
	first := string(f.store.webhook.SigningSecretEnc)

	_, body := f.call(http.MethodPut, "/merchants/me/webhook", `{"url":"https://shopzone.lk/b"}`)
	if _, shown := dataOf(t, body)["signingSecret"]; shown {
		t.Fatal("a candidate secret that was not persisted was returned")
	}
	if string(f.store.webhook.SigningSecretEnc) != first {
		t.Fatal("the kept secret changed")
	}
}

func TestSaveFailureDoesNotReturnSecret(t *testing.T) {
	f := newWebhookFixture("MERCHANT")
	f.store.saveErr = errors.New("database unavailable")
	recorder, body := f.call(http.MethodPut, "/merchants/me/webhook", `{"url":"https://shopzone.lk/a"}`)
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d %v", recorder.Code, body)
	}
	if strings.Contains(recorder.Body.String(), "whsec_") {
		t.Fatal("a failed save returned a signing secret")
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
	if got.Success == nil || *got.Success ||
		got.TrackingNumber != "SDK7M2Q9XP4" || got.Limit != 20 || got.Offset != 20 {
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

func TestDeliveriesPaginationBoundaries(t *testing.T) {
	tests := []struct {
		name       string
		query      string
		wantStatus int
		wantOffset int
		wantLimit  int
	}{
		{name: "first page", query: "page=1&pageSize=20", wantStatus: http.StatusOK, wantOffset: 0, wantLimit: 20},
		{name: "last allowed page", query: "page=10000&pageSize=100", wantStatus: http.StatusOK, wantOffset: 999900, wantLimit: 100},
		{name: "page above limit", query: "page=10001", wantStatus: http.StatusUnprocessableEntity},
		{name: "page overflows int", query: "page=999999999999999999999999", wantStatus: http.StatusUnprocessableEntity},
		{name: "zero page", query: "page=0", wantStatus: http.StatusUnprocessableEntity},
		{name: "negative page", query: "page=-1", wantStatus: http.StatusUnprocessableEntity},
		{name: "invalid page", query: "page=abc", wantStatus: http.StatusUnprocessableEntity},
		{name: "zero page size", query: "pageSize=0", wantStatus: http.StatusUnprocessableEntity},
		{name: "negative page size", query: "pageSize=-1", wantStatus: http.StatusUnprocessableEntity},
		{name: "invalid page size", query: "pageSize=abc", wantStatus: http.StatusUnprocessableEntity},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newWebhookFixture("MERCHANT")
			recorder, body := f.call(http.MethodGet, "/merchants/me/webhook/deliveries?"+tc.query, "")
			if recorder.Code != tc.wantStatus {
				t.Fatalf("status = %d %v, want %d", recorder.Code, body, tc.wantStatus)
			}
			if tc.wantStatus != http.StatusOK {
				if webhookErrorCode(body) != "VALIDATION_ERROR" {
					t.Fatalf("error code = %q, want VALIDATION_ERROR", webhookErrorCode(body))
				}
				if f.store.filter.Offset != 0 || f.store.filter.Limit != 0 {
					t.Fatalf("an invalid request reached the store: %+v", f.store.filter)
				}
				return
			}
			if f.store.filter.Offset != tc.wantOffset || f.store.filter.Limit != tc.wantLimit {
				t.Fatalf("filter = %+v, want offset %d limit %d", f.store.filter, tc.wantOffset, tc.wantLimit)
			}
		})
	}
}

func TestDeliveriesRejectsBadQuery(t *testing.T) {
	for _, query := range []string{"success=maybe", "trackingNumber=" + strings.Repeat("A", 21)} {
		f := newWebhookFixture("MERCHANT")
		recorder, body := f.call(http.MethodGet, "/merchants/me/webhook/deliveries?"+query, "")
		if recorder.Code != http.StatusUnprocessableEntity || webhookErrorCode(body) != "VALIDATION_ERROR" {
			t.Errorf("%s: got %d %v", query, recorder.Code, body)
		}
	}
}
