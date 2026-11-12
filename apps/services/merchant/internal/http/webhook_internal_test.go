package servicehttp

import (
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
)

const (
	internalTestToken    = "internal-test-token-32-bytes-long!!"
	internalTestMerchant = "5f0b2c0e-6c53-4f6e-9a38-0f4b1f0e7a11"
	internalTestEvent    = "e1a2b3c4-5c6d-4e7f-8a9b-0c1d2e3f4a5b"
	internalTestParcel   = "e5a1b7c2-6d3f-4a8b-9c10-2d3e4f5a6b7c"
)

type fakeInternalStore struct {
	webhook     *repository.Webhook
	merchant    repository.Merchant
	recorded    []repository.RecordAttemptParams
	duplicate   bool
	getErr      error
	merchantErr error
	recordErr   error
	merchantIDs []string
}

func (s *fakeInternalStore) WebhookByMerchant(_ context.Context, merchantID string) (repository.Webhook, error) {
	s.merchantIDs = append(s.merchantIDs, merchantID)
	if s.getErr != nil {
		return repository.Webhook{}, s.getErr
	}
	if s.webhook == nil {
		return repository.Webhook{}, repository.ErrWebhookNotConfigured
	}
	return *s.webhook, nil
}

func (s *fakeInternalStore) MerchantByID(_ context.Context, merchantID string) (repository.Merchant, error) {
	if s.merchantErr != nil {
		return repository.Merchant{}, s.merchantErr
	}
	if s.merchant.ID == "" {
		return repository.Merchant{ID: merchantID, Status: repository.MerchantStatusApproved}, nil
	}
	return s.merchant, nil
}

func (s *fakeInternalStore) RecordDeliveryAttempt(_ context.Context, p repository.RecordAttemptParams) (bool, error) {
	if s.recordErr != nil {
		return false, s.recordErr
	}
	s.recorded = append(s.recorded, p)
	return !s.duplicate, nil
}

type internalFixture struct {
	router *gin.Engine
	store  *fakeInternalStore
}

func newInternalFixture(token string, withStore, withCipher bool) *internalFixture {
	gin.SetMode(gin.TestMode)
	store := &fakeInternalStore{
		webhook: &repository.Webhook{
			ID: "w1", MerchantID: internalTestMerchant, URL: "https://shopzone.lk/hooks/swiftdrop",
			SigningSecretEnc: []byte("v1:whsec_real_secret"), EventTypes: []string{"parcel.status_changed"}, IsActive: true,
		},
		merchant: repository.Merchant{ID: internalTestMerchant, Status: repository.MerchantStatusApproved},
	}
	router := httpx.NewRouter(slog.New(slog.NewTextHandler(io.Discard, nil)))
	var s internalWebhookStore
	if withStore {
		s = store
	}
	var c secretCipher
	if withCipher {
		c = fakeCipher{}
	}
	registerInternalWebhookRoutes(router, s, c, token, slog.New(slog.NewTextHandler(io.Discard, nil)))
	return &internalFixture{router: router, store: store}
}

func (f *internalFixture) do(method, path, token, requestID, body string) *httptest.ResponseRecorder {
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	request := httptest.NewRequest(method, path, reader)
	if token != "" {
		request.Header.Set("X-Internal-Token", token)
	}
	if requestID != "" {
		request.Header.Set("X-Request-Id", requestID)
	}
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	recorder := httptest.NewRecorder()
	f.router.ServeHTTP(recorder, request)
	return recorder
}

func (f *internalFixture) get(path string) *httptest.ResponseRecorder {
	return f.do(http.MethodGet, path, internalTestToken, "req-1", "")
}

func (f *internalFixture) post(path, body string) *httptest.ResponseRecorder {
	return f.do(http.MethodPost, path, internalTestToken, "req-1", body)
}

func webhookPath() string  { return "/internal/merchants/" + internalTestMerchant + "/webhook" }
func attemptsPath() string { return webhookPath() + "/attempts" }

func decodeBody(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not JSON: %v (%s)", err, recorder.Body.String())
	}
	return body
}

