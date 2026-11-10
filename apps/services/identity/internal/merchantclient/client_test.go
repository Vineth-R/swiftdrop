package merchantclient

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestApprovalReadsStatusAndFailsClosed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Internal-Token") != "token" || r.Header.Get("X-Request-Id") == "" {
			t.Fatal("internal headers were not sent")
		}
		if r.URL.Path != "/internal/merchants/owners/user-1/approval" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"status":"APPROVED"}}`))
	}))
	defer server.Close()

	client, err := New(server.URL, "token", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	status, err := client.Approval(context.Background(), "user-1")
	if err != nil || status != "APPROVED" {
		t.Fatalf("Approval = %q, %v", status, err)
	}
	status, err = client.Approval(context.Background(), "missing")
	if err != nil || status != "" {
		t.Fatalf("missing Approval = %q, %v", status, err)
	}
}

func TestApprovalMapsServerFailureToUnavailable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()
	client, err := New(server.URL, "token", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Approval(context.Background(), "user-1"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Approval error = %v, want ErrUnavailable", err)
	}
}

func TestNewRejectsInvalidBaseURL(t *testing.T) {
	if _, err := New("file:///tmp", "token", time.Second); err == nil {
		t.Fatal("invalid base URL was accepted")
	}
}
