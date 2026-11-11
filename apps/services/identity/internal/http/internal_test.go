package servicehttp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Vineth-R/swiftdrop/apps/services/identity/internal/repository"
	"github.com/Vineth-R/swiftdrop/apps/services/identity/internal/service"
	"github.com/Vineth-R/swiftdrop/packages/go-common/auth"
)

const (
	testInternalToken = "internal-token-for-tests-0123456789"
	testRequestID     = "req_test-123"
	courierPath       = "/internal/couriers/3f1c7a52-0000-4000-8000-000000000001/eligibility"
	contactPath       = "/internal/users/3f1c7a52-0000-4000-8000-000000000002/contact"
)

type fakeInternalService struct {
	eligibility  service.CourierEligibility
	contact      service.UserContact
	owner        service.MerchantOwnerResult
	lifecycle    service.LifecycleResult
	sessionState service.SessionState
	err          error

	calls  int
	gotID  string
	method string
}

func (f *fakeInternalService) CourierEligibility(_ context.Context, id string) (service.CourierEligibility, error) {
	f.calls++
	f.gotID, f.method = id, "eligibility"
	return f.eligibility, f.err
}

func (f *fakeInternalService) CreateMerchantOwner(context.Context, service.MerchantOwnerInput) (service.MerchantOwnerResult, error) {
	return f.owner, f.err
}
func (f *fakeInternalService) ApplyMerchantLifecycle(context.Context, service.LifecycleInput) (service.LifecycleResult, error) {
	return f.lifecycle, f.err
}
func (f *fakeInternalService) SessionState(context.Context, string) (service.SessionState, error) {
	return f.sessionState, f.err
}
func (f *fakeInternalService) DeleteMerchantOwner(context.Context, string) error { return f.err }

func (f *fakeInternalService) UserContact(_ context.Context, id string) (service.UserContact, error) {
	f.calls++
	f.gotID, f.method = id, "contact"
	return f.contact, f.err
}

type internalHarness struct {
	service *fakeInternalService
	handler http.Handler
	logs    *bytes.Buffer
}

func newInternalHarness(token string) *internalHarness {
	logs := &bytes.Buffer{}
	fake := &fakeInternalService{
		eligibility: service.CourierEligibility{IsActive: true, IsAvailable: true, Role: auth.RoleCourier},
		contact:     service.UserContact{FullName: "Nimal Perera", Email: "nimal@example.com", Phone: "+94771234567"},
	}
	handler := New(slog.New(slog.NewTextHandler(logs, nil)), WithInternal(InternalRoutes{Service: fake, Token: token}))
	return &internalHarness{service: fake, handler: handler, logs: logs}
}

// get sends a request carrying the given headers. Pass pairs of name, value.
func (h *internalHarness) get(path string, headers ...string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, path, nil)
	for i := 0; i+1 < len(headers); i += 2 {
		request.Header.Set(headers[i], headers[i+1])
	}
	recorder := httptest.NewRecorder()
	h.handler.ServeHTTP(recorder, request)
	return recorder
}

func (h *internalHarness) post(path, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(internalTokenHeader, testInternalToken)
	request.Header.Set("X-Request-Id", testRequestID)
	recorder := httptest.NewRecorder()
	h.handler.ServeHTTP(recorder, request)
	return recorder
}

func (h *internalHarness) delete(path string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodDelete, path, nil)
	request.Header.Set(internalTokenHeader, testInternalToken)
	request.Header.Set("X-Request-Id", testRequestID)
	recorder := httptest.NewRecorder()
	h.handler.ServeHTTP(recorder, request)
	return recorder
}

func (h *internalHarness) authorised(path string) *httptest.ResponseRecorder {
	return h.get(path, internalTokenHeader, testInternalToken, "X-Request-Id", testRequestID)
}