func errorCodeOf(body map[string]any) string {
	detail, _ := body["error"].(map[string]any)
	code, _ := detail["code"].(string)
	return code
}

func validAttemptBody() string {
	return `{"eventId":"` + internalTestEvent + `","parcelId":"` + internalTestParcel + `",` +
		`"trackingNumber":"SDN4T8W2LC6","attemptNumber":2,"httpStatus":503,"success":false,` +
		`"errorMessage":"upstream unavailable","nextRetryAt":"2026-11-04T12:35:00Z","durationMs":182}`
}

func TestInternalWebhookRefusesCallsWithoutTheToken(t *testing.T) {
	fixture := newInternalFixture(internalTestToken, true, true)
	cases := []struct {
		name  string
		token string
	}{
		{"no token", ""},
		{"wrong token", "wrong-token-for-the-internal-test"},
		{"almost right token", internalTestToken + "x"},
	}
	for _, c := range cases {
		for _, call := range []struct{ method, path string }{
			{http.MethodGet, webhookPath()},
			{http.MethodPost, attemptsPath()},
		} {
			recorder := fixture.do(call.method, call.path, c.token, "req-1", validAttemptBody())
			if recorder.Code != http.StatusUnauthorized || errorCodeOf(decodeBody(t, recorder)) != "UNAUTHENTICATED" {
				t.Errorf("%s %s (%s): status = %d body = %s, want 401 UNAUTHENTICATED", call.method, call.path, c.name, recorder.Code, recorder.Body.String())
			}
		}
	}
	if len(fixture.store.merchantIDs) != 0 || len(fixture.store.recorded) != 0 {
		t.Fatal("the store was reached without a valid token")
	}
}

func TestInternalWebhookWithoutConfiguredTokenRefusesEverything(t *testing.T) {
	fixture := newInternalFixture("", true, true)
	// Even a request that sends an empty token header must be refused.
	for _, token := range []string{"", internalTestToken} {
		recorder := fixture.do(http.MethodGet, webhookPath(), token, "req-1", "")
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("token %q: status = %d, want 401", token, recorder.Code)
		}
	}
}

func TestInternalWebhookRequiresAWellFormedRequestID(t *testing.T) {
	fixture := newInternalFixture(internalTestToken, true, true)
	for _, id := range []string{"", "bad id with spaces", "id/with/slashes", strings.Repeat("a", 129)} {
		recorder := fixture.do(http.MethodGet, webhookPath(), internalTestToken, id, "")
		if recorder.Code != http.StatusBadRequest || errorCodeOf(decodeBody(t, recorder)) != "BAD_REQUEST" {
			t.Errorf("request id %q: status = %d body = %s, want 400 BAD_REQUEST", id, recorder.Code, recorder.Body.String())
		}
	}
}

func TestInternalWebhookTokenIsCheckedBeforeRequestID(t *testing.T) {
	fixture := newInternalFixture(internalTestToken, true, true)
	recorder := fixture.do(http.MethodGet, webhookPath(), "wrong-token-value-000000", "", "")
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 so a stranger learns nothing else", recorder.Code)
	}
}

func TestInternalWebhookGetReturnsTheDecryptedSecret(t *testing.T) {
	fixture := newInternalFixture(internalTestToken, true, true)
	recorder := fixture.get(webhookPath())
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
	}
	if got := recorder.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", got)
	}
	data, _ := decodeBody(t, recorder)["data"].(map[string]any)
	if data["url"] != "https://shopzone.lk/hooks/swiftdrop" || data["signingSecret"] != "whsec_real_secret" || data["isActive"] != true || data["merchantStatus"] != "APPROVED" {
		t.Fatalf("data = %v", data)
	}
	types, _ := data["eventTypes"].([]any)
	if len(types) != 1 || types[0] != "parcel.status_changed" {
		t.Fatalf("eventTypes = %v", data["eventTypes"])
	}
	if fixture.store.merchantIDs[0] != internalTestMerchant {
		t.Fatalf("looked up %q, want the path id", fixture.store.merchantIDs[0])
	}
}

