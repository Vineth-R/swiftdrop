package servicehttp

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Vineth-R/swiftdrop/apps/services/identity/internal/service"
	"github.com/Vineth-R/swiftdrop/packages/go-common/auth"
)

// send makes a request with a JSON body. An empty contentType means
// application/json.
func (h *profileHarness) send(t *testing.T, method, path, bearer, body, contentType string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	if contentType == "" {
		contentType = "application/json"
	}
	request.Header.Set("Content-Type", contentType)
	if bearer != "" {
		request.Header.Set("Authorization", "Bearer "+bearer)
	}
	recorder := httptest.NewRecorder()
	h.handler.ServeHTTP(recorder, request)
	return recorder
}

func (h *profileHarness) customerToken(t *testing.T) string {
	t.Helper()
	return h.token(t, profileUserID, auth.RoleCustomer)
}

func ptr(value string) *string { return &value }

const (
	typedCurrent = `"Str0ngPass!"`
	typedNew     = `"N3wStrongPass!"`
)

func TestUpdateProfile_requiresAuthentication(t *testing.T) {
	h := newProfileHarness(t)

	response := h.send(t, http.MethodPatch, "/users/me", "", `{"fullName":"Nimal P. Perera"}`, "")

	if response.Code != http.StatusUnauthorized || errorCode(t, response) != "UNAUTHENTICATED" {
		t.Fatalf("status = %d body = %s, want 401 UNAUTHENTICATED", response.Code, response.Body)
	}
	if h.service.updateCalls != 0 {
		t.Fatal("an unauthenticated request reached the service")
	}
}

func TestUpdateProfile_returnsTheUpdatedAccount(t *testing.T) {
	h := newProfileHarness(t)

	response := h.send(t, http.MethodPatch, "/users/me", h.customerToken(t),
		`{"fullName":"Nimal P. Perera","phone":"+94771234000"}`, "")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s, want 200", response.Code, response.Body)
	}
	var body struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not JSON: %v", err)
	}
	want := map[string]any{
		"id":           profileUserID,
		"email":        "nimal@example.com",
		"fullName":     "Nimal P. Perera",
		"phone":        "+94771234000",
		"role":         "CUSTOMER",
		"authProvider": "LOCAL",
		"createdAt":    "2026-11-02T09:30:00Z",
	}
	if len(body.Data) != len(want) {
		t.Fatalf("data has %d fields, want the %d of section 2.7: %v", len(body.Data), len(want), body.Data)
	}
	for key, value := range want {
		if body.Data[key] != value {
			t.Errorf("data.%s = %v, want %v", key, body.Data[key], value)
		}
	}
	if got := response.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", got)
	}
}

func TestUpdateProfile_passesOnlyTheFieldsThatWereSent(t *testing.T) {
	tests := []struct {
		name      string
		body      string
		wantName  *string
		wantPhone *string
	}{
		{"name only", `{"fullName":"Nimal P. Perera"}`, ptr("Nimal P. Perera"), nil},
		{"phone only", `{"phone":"+94771234000"}`, nil, ptr("+94771234000")},
		{"both", `{"fullName":"Nimal P. Perera","phone":"+94771234000"}`, ptr("Nimal P. Perera"), ptr("+94771234000")},
		{"null means unchanged", `{"fullName":null,"phone":"+94771234000"}`, nil, ptr("+94771234000")},
		{"nothing", `{}`, nil, nil},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			h := newProfileHarness(t)

			h.send(t, http.MethodPatch, "/users/me", h.customerToken(t), test.body, "")

			got := h.service.updateInput
			if !samePointer(got.FullName, test.wantName) || !samePointer(got.Phone, test.wantPhone) {
				t.Fatalf("service got %+v, want name %v phone %v", got, test.wantName, test.wantPhone)
			}
		})
	}
}

func samePointer(a, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func TestUpdateProfile_cannotChangeTheEmailOrRole(t *testing.T) {
	h := newProfileHarness(t)

	response := h.send(t, http.MethodPatch, "/users/me", h.customerToken(t),
		`{"fullName":"Nimal P. Perera","email":"attacker@example.com","role":"ADMIN","id":"someone-else"}`, "")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.Code)
	}
	if h.service.updateInput.FullName == nil || h.service.updateInput.Phone != nil {
		t.Fatalf("service got %+v, want only the full name", h.service.updateInput)
	}
	if h.service.lastWriteUserID != profileUserID {
		t.Fatalf("service was asked to update %q, want the user in the token", h.service.lastWriteUserID)
	}
}