func TestInternal_courierEligibility_returnsDocumentedFields(t *testing.T) {
	h := newInternalHarness(testInternalToken)

	response := h.authorised(courierPath)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", response.Code, response.Body)
	}
	var body struct {
		Data map[string]any `json:"data"`
		Meta struct {
			RequestID string `json:"requestId"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	want := map[string]any{"isActive": true, "isAvailable": true, "role": "COURIER"}
	if len(body.Data) != len(want) {
		t.Fatalf("data = %v, want exactly %v", body.Data, want)
	}
	for key, value := range want {
		if body.Data[key] != value {
			t.Fatalf("data[%s] = %v, want %v", key, body.Data[key], value)
		}
	}
	if body.Meta.RequestID != testRequestID {
		t.Fatalf("meta.requestId = %q, want the caller's %q", body.Meta.RequestID, testRequestID)
	}
	if response.Header().Get("X-Request-Id") != testRequestID {
		t.Fatal("X-Request-Id was not echoed")
	}
	if response.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("internal responses must not be cached")
	}
	if h.service.gotID != "3f1c7a52-0000-4000-8000-000000000001" {
		t.Fatalf("service got ID %q", h.service.gotID)
	}
}

func TestInternal_courierEligibility_reportsInactiveUnavailableAndOtherRoles(t *testing.T) {
	tests := []struct {
		name string
		in   service.CourierEligibility
		want string
	}{
		{name: "inactive courier", in: service.CourierEligibility{IsActive: false, IsAvailable: true, Role: auth.RoleCourier},
			want: `{"isActive":false,"isAvailable":true,"role":"COURIER"}`},
		{name: "unavailable courier", in: service.CourierEligibility{IsActive: true, IsAvailable: false, Role: auth.RoleCourier},
			want: `{"isActive":true,"isAvailable":false,"role":"COURIER"}`},
		{name: "not a courier", in: service.CourierEligibility{IsActive: true, IsAvailable: false, Role: auth.RoleCustomer},
			want: `{"isActive":true,"isAvailable":false,"role":"CUSTOMER"}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			h := newInternalHarness(testInternalToken)
			h.service.eligibility = test.in

			response := h.authorised(courierPath)

			var body struct {
				Data json.RawMessage `json:"data"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || response.Code != http.StatusOK {
				t.Fatalf("status = %d err = %v body = %s", response.Code, err, response.Body)
			}
			if string(body.Data) != test.want {
				t.Fatalf("data = %s, want %s", body.Data, test.want)
			}
		})
	}
}

func TestInternal_userContact_returnsDocumentedFields(t *testing.T) {
	h := newInternalHarness(testInternalToken)

	response := h.authorised(contactPath)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", response.Code, response.Body)
	}
	var body struct {
		Data json.RawMessage `json:"data"`
	}
	_ = json.Unmarshal(response.Body.Bytes(), &body)
	want := `{"fullName":"Nimal Perera","email":"nimal@example.com","phone":"+94771234567"}`
	if string(body.Data) != want {
		t.Fatalf("data = %s, want %s", body.Data, want)
	}
	if h.service.gotID != "3f1c7a52-0000-4000-8000-000000000002" {
		t.Fatalf("service got ID %q", h.service.gotID)
	}
}

func TestInternal_userContact_userWithoutPhone(t *testing.T) {
	h := newInternalHarness(testInternalToken)
	h.service.contact = service.UserContact{FullName: "Google Person", Email: "g@example.com"}

	response := h.authorised(contactPath)

	if !strings.Contains(response.Body.String(), `"phone":""`) {
		t.Fatalf("a missing phone must still be present as an empty string: %s", response.Body)
	}
}

func TestInternal_refusesBadOrMissingTokens(t *testing.T) {
	tests := []struct {
		name  string
		token string
		sent  []string
	}{
		{name: "no token header", token: testInternalToken, sent: []string{"X-Request-Id", testRequestID}},
		{name: "wrong token", token: testInternalToken, sent: []string{internalTokenHeader, "wrong-token-value-0123456789", "X-Request-Id", testRequestID}},
		{name: "token with different case", token: testInternalToken, sent: []string{internalTokenHeader, strings.ToUpper(testInternalToken), "X-Request-Id", testRequestID}},
		{name: "prefix of the token", token: testInternalToken, sent: []string{internalTokenHeader, testInternalToken[:10], "X-Request-Id", testRequestID}},
		{name: "token plus extra", token: testInternalToken, sent: []string{internalTokenHeader, testInternalToken + "x", "X-Request-Id", testRequestID}},
		{name: "empty token header", token: testInternalToken, sent: []string{internalTokenHeader, "", "X-Request-Id", testRequestID}},
		{name: "bearer token is not accepted instead", token: testInternalToken, sent: []string{"Authorization", "Bearer " + testInternalToken, "X-Request-Id", testRequestID}},
	}
	for _, test := range tests {
		for _, path := range []string{courierPath, contactPath} {
			t.Run(test.name+" "+path, func(t *testing.T) {
				h := newInternalHarness(test.token)

				response := h.get(path, test.sent...)

				if response.Code != http.StatusUnauthorized || errorCode(t, response) != "UNAUTHENTICATED" {
					t.Fatalf("status = %d body = %s, want 401 UNAUTHENTICATED", response.Code, response.Body)
				}
				if h.service.calls != 0 {
					t.Fatal("the service was reached without a valid token")
				}
			})
		}
	}
}

func TestInternal_failsClosedWhenNoTokenIsConfigured(t *testing.T) {
	// The dangerous case: configured "" and supplied "" must not match.
	for _, supplied := range [][]string{
		{"X-Request-Id", testRequestID},
		{internalTokenHeader, "", "X-Request-Id", testRequestID},
		{internalTokenHeader, "   ", "X-Request-Id", testRequestID},
		{internalTokenHeader, testInternalToken, "X-Request-Id", testRequestID},
	} {
		for _, path := range []string{courierPath, contactPath} {
			h := newInternalHarness("")

			response := h.get(path, supplied...)

			if response.Code != http.StatusUnauthorized {
				t.Fatalf("headers %v on %s: status = %d, want 401 with no server token", supplied, path, response.Code)
			}
			if h.service.calls != 0 {
				t.Fatal("the service was reached although no token is configured")
			}
		}
	}
}

func TestInternal_requiresAWellFormedRequestID(t *testing.T) {
	tests := []struct {
		name    string
		headers []string
	}{
		{name: "missing", headers: []string{internalTokenHeader, testInternalToken}},
		{name: "empty", headers: []string{internalTokenHeader, testInternalToken, "X-Request-Id", ""}},
		{name: "blank", headers: []string{internalTokenHeader, testInternalToken, "X-Request-Id", "   "}},
		{name: "illegal characters", headers: []string{internalTokenHeader, testInternalToken, "X-Request-Id", "req id with spaces"}},
		{name: "script characters", headers: []string{internalTokenHeader, testInternalToken, "X-Request-Id", "<script>"}},
		{name: "too long", headers: []string{internalTokenHeader, testInternalToken, "X-Request-Id", strings.Repeat("a", 129)}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			h := newInternalHarness(testInternalToken)

			response := h.get(contactPath, test.headers...)

			if response.Code != http.StatusBadRequest || errorCode(t, response) != "BAD_REQUEST" {
				t.Fatalf("status = %d body = %s, want 400 BAD_REQUEST", response.Code, response.Body)
			}
			if h.service.calls != 0 {
				t.Fatal("the service was reached without a valid request ID")
			}
		})
	}
}

func TestInternal_tokenIsCheckedBeforeTheRequestID(t *testing.T) {
	// An unauthenticated caller must learn nothing, not even which header is
	// malformed.
	h := newInternalHarness(testInternalToken)

	response := h.get(contactPath)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", response.Code)
	}
}

func TestInternal_notFound(t *testing.T) {
	for name, path := range map[string]string{"eligibility": courierPath, "contact": contactPath} {
		t.Run(name, func(t *testing.T) {
			h := newInternalHarness(testInternalToken)
			h.service.err = service.ErrUserNotFound

			response := h.authorised(path)

			if response.Code != http.StatusNotFound || errorCode(t, response) != "NOT_FOUND" {
				t.Fatalf("status = %d body = %s, want 404 NOT_FOUND", response.Code, response.Body)
			}
		})
	}
}

func TestInternal_unexpectedFailureIsGeneric500AndLogged(t *testing.T) {
	h := newInternalHarness(testInternalToken)
	h.service.err = errors.New("pq: password authentication failed for user swiftdrop")

	response := h.authorised(contactPath)

	if response.Code != http.StatusInternalServerError || errorCode(t, response) != "INTERNAL_ERROR" {
		t.Fatalf("status = %d body = %s, want 500 INTERNAL_ERROR", response.Code, response.Body)
	}
	if strings.Contains(response.Body.String(), "password") || strings.Contains(response.Body.String(), "swiftdrop") {
		t.Fatal("internal details leaked to the caller")
	}
	if !strings.Contains(h.logs.String(), "internal request failed") || !strings.Contains(h.logs.String(), testRequestID) {
		t.Fatalf("failure was not logged with the request ID: %s", h.logs)
	}
}

func TestInternal_logsNeverContainTheTokenOrContactDetails(t *testing.T) {
	h := newInternalHarness(testInternalToken)

	h.authorised(contactPath)
	h.get(contactPath, internalTokenHeader, "guess-0123456789-guess", "X-Request-Id", testRequestID)
	h.service.err = errors.New("boom")
	h.authorised(contactPath)

	logs := h.logs.String()
	for _, secret := range []string{testInternalToken, "guess-0123456789-guess", "nimal@example.com", "+94771234567"} {
		if strings.Contains(logs, secret) {
			t.Fatalf("logs contain %q", secret)
		}
	}
}

func TestInternal_errorsNeverEchoTheToken(t *testing.T) {
	h := newInternalHarness(testInternalToken)

	response := h.get(contactPath, internalTokenHeader, "guess-0123456789-guess", "X-Request-Id", testRequestID)

	if strings.Contains(response.Body.String(), "guess-0123456789-guess") || strings.Contains(response.Body.String(), testInternalToken) {
		t.Fatal("the response echoes a token")
	}
}

func TestInternal_onlyGETIsAllowed(t *testing.T) {
	h := newInternalHarness(testInternalToken)

	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch} {
		request := httptest.NewRequest(method, contactPath, nil)
		request.Header.Set(internalTokenHeader, testInternalToken)
		request.Header.Set("X-Request-Id", testRequestID)
		recorder := httptest.NewRecorder()
		h.handler.ServeHTTP(recorder, request)

		if recorder.Code != http.StatusMethodNotAllowed {
			t.Fatalf("%s status = %d, want 405", method, recorder.Code)
		}
	}
	if h.service.calls != 0 {
		t.Fatal("a non-GET request reached the service")
	}
}

func TestInternal_unknownInternalPathIsNotFound(t *testing.T) {
	h := newInternalHarness(testInternalToken)

	if response := h.authorised("/internal/nothing"); response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", response.Code)
	}
}

func TestRouter_withoutInternalHasNoInternalRoutes(t *testing.T) {
	handler := New(slog.New(slog.DiscardHandler))

	for _, path := range []string{courierPath, contactPath} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		request.Header.Set(internalTokenHeader, testInternalToken)
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusNotFound {
			t.Fatalf("%s status = %d, want 404", path, recorder.Code)
		}
	}
}

func TestInternal_merchantOwnerLifecycleSessionAndDelete(t *testing.T) {
	h := newInternalHarness(testInternalToken)
	h.service.owner = service.MerchantOwnerResult{UserID: "user-1", Created: true}
	response := h.post("/internal/merchant-owners", `{"email":"owner@example.com","password":"Str0ngPass!","fullName":"Owner Name","phone":"+94771234567"}`)
	if response.Code != http.StatusCreated {
		t.Fatalf("create owner status = %d (%s)", response.Code, response.Body)
	}

	h.service.lifecycle = service.LifecycleResult{
		UserID: "user-1", TransitionID: "3f1c7a52-0000-4000-8000-000000000010",
		Sequence: 1, Operation: "APPROVE", SessionVersion: 2, IsActive: true,
	}
	response = h.post("/internal/merchant-owners/user-1/lifecycle", `{"transitionId":"3f1c7a52-0000-4000-8000-000000000010","sequence":1,"operation":"APPROVE"}`)
	if response.Code != http.StatusOK {
		t.Fatalf("lifecycle status = %d (%s)", response.Code, response.Body)
	}
	if !strings.Contains(response.Body.String(), `"sessionVersion":2`) {
		t.Fatalf("lifecycle body = %s", response.Body)
	}

	h.service.sessionState = service.SessionState{Role: auth.RoleMerchant, IsActive: true, SessionVersion: 2}
	response = h.authorised("/internal/users/user-1/session-state")
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"sessionVersion":2`) {
		t.Fatalf("session state = %d (%s)", response.Code, response.Body)
	}

	response = h.delete("/internal/merchant-owners/user-1")
	if response.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d (%s)", response.Code, response.Body)
	}

	h.service.err = service.ErrUserNotFound
	if response := h.authorised("/internal/users/user-1/session-state"); response.Code != http.StatusNotFound {
		t.Fatalf("missing session state = %d, want 404", response.Code)
	}
}