func TestInternalWebhookGetEmptyEventTypesIsAnArray(t *testing.T) {
	fixture := newInternalFixture(internalTestToken, true, true)
	fixture.store.webhook.EventTypes = nil
	data, _ := decodeBody(t, fixture.get(webhookPath()))["data"].(map[string]any)
	if types, ok := data["eventTypes"].([]any); !ok || len(types) != 0 {
		t.Fatalf("eventTypes = %#v, want an empty array", data["eventTypes"])
	}
}

func TestInternalWebhookGetNotConfigured(t *testing.T) {
	fixture := newInternalFixture(internalTestToken, true, true)
	fixture.store.webhook = nil
	recorder := fixture.get(webhookPath())
	if recorder.Code != http.StatusNotFound || errorCodeOf(decodeBody(t, recorder)) != "WEBHOOK_NOT_CONFIGURED" {
		t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestInternalWebhookMalformedMerchantIDIsNotFound(t *testing.T) {
	fixture := newInternalFixture(internalTestToken, true, true)
	for _, id := range []string{"m1", "not-a-uuid", "5f0b2c0e-6c53-4f6e-9a38-0f4b1f0e7a1", "5f0b2c0e-6c53-4f6e-9a38-0f4b1f0e7a1g"} {
		recorder := fixture.get("/internal/merchants/" + id + "/webhook")
		if recorder.Code != http.StatusNotFound || errorCodeOf(decodeBody(t, recorder)) != "NOT_FOUND" {
			t.Errorf("id %q: status = %d body = %s, want 404 NOT_FOUND", id, recorder.Code, recorder.Body.String())
		}
	}
	if len(fixture.store.merchantIDs) != 0 {
		t.Fatal("a malformed id reached the database")
	}
}

func TestInternalWebhookDecryptFailureNeverLeaks(t *testing.T) {
	fixture := newInternalFixture(internalTestToken, true, true)
	fixture.store.webhook.SigningSecretEnc = []byte("garbage-not-v1")
	recorder := fixture.get(webhookPath())
	if recorder.Code != http.StatusInternalServerError || errorCodeOf(decodeBody(t, recorder)) != "INTERNAL_ERROR" {
		t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), "garbage") || strings.Contains(recorder.Body.String(), "bad data") {
		t.Fatalf("response leaks internals: %s", recorder.Body.String())
	}
}

func TestInternalWebhookStoreFailureIsInternalError(t *testing.T) {
	fixture := newInternalFixture(internalTestToken, true, true)
	fixture.store.getErr = errors.New("connection refused to 10.0.0.5")
	recorder := fixture.get(webhookPath())
	if recorder.Code != http.StatusInternalServerError || strings.Contains(recorder.Body.String(), "10.0.0.5") {
		t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestInternalWebhookFailsClosedWithoutStoreOrCipher(t *testing.T) {
	for _, setup := range []struct {
		name       string
		store, box bool
		wantStatus int
	}{
		{"no cipher", true, false, http.StatusServiceUnavailable},
		{"no store", false, true, http.StatusServiceUnavailable},
	} {
		fixture := newInternalFixture(internalTestToken, setup.store, setup.box)
		for _, call := range []struct{ method, path string }{
			{http.MethodGet, webhookPath()},
			{http.MethodPost, attemptsPath()},
		} {
			recorder := fixture.do(call.method, call.path, internalTestToken, "req-1", validAttemptBody())
			if recorder.Code != setup.wantStatus || errorCodeOf(decodeBody(t, recorder)) != "SERVICE_UNAVAILABLE" {
				t.Errorf("%s %s %s: status = %d body = %s", setup.name, call.method, call.path, recorder.Code, recorder.Body.String())
			}
		}
		// Without the token the answer is still 401, never 503.
		if recorder := fixture.do(http.MethodGet, webhookPath(), "", "req-1", ""); recorder.Code != http.StatusUnauthorized {
			t.Errorf("%s: unauthenticated status = %d, want 401", setup.name, recorder.Code)
		}
	}
}

func TestInternalWebhookRecordsAnAttempt(t *testing.T) {
	fixture := newInternalFixture(internalTestToken, true, true)
	recorder := fixture.post(attemptsPath(), validAttemptBody())
	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
	}
	data, _ := decodeBody(t, recorder)["data"].(map[string]any)
	if data["recorded"] != true || data["eventId"] != internalTestEvent || data["attemptNumber"] != float64(2) {
		t.Fatalf("data = %v", data)
	}
	if len(fixture.store.recorded) != 1 {
		t.Fatalf("recorded %d attempts, want 1", len(fixture.store.recorded))
	}
	got := fixture.store.recorded[0]
	if got.MerchantID != internalTestMerchant || got.EventID != internalTestEvent || got.ParcelID != internalTestParcel ||
		got.TrackingNumber != "SDN4T8W2LC6" || got.AttemptNumber != 2 || got.Success || got.DeadLettered {
		t.Fatalf("params = %+v", got)
	}
	if got.HTTPStatus == nil || *got.HTTPStatus != 503 || got.DurationMS == nil || *got.DurationMS != 182 {
		t.Fatalf("params = %+v", got)
	}
	if got.ErrorMessage == nil || *got.ErrorMessage != "upstream unavailable" {
		t.Fatalf("error message = %v", got.ErrorMessage)
	}
	if got.NextRetryAt == nil || !got.NextRetryAt.Equal(time.Date(2026, 11, 4, 12, 35, 0, 0, time.UTC)) || got.NextRetryAt.Location() != time.UTC {
		t.Fatalf("nextRetryAt = %v", got.NextRetryAt)
	}
	if got.ID == "" || !isCanonicalUUID(got.ID) {
		t.Fatalf("attempt id = %q, want a generated UUID", got.ID)
	}
}

func TestInternalWebhookRecordIsIdempotent(t *testing.T) {
	fixture := newInternalFixture(internalTestToken, true, true)
	fixture.store.duplicate = true
	recorder := fixture.post(attemptsPath(), validAttemptBody())
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 for an attempt that was already recorded", recorder.Code)
	}
	if data, _ := decodeBody(t, recorder)["data"].(map[string]any); data["recorded"] != false {
		t.Fatalf("recorded = %v, want false", data["recorded"])
	}
}

func TestInternalWebhookRecordSuccessAndDeadLetter(t *testing.T) {
	fixture := newInternalFixture(internalTestToken, true, true)
	success := `{"eventId":"` + internalTestEvent + `","parcelId":"` + internalTestParcel + `","trackingNumber":"SDK7M2Q9XP4","attemptNumber":1,"httpStatus":200,"success":true}`
	if recorder := fixture.post(attemptsPath(), success); recorder.Code != http.StatusCreated {
		t.Fatalf("success status = %d body = %s", recorder.Code, recorder.Body.String())
	}
	dead := `{"eventId":"` + internalTestEvent + `","parcelId":"` + internalTestParcel + `","trackingNumber":"SDK7M2Q9XP4","attemptNumber":6,"success":false,"deadLettered":true,"errorMessage":"timeout"}`
	if recorder := fixture.post(attemptsPath(), dead); recorder.Code != http.StatusCreated {
		t.Fatalf("dead letter status = %d body = %s", recorder.Code, recorder.Body.String())
	}
	last := fixture.store.recorded[1]
	if !last.DeadLettered || last.HTTPStatus != nil || last.NextRetryAt != nil {
		t.Fatalf("dead letter params = %+v", last)
	}
}

func TestInternalWebhookRecordRequiresSuccessAndPreservesItsValue(t *testing.T) {
	body := func(success string) string {
		field := ""
		if success != "" {
			field = `,"success":` + success
		}
		return `{"eventId":"` + internalTestEvent + `","parcelId":"` + internalTestParcel + `",` +
			`"trackingNumber":"SDK7M2Q9XP4","attemptNumber":1,"httpStatus":200` + field + `}`
	}

	t.Run("missing is rejected", func(t *testing.T) {
		fixture := newInternalFixture(internalTestToken, true, true)
		recorder := fixture.post(attemptsPath(), body(""))
		if recorder.Code != http.StatusUnprocessableEntity || errorCodeOf(decodeBody(t, recorder)) != "VALIDATION_ERROR" {
			t.Fatalf("status = %d body = %s, want 422 VALIDATION_ERROR", recorder.Code, recorder.Body.String())
		}
		if len(fixture.store.recorded) != 0 {
			t.Fatal("a report without success was stored")
		}
	})

	for _, value := range []string{"true", "false"} {
		t.Run("explicit "+value+" is kept", func(t *testing.T) {
			fixture := newInternalFixture(internalTestToken, true, true)
			if recorder := fixture.post(attemptsPath(), body(value)); recorder.Code != http.StatusCreated {
				t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
			}
			if got := fixture.store.recorded[0].Success; got != (value == "true") {
				t.Fatalf("stored success = %v, want %s", got, value)
			}
		})
	}
}

func TestInternalWebhookRecordRejectsBadReports(t *testing.T) {
	base := func(mutate func(m map[string]any)) string {
		m := map[string]any{
			"eventId": internalTestEvent, "parcelId": internalTestParcel, "trackingNumber": "SDK7M2Q9XP4",
			"attemptNumber": 1, "httpStatus": 200, "success": true,
		}
		mutate(m)
		raw, _ := json.Marshal(m)
		return string(raw)
	}
	cases := map[string]string{
		"bad event id":              base(func(m map[string]any) { m["eventId"] = "nope" }),
		"bad parcel id":             base(func(m map[string]any) { m["parcelId"] = "" }),
		"missing tracking number":   base(func(m map[string]any) { m["trackingNumber"] = "  " }),
		"tracking number too long":  base(func(m map[string]any) { m["trackingNumber"] = strings.Repeat("A", 21) }),
		"attempt zero":              base(func(m map[string]any) { m["attemptNumber"] = 0 }),
		"attempt seven":             base(func(m map[string]any) { m["attemptNumber"] = 7 }),
		"missing success":           base(func(m map[string]any) { delete(m, "success") }),
		"status below range":        base(func(m map[string]any) { m["httpStatus"] = 99 }),
		"status above range":        base(func(m map[string]any) { m["httpStatus"] = 600 }),
		"negative duration":         base(func(m map[string]any) { m["durationMs"] = -1 }),
		"huge duration":             base(func(m map[string]any) { m["durationMs"] = 600001 }),
		"success with a retry":      base(func(m map[string]any) { m["nextRetryAt"] = "2026-11-04T12:35:00Z" }),
		"success but dead lettered": base(func(m map[string]any) { m["deadLettered"] = true }),
		"dead letter with a retry": base(func(m map[string]any) {
			m["success"] = false
			m["deadLettered"] = true
			m["nextRetryAt"] = "2026-11-04T12:35:00Z"
		}),
	}
	for name, body := range cases {
		fixture := newInternalFixture(internalTestToken, true, true)
		recorder := fixture.post(attemptsPath(), body)
		if recorder.Code != http.StatusUnprocessableEntity || errorCodeOf(decodeBody(t, recorder)) != "VALIDATION_ERROR" {
			t.Errorf("%s: status = %d body = %s, want 422 VALIDATION_ERROR", name, recorder.Code, recorder.Body.String())
		}
		if len(fixture.store.recorded) != 0 {
			t.Errorf("%s: an invalid report was stored", name)
		}
	}
}

func TestInternalWebhookRecordMalformedAndOversizedBodies(t *testing.T) {
	fixture := newInternalFixture(internalTestToken, true, true)
	for _, body := range []string{"{not json", `{"attemptNumber":"two"}`} {
		recorder := fixture.post(attemptsPath(), body)
		if recorder.Code != http.StatusBadRequest || errorCodeOf(decodeBody(t, recorder)) != "BAD_REQUEST" {
			t.Errorf("body %q: status = %d body = %s, want 400", body, recorder.Code, recorder.Body.String())
		}
	}
	huge := `{"errorMessage":"` + strings.Repeat("x", maxAttemptBodyBytes) + `"}`
	recorder := fixture.post(attemptsPath(), huge)
	if recorder.Code != http.StatusRequestEntityTooLarge || errorCodeOf(decodeBody(t, recorder)) != "PAYLOAD_TOO_LARGE" {
		t.Fatalf("oversized: status = %d body = %s, want 413", recorder.Code, recorder.Body.String())
	}
	if len(fixture.store.recorded) != 0 {
		t.Fatal("a rejected body was stored")
	}
}

func TestInternalWebhookRecordTruncatesLongErrorMessages(t *testing.T) {
	fixture := newInternalFixture(internalTestToken, true, true)
	long := strings.Repeat("é", 400) // multi-byte, so a byte cut would corrupt it
	body := `{"eventId":"` + internalTestEvent + `","parcelId":"` + internalTestParcel + `","trackingNumber":"SDK7M2Q9XP4","attemptNumber":1,"success":false,"errorMessage":"` + long + `"}`
	if recorder := fixture.post(attemptsPath(), body); recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
	}
	got := fixture.store.recorded[0].ErrorMessage
	if got == nil {
		t.Fatal("the truncated message was dropped")
	}
	if n := len([]rune(*got)); n != maxErrorMessageChars {
		t.Fatalf("stored %d characters, want %d", n, maxErrorMessageChars)
	}
}

