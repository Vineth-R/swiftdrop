package merchantclient

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Vineth-R/swiftdrop/packages/go-common/requestid"
)

func TestMerchantReturnsContactAndSendsInternalHeaders(t *testing.T) {
	var gotPath, gotToken, gotRequestID string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotToken = r.Header.Get("X-Internal-Token")
		gotRequestID = r.Header.Get("X-Request-Id")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"merchantId":"4f1c7a52-0000-4000-8000-000000000002","ownerUserId":"5f1c7a52-0000-4000-8000-000000000003","businessName":"Test Merchant","contactEmail":"merchant@example.test","status":"APPROVED"},"meta":{"requestId":"req_01"}}`))
	}))
	defer server.Close()

	client, err := New(server.URL, "shared-token", time.Second)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	merchant, err := client.Merchant(context.Background(), "4f1c7a52-0000-4000-8000-000000000002")
	if err != nil {
		t.Fatalf("Merchant() error = %v", err)
	}
	if merchant.ID != "4f1c7a52-0000-4000-8000-000000000002" || merchant.BusinessName != "Test Merchant" ||
		merchant.ContactEmail != "merchant@example.test" || merchant.Status != "APPROVED" {
		t.Fatalf("Merchant() = %+v", merchant)
	}
	if gotPath != "/internal/merchants/4f1c7a52-0000-4000-8000-000000000002/contact" {
		t.Fatalf("path = %q", gotPath)
	}
	if gotToken != "shared-token" {
		t.Fatalf("X-Internal-Token = %q", gotToken)
	}
	if !requestid.Valid(gotRequestID) {
		t.Fatalf("X-Request-Id = %q, want a valid request ID", gotRequestID)
	}
}

func TestMerchantPropagatesRequestID(t *testing.T) {
	var gotRequestID string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotRequestID = r.Header.Get("X-Request-Id")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"merchantId":"4f1c7a52-0000-4000-8000-000000000002","contactEmail":"merchant@example.test","status":"APPROVED"}}`))
	}))
	defer server.Close()

	client, _ := New(server.URL, "token", time.Second)
	ctx := requestid.WithContext(context.Background(), "req_01HF")
	if _, err := client.Merchant(ctx, "4f1c7a52-0000-4000-8000-000000000002"); err != nil {
		t.Fatalf("Merchant() error = %v", err)
	}
	if gotRequestID != "req_01HF" {
		t.Fatalf("X-Request-Id = %q, want the inbound request ID", gotRequestID)
	}
}

func TestMerchantRejectsRedirectsAndDoesNotForwardInternalToken(t *testing.T) {
	var targetCalls int
	var targetToken string
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		targetCalls++
		targetToken = r.Header.Get("X-Internal-Token")
		_, _ = w.Write([]byte(`{"data":{"contactEmail":"redirect@example.test","status":"APPROVED"}}`))
	}))
	defer target.Close()

	merchant := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+r.URL.Path, http.StatusFound)
	}))
	defer merchant.Close()

	client, _ := New(merchant.URL, "shared-token", time.Second)
	got, err := client.Merchant(context.Background(), "4f1c7a52-0000-4000-8000-000000000002")
	if err == nil || !errors.Is(err, ErrInvalidResponse) {
		t.Fatalf("error = %v, want ErrInvalidResponse", err)
	}
	if targetCalls != 0 || targetToken != "" {
		t.Fatalf("redirect target calls = %d token = %q", targetCalls, targetToken)
	}
	if got != (Merchant{}) {
		t.Fatalf("Merchant() = %+v, want zero value", got)
	}
}

func TestMerchantClassifiesResponses(t *testing.T) {
	tests := []struct {
		name      string
		status    int
		body      string
		want      error
		wantRetry bool
	}{
		{name: "not found", status: http.StatusNotFound, body: `{"error":{"code":"NOT_FOUND"}}`, want: ErrNotFound},
		{name: "unauthorized", status: http.StatusUnauthorized, body: `{"error":{"code":"UNAUTHENTICATED"}}`, want: ErrUnauthorized},
		{name: "forbidden", status: http.StatusForbidden, body: `{"error":{"code":"FORBIDDEN"}}`, want: ErrUnauthorized},
		{name: "bad request", status: http.StatusBadRequest, body: `{"error":{"code":"BAD_REQUEST"}}`, want: ErrInvalidResponse},
		{name: "method not allowed", status: http.StatusMethodNotAllowed, body: `{}`, want: ErrInvalidResponse},
		{name: "unavailable", status: http.StatusServiceUnavailable, body: `{}`, want: ErrUnavailable, wantRetry: true},
		{name: "internal error", status: http.StatusInternalServerError, body: `{}`, want: ErrUnavailable, wantRetry: true},
		{name: "invalid json", status: http.StatusOK, body: `not-json`, want: ErrInvalidResponse},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(test.status)
				_, _ = w.Write([]byte(test.body))
			}))
			defer server.Close()

			client, _ := New(server.URL, "token", time.Second)
			_, err := client.Merchant(context.Background(), "4f1c7a52-0000-4000-8000-000000000002")
			if !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
			if test.wantRetry && !errors.Is(err, ErrUnavailable) {
				t.Fatalf("error = %v, want a retryable unavailable error", err)
			}
		})
	}
}

func TestMerchantTimesOut(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(100 * time.Millisecond)
		_, _ = w.Write([]byte(`{"data":{"contactEmail":"merchant@example.test","status":"APPROVED"}}`))
	}))
	defer server.Close()

	client, _ := New(server.URL, "token", 10*time.Millisecond)
	_, err := client.Merchant(context.Background(), "4f1c7a52-0000-4000-8000-000000000002")
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("error = %v, want ErrUnavailable", err)
	}
}

func TestMerchantDoesNotLeakContactDataInErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"data":{"contactEmail":"merchant@example.test"}}`))
	}))
	defer server.Close()

	client, _ := New(server.URL, "token", time.Second)
	_, err := client.Merchant(context.Background(), "4f1c7a52-0000-4000-8000-000000000002")
	if err == nil {
		t.Fatal("Merchant() error = nil")
	}
	if strings.Contains(err.Error(), "merchant@example.test") || strings.Contains(err.Error(), "token") {
		t.Fatalf("error leaked sensitive data: %v", err)
	}
}

func TestNewRejectsInvalidBaseURL(t *testing.T) {
	for _, value := range []string{"", "merchant:8084", "ftp://merchant:8084", "http://"} {
		if _, err := New(value, "token", time.Second); err == nil {
			t.Fatalf("New(%q) error = nil", value)
		}
	}
}