func TestUpdateProfile_reportsEveryInvalidField(t *testing.T) {
	h := newProfileHarness(t)
	h.service.writeErr = &service.ValidationError{Issues: []service.Issue{
		{Field: "fullName", Issue: "must be 2 to 150 characters"},
		{Field: "phone", Issue: "must be in E.164 format, for example +94771234567"},
	}}

	response := h.send(t, http.MethodPatch, "/users/me", h.customerToken(t), `{"fullName":"A","phone":"0771234567"}`, "")

	if response.Code != http.StatusUnprocessableEntity || errorCode(t, response) != "VALIDATION_ERROR" {
		t.Fatalf("status = %d body = %s, want 422 VALIDATION_ERROR", response.Code, response.Body)
	}
	var body struct {
		Error struct {
			Details []struct{ Field, Issue string } `json:"details"`
		} `json:"error"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not JSON: %v", err)
	}
	if len(body.Error.Details) != 2 || body.Error.Details[0].Field != "fullName" || body.Error.Details[1].Field != "phone" {
		t.Fatalf("details = %+v, want fullName then phone", body.Error.Details)
	}
	if strings.Contains(response.Body.String(), "0771234567") {
		t.Fatalf("the response echoes the submitted phone: %s", response.Body)
	}
}

func TestUpdateProfile_refusesAnUnusableBody(t *testing.T) {
	tests := []struct {
		name        string
		body        string
		contentType string
		status      int
		code        string
	}{
		{"not JSON", `{"fullName":`, "", http.StatusBadRequest, "BAD_REQUEST"},
		{"wrong field type", `{"fullName":42}`, "", http.StatusBadRequest, "BAD_REQUEST"},
		{"not a JSON content type", `fullName=Nimal`, "application/x-www-form-urlencoded", http.StatusUnsupportedMediaType, "UNSUPPORTED_MEDIA_TYPE"},
		{"too large", `{"fullName":"` + strings.Repeat("a", maxBodyBytes) + `"}`, "", http.StatusRequestEntityTooLarge, "PAYLOAD_TOO_LARGE"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			h := newProfileHarness(t)

			response := h.send(t, http.MethodPatch, "/users/me", h.customerToken(t), test.body, test.contentType)

			if response.Code != test.status || errorCode(t, response) != test.code {
				t.Fatalf("status = %d body = %s, want %d %s", response.Code, response.Body, test.status, test.code)
			}
			if h.service.updateCalls != 0 {
				t.Fatal("an unusable body reached the service")
			}
		})
	}
}

func TestUpdateProfile_refusesAccountsThatAreGoneOrDisabledAndHidesServerErrors(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{"the account no longer exists", service.ErrUserNotFound, http.StatusUnauthorized, "UNAUTHENTICATED"},
		{"the account was deactivated", service.ErrAccountDisabled, http.StatusForbidden, "ACCOUNT_DISABLED"},
		{"a server fault", errors.New("connection to db.internal:5432 refused"), http.StatusInternalServerError, "INTERNAL_ERROR"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			h := newProfileHarness(t)
			h.service.writeErr = test.err

			response := h.send(t, http.MethodPatch, "/users/me", h.customerToken(t), `{"fullName":"Nimal P. Perera"}`, "")

			if response.Code != test.status || errorCode(t, response) != test.code {
				t.Fatalf("status = %d body = %s, want %d %s", response.Code, response.Body, test.status, test.code)
			}
			if strings.Contains(response.Body.String(), "db.internal") {
				t.Fatalf("the response leaks the underlying error: %s", response.Body)
			}
		})
	}
}

func TestChangePassword_requiresAuthentication(t *testing.T) {
	h := newProfileHarness(t)

	response := h.send(t, http.MethodPut, "/users/me/password", "", `{"currentPassword":`+typedCurrent+`,"newPassword":`+typedNew+`}`, "")

	if response.Code != http.StatusUnauthorized || errorCode(t, response) != "UNAUTHENTICATED" {
		t.Fatalf("status = %d body = %s, want 401 UNAUTHENTICATED", response.Code, response.Body)
	}
	if h.service.passwordCalls != 0 {
		t.Fatal("an unauthenticated request reached the service")
	}
}

func TestChangePassword_answers204AndSignsTheBrowserOut(t *testing.T) {
	h := newProfileHarness(t)

	response := h.send(t, http.MethodPut, "/users/me/password", h.customerToken(t),
		`{"currentPassword":`+typedCurrent+`,"newPassword":`+typedNew+`,"userId":"someone-else"}`, "")

	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d body = %s, want 204", response.Code, response.Body)
	}
	if response.Body.Len() != 0 {
		t.Errorf("a 204 response has a body: %s", response.Body)
	}
	if h.service.lastWriteUserID != profileUserID {
		t.Fatalf("service changed the password of %q, want the user in the token", h.service.lastWriteUserID)
	}
	if h.service.passwordInput.CurrentPassword != "Str0ngPass!" || h.service.passwordInput.NewPassword != "N3wStrongPass!" {
		t.Fatalf("service got %+v, want the submitted passwords", h.service.passwordInput)
	}

	cleared := map[string]bool{}
	for _, cookie := range response.Result().Cookies() {
		if cookie.MaxAge < 0 && cookie.Value == "" && cookie.HttpOnly {
			cleared[cookie.Name] = true
		}
	}
	if !cleared["sd_access"] || !cleared["sd_refresh"] {
		t.Errorf("cleared cookies = %v, want sd_access and sd_refresh expired", cleared)
	}
	if got := response.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", got)
	}
}

func TestChangePassword_failuresKeepTheSessionAndNeverEchoPasswords(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{"wrong current password", service.ErrInvalidCredentials, http.StatusUnauthorized, "INVALID_CREDENTIALS"},
		{"a Google-only account", service.ErrPasswordNotSet, http.StatusConflict, "PASSWORD_NOT_SET"},
		{"the account no longer exists", service.ErrUserNotFound, http.StatusUnauthorized, "UNAUTHENTICATED"},
		{"the account was deactivated", service.ErrAccountDisabled, http.StatusForbidden, "ACCOUNT_DISABLED"},
		{"a weak new password", &service.ValidationError{Issues: []service.Issue{{Field: "newPassword", Issue: "must be at least 8 characters"}}}, http.StatusUnprocessableEntity, "VALIDATION_ERROR"},
		{"a server fault", errors.New("connection to db.internal:5432 refused"), http.StatusInternalServerError, "INTERNAL_ERROR"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			h := newProfileHarness(t)
			h.service.writeErr = test.err

			response := h.send(t, http.MethodPut, "/users/me/password", h.customerToken(t),
				`{"currentPassword":`+typedCurrent+`,"newPassword":`+typedNew+`}`, "")

			if response.Code != test.status || errorCode(t, response) != test.code {
				t.Fatalf("status = %d body = %s, want %d %s", response.Code, response.Body, test.status, test.code)
			}
			if len(response.Result().Cookies()) != 0 {
				t.Errorf("a failed change touched the cookies: %v", response.Result().Cookies())
			}
			for _, secret := range []string{"Str0ngPass!", "N3wStrongPass!", "db.internal"} {
				if strings.Contains(response.Body.String(), secret) {
					t.Errorf("the response contains %q: %s", secret, response.Body)
				}
			}
		})
	}
}

func TestChangePassword_refusesAnUnusableBody(t *testing.T) {
	tests := []struct {
		name        string
		body        string
		contentType string
		status      int
		code        string
	}{
		{"not JSON", `{"currentPassword":`, "", http.StatusBadRequest, "BAD_REQUEST"},
		{"not a JSON content type", `currentPassword=x`, "text/plain", http.StatusUnsupportedMediaType, "UNSUPPORTED_MEDIA_TYPE"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			h := newProfileHarness(t)

			response := h.send(t, http.MethodPut, "/users/me/password", h.customerToken(t), test.body, test.contentType)

			if response.Code != test.status || errorCode(t, response) != test.code {
				t.Fatalf("status = %d body = %s, want %d %s", response.Code, response.Body, test.status, test.code)
			}
			if h.service.passwordCalls != 0 {
				t.Fatal("an unusable body reached the service")
			}
		})
	}
}

func TestChangePassword_isLimitedMuchHarderThanTheRest(t *testing.T) {
	h := newProfileHarness(t)
	h.service.writeErr = service.ErrInvalidCredentials
	token := h.customerToken(t)
	body := `{"currentPassword":"Wrong-Pass-1","newPassword":` + typedNew + `}`

	var last *httptest.ResponseRecorder
	for range passwordChangesPerMinute + 1 {
		last = h.send(t, http.MethodPut, "/users/me/password", token, body, "")
	}

	if last.Code != http.StatusTooManyRequests || errorCode(t, last) != "RATE_LIMITED" {
		t.Fatalf("request %d status = %d, want 429 RATE_LIMITED", passwordChangesPerMinute+1, last.Code)
	}
	if last.Header().Get("Retry-After") == "" {
		t.Error("a rate limited response must carry Retry-After")
	}
	if h.service.passwordCalls != passwordChangesPerMinute {
		t.Errorf("service was called %d times, want %d", h.service.passwordCalls, passwordChangesPerMinute)
	}

	other := h.send(t, http.MethodPut, "/users/me/password", h.token(t, "3f1c7a52-0000-4000-8000-0000000000d4", auth.RoleCustomer), body, "")
	if other.Code == http.StatusTooManyRequests {
		t.Fatal("another user was limited too")
	}
	if got := h.get(t, "/users/me", token); got.Code != http.StatusOK {
		t.Fatalf("reading the profile was limited by the password limit: status = %d", got.Code)
	}
}