func TestInternalWebhookRecordBlankErrorMessageIsNull(t *testing.T) {
	fixture := newInternalFixture(internalTestToken, true, true)
	body := `{"eventId":"` + internalTestEvent + `","parcelId":"` + internalTestParcel + `","trackingNumber":"SDK7M2Q9XP4","attemptNumber":1,"httpStatus":200,"success":true,"errorMessage":"   "}`
	if recorder := fixture.post(attemptsPath(), body); recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
	}
	if fixture.store.recorded[0].ErrorMessage != nil {
		t.Fatal("a blank message must be stored as NULL")
	}
}

func TestInternalWebhookRecordNotConfiguredAndStoreFailure(t *testing.T) {
	fixture := newInternalFixture(internalTestToken, true, true)
	fixture.store.recordErr = repository.ErrWebhookNotConfigured
	recorder := fixture.post(attemptsPath(), validAttemptBody())
	if recorder.Code != http.StatusNotFound || errorCodeOf(decodeBody(t, recorder)) != "WEBHOOK_NOT_CONFIGURED" {
		t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
	}

	fixture.store.recordErr = errors.New("pq: connection to 10.0.0.5 lost")
	recorder = fixture.post(attemptsPath(), validAttemptBody())
	if recorder.Code != http.StatusInternalServerError || strings.Contains(recorder.Body.String(), "10.0.0.5") {
		t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestInternalWebhookRoutesAreOnlyUnderInternal(t *testing.T) {
	fixture := newInternalFixture(internalTestToken, true, true)
	for _, route := range fixture.router.Routes() {
		if !strings.HasPrefix(route.Path, "/internal/") && !strings.HasPrefix(route.Path, "/health") && !strings.HasPrefix(route.Path, "/ready") {
			t.Errorf("unexpected route outside /internal: %s %s", route.Method, route.Path)
		}
	}
}

func TestIsCanonicalUUID(t *testing.T) {
	for _, good := range []string{internalTestMerchant, strings.ToUpper(internalTestMerchant), "00000000-0000-0000-0000-000000000000"} {
		if !isCanonicalUUID(good) {
			t.Errorf("%q rejected", good)
		}
	}
	for _, bad := range []string{"", "m1", internalTestMerchant + "0", strings.ReplaceAll(internalTestMerchant, "-", "x"), "5f0b2c0e-6c53-4f6e-9a38-0f4b1f0e7a1z"} {
		if isCanonicalUUID(bad) {
			t.Errorf("%q accepted", bad)
		}
	}
}
