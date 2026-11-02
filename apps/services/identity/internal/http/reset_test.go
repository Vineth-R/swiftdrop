package servicehttp

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Vineth-R/swiftdrop/apps/services/identity/internal/service"
)

type fakeResetService struct {
	forgotErr, resetErr error

	forgot      []service.ForgotPasswordInput
	resetInputs []service.ResetPasswordInput
}

func (f *fakeResetService) ForgotPassword(_ context.Context, input service.ForgotPasswordInput) error {
	f.forgot = append(f.forgot, input)
	return f.forgotErr
}

func (f *fakeResetService) ResetPassword(_ context.Context, input service.ResetPasswordInput) error {
	f.resetInputs = append(f.resetInputs, input)
	return f.resetErr
}

type resetHarness struct {
	service *fakeResetService
	handler http.Handler
}

func newResetHarness() *resetHarness {
	fake := &fakeResetService{}
	handler := New(slog.New(slog.DiscardHandler), WithPasswordReset(PasswordResetRoutes{
		Service: fake,
		Cookies: CookieConfig{AccessName: "sd_access", RefreshName: "sd_refresh", SameSite: http.SameSiteLaxMode},
	}))
	return &resetHarness{service: fake, handler: handler}
}

func (h *resetHarness) post(path, body string, headers ...string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	for i := 0; i+1 < len(headers); i += 2 {
		request.Header.Set(headers[i], headers[i+1])
	}
	recorder := httptest.NewRecorder()
	h.handler.ServeHTTP(recorder, request)
	return recorder
}

