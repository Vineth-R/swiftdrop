package servicehttp

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/Vineth-R/swiftdrop/apps/services/identity/internal/service"
)

func TestVerifyEmailHandlerSuccessAndUniformFailure(t *testing.T) {
	h := newHarness(t)
	response := h.do(http.MethodPost, "/auth/verify-email", `{"token":"known-token"}`)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "verified") {
		t.Fatalf("verify status = %d body = %s", response.Code, response.Body)
	}

	h.service.err = service.ErrVerificationTokenInvalid
	response = h.do(http.MethodPost, "/auth/verify-email", `{"token":"bad-token"}`)
	if response.Code != http.StatusBadRequest || errorCode(t, response) != "INVALID_OR_EXPIRED_VERIFICATION_TOKEN" {
		t.Fatalf("invalid token status = %d body = %s", response.Code, response.Body)
	}

	response = h.do(http.MethodPost, "/auth/verify-email", `{`)
	if response.Code != http.StatusBadRequest || errorCode(t, response) != "BAD_REQUEST" {
		t.Fatalf("malformed body status = %d body = %s", response.Code, response.Body)
	}
}

func TestResendVerificationHandlerIsGeneric(t *testing.T) {
	h := newHarness(t)
	for _, email := range []string{"nimal@example.com", "nobody@example.com"} {
		response := h.do(http.MethodPost, "/auth/resend-verification", `{"email":"`+email+`"}`)
		if response.Code != http.StatusAccepted || !strings.Contains(response.Body.String(), "If an account needs verification") {
			t.Fatalf("resend %s status = %d body = %s", email, response.Code, response.Body)
		}
		if strings.Contains(response.Body.String(), email) {
			t.Fatal("resend response revealed the submitted email")
		}
	}

	h.service.err = &service.ValidationError{Issues: []service.Issue{{Field: "email", Issue: "must be a valid email address"}}}
	response := h.do(http.MethodPost, "/auth/resend-verification", `{"email":"not-an-email"}`)
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid email status = %d body = %s", response.Code, response.Body)
	}
}

func TestLoginTellsAMerchantOwnerWhereTheApplicationStands(t *testing.T) {
	h := newHarness(t)
	h.service.err = &service.MerchantInactiveError{
		Status: "REJECTED", Reason: "The registration number could not be verified.", SupportEmail: "support@swiftdrop.example",
	}

	response := h.do(http.MethodPost, "/auth/login", `{"email":"owner@shop.example","password":"Str0ngPass!"}`)

	if response.Code != http.StatusForbidden || errorCode(t, response) != "MERCHANT_ACCOUNT_INACTIVE" {
		t.Fatalf("status = %d body = %s", response.Code, response.Body)
	}
	var body struct {
		Error struct {
			Details []struct {
				Field string `json:"field"`
				Issue string `json:"issue"`
			} `json:"details"`
		} `json:"error"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	got := map[string]string{}
	for _, item := range body.Error.Details {
		got[item.Field] = item.Issue
	}
	want := map[string]string{
		"status":       "REJECTED",
		"reason":       "The registration number could not be verified.",
		"supportEmail": "support@swiftdrop.example",
	}
	if len(got) != len(want) {
		t.Fatalf("details = %v, want %v", got, want)
	}
	for field, issue := range want {
		if got[field] != issue {
			t.Fatalf("details[%s] = %q, want %q", field, got[field], issue)
		}
	}
}

func TestLoginLeavesOutDetailsThatDoNotExist(t *testing.T) {
	h := newHarness(t)
	h.service.err = &service.MerchantInactiveError{Status: "PENDING"}

	response := h.do(http.MethodPost, "/auth/login", `{"email":"owner@shop.example","password":"Str0ngPass!"}`)

	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d body = %s", response.Code, response.Body)
	}
	if strings.Contains(response.Body.String(), `"reason"`) || strings.Contains(response.Body.String(), `"supportEmail"`) {
		t.Fatalf("empty reason or support email was sent: %s", response.Body)
	}

	// The plain refusal has no details at all.
	h = newHarness(t)
	h.service.err = service.ErrMerchantAccountInactive
	response = h.do(http.MethodPost, "/auth/login", `{"email":"owner@shop.example","password":"Str0ngPass!"}`)
	if strings.Contains(response.Body.String(), `"details"`) {
		t.Fatalf("the plain refusal carried details: %s", response.Body)
	}
}

func TestLoginMapsVerificationAndMerchantErrors(t *testing.T) {
	tests := []struct {
		err    error
		status int
		code   string
	}{
		{service.ErrEmailVerificationRequired, http.StatusForbidden, "EMAIL_VERIFICATION_REQUIRED"},
		{service.ErrMerchantAccountInactive, http.StatusForbidden, "MERCHANT_ACCOUNT_INACTIVE"},
		{service.ErrServiceUnavailable, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE"},
	}
	for _, test := range tests {
		h := newHarness(t)
		h.service.err = test.err
		response := h.do(http.MethodPost, "/auth/login", `{"email":"nimal@example.com","password":"Str0ngPass!"}`)
		if response.Code != test.status || errorCode(t, response) != test.code {
			t.Fatalf("%v status = %d body = %s, want %d %s", test.err, response.Code, response.Body, test.status, test.code)
		}
	}
}