// A refusal the caller can act on is a 409 with its own code, not a 500: the
// contract in contracts/openapi/identity-internal.yaml says so.
func TestInternal_merchantOwnerRefusalsAreConflictsAndNotFound(t *testing.T) {
	lifecycleBody := `{"transitionId":"3f1c7a52-0000-4000-8000-000000000010","sequence":1,"operation":"APPROVE"}`
	tests := []struct {
		name   string
		err    error
		call   func(h *internalHarness) *httptest.ResponseRecorder
		status int
		code   string
	}{
		{"removing an owner that cannot be removed", repository.ErrOwnerNotEligible,
			func(h *internalHarness) *httptest.ResponseRecorder {
				return h.delete("/internal/merchant-owners/user-1")
			},
			http.StatusConflict, "OWNER_NOT_ELIGIBLE"},
		{"a lifecycle transition for a user who is not a merchant", repository.ErrNotMerchantOwner,
			func(h *internalHarness) *httptest.ResponseRecorder {
				return h.post("/internal/merchant-owners/user-1/lifecycle", lifecycleBody)
			},
			http.StatusConflict, "NOT_A_MERCHANT_OWNER"},
		{"a lifecycle transition for an unknown user", service.ErrUserNotFound,
			func(h *internalHarness) *httptest.ResponseRecorder {
				return h.post("/internal/merchant-owners/user-1/lifecycle", lifecycleBody)
			},
			http.StatusNotFound, "NOT_FOUND"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			h := newInternalHarness(testInternalToken)
			h.service.err = test.err

			response := test.call(h)

			if response.Code != test.status || internalErrorCode(t, response) != test.code {
				t.Fatalf("status = %d code = %s (%s), want %d %s", response.Code, internalErrorCode(t, response), response.Body, test.status, test.code)
			}
			if strings.Contains(h.logs.String(), "internal request failed") {
				t.Fatalf("a refusal the caller can act on was logged as a failure:\n%s", h.logs)
			}
		})
	}
}

func internalErrorCode(t *testing.T, response *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not an error envelope: %v (%s)", err, response.Body.String())
	}
	return body.Error.Code
}