func messageOf(t *testing.T, response *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Data struct {
			Message string `json:"message"`
		} `json:"data"`
		Meta struct {
			RequestID string `json:"requestId"`
			Timestamp string `json:"timestamp"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("not a success envelope: %v (%s)", err, response.Body.String())
	}
	if body.Meta.RequestID == "" || body.Meta.Timestamp == "" {
		t.Fatalf("meta is incomplete: %s", response.Body.String())
	}
	return body.Data.Message
}

func TestForgotPassword_sameReplyAndCallsService(t *testing.T) {
	h := newResetHarness()

	response := h.post("/auth/forgot-password", `{"email":"nimal@example.com"}`, "X-Real-IP", "203.0.113.4")

	if response.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", response.Code)
	}
	if got := messageOf(t, response); got != forgotPasswordMessage {
		t.Fatalf("message = %q", got)
	}
	if len(h.service.forgot) != 1 || h.service.forgot[0].Email != "nimal@example.com" {
		t.Fatalf("service calls = %+v", h.service.forgot)
	}
	if h.service.forgot[0].IP.String() != "203.0.113.4" {
		t.Fatalf("service got IP %v, want the gateway-provided client address", h.service.forgot[0].IP)
	}
	if response.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("reset responses must not be cached")
	}
}

func TestForgotPassword_replyDoesNotDependOnTheAccount(t *testing.T) {
	// The service returns nil for known, unknown, Google-only and disabled
	// accounts alike; the handler must turn that into one identical response.
	first := newResetHarness().post("/auth/forgot-password", `{"email":"known@example.com"}`)
	second := newResetHarness().post("/auth/forgot-password", `{"email":"nobody@example.com"}`)

	if first.Code != second.Code || messageOf(t, first) != messageOf(t, second) {
		t.Fatalf("replies differ: %d %q vs %d %q", first.Code, first.Body, second.Code, second.Body)
	}
}

func TestForgotPassword_validationAndBadBodies(t *testing.T) {
	h := newResetHarness()
	h.service.forgotErr = &service.ValidationError{Issues: []service.Issue{{Field: "email", Issue: "must be a valid email address"}}}

	response := h.post("/auth/forgot-password", `{"email":"nope"}`)
	if response.Code != http.StatusUnprocessableEntity || errorCode(t, response) != "VALIDATION_ERROR" {
		t.Fatalf("status = %d body = %s, want 422 VALIDATION_ERROR", response.Code, response.Body)
	}

	for body, wantStatus := range map[string]int{"{not json": http.StatusBadRequest, "": http.StatusBadRequest} {
		if got := newResetHarness().post("/auth/forgot-password", body); got.Code != wantStatus {
			t.Fatalf("body %q status = %d, want %d", body, got.Code, wantStatus)
		}
	}

	wrongType := httptest.NewRequest(http.MethodPost, "/auth/forgot-password", strings.NewReader("email=a"))
	wrongType.Header.Set("Content-Type", "text/plain")
	recorder := httptest.NewRecorder()
	h.handler.ServeHTTP(recorder, wrongType)
	if recorder.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("status = %d, want 415", recorder.Code)
	}
}

func TestForgotPassword_unexpectedFailureIsGeneric500(t *testing.T) {
	h := newResetHarness()
	h.service.forgotErr = errors.New("connection to db at 10.0.0.5 refused")

	response := h.post("/auth/forgot-password", `{"email":"nimal@example.com"}`)

	if response.Code != http.StatusInternalServerError || errorCode(t, response) != "INTERNAL_ERROR" {
		t.Fatalf("status = %d body = %s", response.Code, response.Body)
	}
	if strings.Contains(response.Body.String(), "10.0.0.5") {
		t.Fatal("internal details leaked to the client")
	}
}

func TestForgotPassword_rateLimitedPerIP(t *testing.T) {
	h := newResetHarness()

	for i := 0; i < resetAttemptsPerMinute; i++ {
		// A different email each time, so only the address limit can trigger.
		body := `{"email":"user` + string(rune('a'+i)) + `@example.com"}`
		if response := h.post("/auth/forgot-password", body, "X-Real-IP", "198.51.100.1"); response.Code != http.StatusAccepted {
			t.Fatalf("request %d status = %d, want 202", i+1, response.Code)
		}
	}
	response := h.post("/auth/forgot-password", `{"email":"another@example.com"}`, "X-Real-IP", "198.51.100.1")
	if response.Code != http.StatusTooManyRequests || errorCode(t, response) != "RATE_LIMITED" {
		t.Fatalf("status = %d body = %s, want 429 RATE_LIMITED", response.Code, response.Body)
	}
	if response.Header().Get("Retry-After") == "" {
		t.Fatal("429 must carry Retry-After")
	}

	// Another client address has its own budget.
	if other := h.post("/auth/forgot-password", `{"email":"another@example.com"}`, "X-Real-IP", "198.51.100.2"); other.Code != http.StatusAccepted {
		t.Fatalf("a different client was limited: %d", other.Code)
	}
}

func TestForgotPassword_rateLimitedPerEmailAcrossAddresses(t *testing.T) {
	h := newResetHarness()

	for i := 0; i < resetAttemptsPerMinute; i++ {
		address := "192.0.2." + string(rune('1'+i))
		if response := h.post("/auth/forgot-password", `{"email":"victim@example.com"}`, "X-Real-IP", address); response.Code != http.StatusAccepted {
			t.Fatalf("request %d status = %d, want 202", i+1, response.Code)
		}
	}
	// Same inbox, new address and different letter case/whitespace.
	response := h.post("/auth/forgot-password", `{"email":"  VICTIM@Example.com "}`, "X-Real-IP", "192.0.2.99")
	if response.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429 for the flooded email", response.Code)
	}
	if len(h.service.forgot) != resetAttemptsPerMinute {
		t.Fatalf("service was called %d times, want %d", len(h.service.forgot), resetAttemptsPerMinute)
	}
}

func TestResetPassword_success(t *testing.T) {
	h := newResetHarness()

	response := h.post("/auth/reset-password", `{"token":"the-token","newPassword":"N3wStr0ngPass!"}`)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.Code)
	}
	if got := messageOf(t, response); got != resetPasswordMessage {
		t.Fatalf("message = %q", got)
	}
	if len(h.service.resetInputs) != 1 || h.service.resetInputs[0].Token != "the-token" || h.service.resetInputs[0].NewPassword != "N3wStr0ngPass!" {
		t.Fatalf("service calls = %+v", h.service.resetInputs)
	}

	cleared := map[string]bool{}
	for _, cookie := range response.Result().Cookies() {
		if cookie.Value == "" && cookie.MaxAge < 0 {
			cleared[cookie.Name] = true
		}
	}
	if !cleared["sd_access"] || !cleared["sd_refresh"] {
		t.Fatalf("session cookies were not cleared: %v", response.Result().Cookies())
	}
	if strings.Contains(response.Body.String(), "N3wStr0ngPass!") || strings.Contains(response.Body.String(), "the-token") {
		t.Fatal("the response echoes the token or password")
	}
}

func TestResetPassword_errors(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		status   int
		wantCode string
	}{
		{name: "invalid or expired token", err: service.ErrResetTokenInvalid, status: http.StatusBadRequest, wantCode: "INVALID_OR_EXPIRED_RESET_TOKEN"},
		{name: "wrapped token error", err: errors.Join(errors.New("detail"), service.ErrResetTokenInvalid), status: http.StatusBadRequest, wantCode: "INVALID_OR_EXPIRED_RESET_TOKEN"},
		{name: "weak password", err: &service.ValidationError{Issues: []service.Issue{{Field: "newPassword", Issue: "must be at least 8 characters"}}}, status: http.StatusUnprocessableEntity, wantCode: "VALIDATION_ERROR"},
		{name: "internal error", err: errors.New("boom"), status: http.StatusInternalServerError, wantCode: "INTERNAL_ERROR"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			h := newResetHarness()
			h.service.resetErr = test.err

			response := h.post("/auth/reset-password", `{"token":"t","newPassword":"x"}`)

			if response.Code != test.status || errorCode(t, response) != test.wantCode {
				t.Fatalf("status = %d body = %s, want %d %s", response.Code, response.Body, test.status, test.wantCode)
			}
			for _, cookie := range response.Result().Cookies() {
				t.Fatalf("a failed reset set cookie %q", cookie.Name)
			}
		})
	}
}

func TestResetPassword_sameMessageForEveryBadToken(t *testing.T) {
	var bodies []string
	for _, err := range []error{service.ErrResetTokenInvalid, errors.Join(service.ErrResetTokenInvalid)} {
		h := newResetHarness()
		h.service.resetErr = err
		bodies = append(bodies, stripRequestID(h.post("/auth/reset-password", `{"token":"t","newPassword":"N3wStr0ngPass"}`).Body.String()))
	}
	if bodies[0] != bodies[1] {
		t.Fatalf("responses differ: %q vs %q", bodies[0], bodies[1])
	}
}

func TestResetPassword_rateLimitedPerIP(t *testing.T) {
	h := newResetHarness()
	h.service.resetErr = service.ErrResetTokenInvalid

	for i := 0; i < resetAttemptsPerMinute; i++ {
		if response := h.post("/auth/reset-password", `{"token":"guess","newPassword":"N3wStr0ngPass"}`, "X-Real-IP", "198.51.100.7"); response.Code != http.StatusBadRequest {
			t.Fatalf("request %d status = %d, want 400", i+1, response.Code)
		}
	}
	if response := h.post("/auth/reset-password", `{"token":"guess","newPassword":"N3wStr0ngPass"}`, "X-Real-IP", "198.51.100.7"); response.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429 so tokens cannot be guessed quickly", response.Code)
	}
}

func TestResetPassword_oversizedBody(t *testing.T) {
	h := newResetHarness()

	response := h.post("/auth/reset-password", `{"token":"`+strings.Repeat("a", maxBodyBytes)+`","newPassword":"x"}`)

	if response.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413", response.Code)
	}
}

func TestRouter_withoutPasswordResetHasNoResetRoutes(t *testing.T) {
	handler := New(slog.New(slog.DiscardHandler))

	for _, path := range []string{"/auth/forgot-password", "/auth/reset-password"} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{}`))
		request.Header.Set("Content-Type", "application/json")
		handler.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusNotFound {
			t.Fatalf("%s status = %d, want 404 when reset is not configured", path, recorder.Code)
		}
	}
}

// stripRequestID removes the per-request ID so two responses can be compared.
func stripRequestID(body string) string {
	start := strings.Index(body, `"requestId":"`)
	if start == -1 {
		return body
	}
	end := strings.Index(body[start+len(`"requestId":"`):], `"`)
	return body[:start] + body[start+len(`"requestId":"`)+end:]
}
