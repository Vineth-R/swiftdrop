package identityclient

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestContactReturnsContactAndSendsInternalHeaders(t *testing.T) {
	var gotPath, gotToken, gotRequestID string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotToken = r.Header.Get("X-Internal-Token")
		gotRequestID = r.Header.Get("X-Request-Id")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"fullName":"Test Customer","email":"customer@example.test","phone":""},"meta":{"requestId":"req_01"}}`))
	}))
	defer server.Close()

	client, err := New(server.URL, "shared-token", time.Second)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	contact, err := client.Contact(context.Background(), "3f1c7a52-0000-4000-8000-000000000001")
	if err != nil {
		t.Fatalf("Contact() error = %v", err)
	}
	if contact.FullName != "Test Customer" || contact.Email != "customer@example.test" || contact.Phone != "" {
		t.Fatalf("Contact() = %+v", contact)
	}
	if gotPath != "/internal/users/3f1c7a52-0000-4000-8000-000000000001/contact" {
		t.Fatalf("path = %q", gotPath)
	}
	if gotToken != "shared-token" {
		t.Fatalf("X-Internal-Token = %q", gotToken)
	}
	if !validRequestID(gotRequestID) {
		t.Fatalf("X-Request-Id = %q, want a generated request ID", gotRequestID)
	}
}

func TestContactRejectsRedirectsAndDoesNotForwardInternalToken(t *testing.T) {
	var targetCalls int
	var targetToken string
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		targetCalls++
		targetToken = r.Header.Get("X-Internal-Token")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"fullName":"Redirect Target","email":"redirect@example.test"}}`))
	}))
	defer target.Close()

	identity := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+r.URL.Path, http.StatusFound)
	}))
	defer identity.Close()

	client, err := New(identity.URL, "shared-token", time.Second)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	contact, err := client.Contact(context.Background(), "3f1c7a52-0000-4000-8000-000000000001")
	if err == nil {
		t.Fatal("Contact() error = nil")
	}
	if !errors.Is(err, ErrInvalidResponse) {
		t.Fatalf("error = %v, want ErrInvalidResponse", err)
	}
	if errors.Is(err, ErrUnavailable) {
		t.Fatalf("error = %v, want a permanent classification", err)
	}
	if targetCalls != 0 {
		t.Fatalf("redirect target calls = %d, want 0", targetCalls)
	}
	if targetToken != "" {
		t.Fatalf("redirect target received X-Internal-Token = %q", targetToken)
	}
	if contact != (Contact{}) {
		t.Fatalf("Contact() = %+v, want zero value", contact)
	}
	if strings.Contains(err.Error(), "shared-token") || strings.Contains(err.Error(), "redirect@example.test") {
		t.Fatalf("error leaked sensitive data: %v", err)
	}
}

func validRequestID(value string) bool {
	return len(value) > 0 && len(value) <= 128
}

func TestContactClassifiesResponses(t *testing.T) {
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
		{name: "unavailable", status: http.StatusInternalServerError, body: `{}`, want: ErrUnavailable, wantRetry: true},
		{name: "invalid json", status: http.StatusOK, body: `not-json`, want: ErrInvalidResponse},
		{name: "empty email", status: http.StatusOK, body: `{"data":{"email":""}}`, want: ErrInvalidResponse},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(test.status)
				_, _ = w.Write([]byte(test.body))
			}))
			defer server.Close()

			client, _ := New(server.URL, "token", time.Second)
			_, err := client.Contact(context.Background(), "3f1c7a52-0000-4000-8000-000000000001")
			if !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
			if test.wantRetry && !errors.Is(err, ErrUnavailable) {
				t.Fatalf("error = %v, want a retryable unavailable error", err)
			}
		})
	}
}

func TestContactTimesOut(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(100 * time.Millisecond)
		_, _ = w.Write([]byte(`{"data":{"email":"customer@example.test"}}`))
	}))
	defer server.Close()

	client, _ := New(server.URL, "token", 10*time.Millisecond)
	_, err := client.Contact(context.Background(), "3f1c7a52-0000-4000-8000-000000000001")
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("error = %v, want ErrUnavailable", err)
	}
}

func TestContactDoesNotLeakContactDataInErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"data":{"email":"customer@example.test"}}`))
	}))
	defer server.Close()

	client, _ := New(server.URL, "token", time.Second)
	_, err := client.Contact(context.Background(), "3f1c7a52-0000-4000-8000-000000000001")
	if err == nil {
		t.Fatal("Contact() error = nil")
	}
	if strings.Contains(err.Error(), "customer@example.test") || strings.Contains(err.Error(), "token") {
		t.Fatalf("error leaked sensitive data: %v", err)
	}
}

func TestNewRejectsInvalidBaseURL(t *testing.T) {
	for _, value := range []string{"", "identity:8081", "ftp://identity:8081", "http://"} {
		if _, err := New(value, "token", time.Second); err == nil {
			t.Fatalf("New(%q) error = nil", value)
		}
	}
}
