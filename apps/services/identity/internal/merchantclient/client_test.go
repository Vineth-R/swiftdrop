package merchantclient

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Vineth-R/swiftdrop/packages/go-common/requestid"
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
	status, reason, err := client.Approval(context.Background(), "user-1")
	if err != nil || status != "APPROVED" || reason != "" {
		t.Fatalf("Approval = %q, %q, %v", status, reason, err)
	}
	status, reason, err = client.Approval(context.Background(), "missing")
	if err != nil || status != "" || reason != "" {
		t.Fatalf("missing Approval = %q, %q, %v", status, reason, err)
	}
}

func TestApprovalReturnsTheAdministratorsReason(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"merchantId":"m-1","status":"REJECTED","reason":"Registration number not found."}}`))
	}))
	defer server.Close()
	client, err := New(server.URL, "token", time.Second)
	if err != nil {
		t.Fatal(err)
	}

	status, reason, err := client.Approval(context.Background(), "user-1")

	if err != nil || status != "REJECTED" || reason != "Registration number not found." {
		t.Fatalf("Approval = %q, %q, %v", status, reason, err)
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
	if _, _, err := client.Approval(context.Background(), "user-1"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Approval error = %v, want ErrUnavailable", err)
	}
}

func TestApprovalPropagatesInboundRequestID(t *testing.T) {
	var got string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get(requestid.Header)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"status":"APPROVED"}}`))
	}))
	defer server.Close()

	client, err := New(server.URL, "token", time.Second)
	if err != nil {
		t.Fatal(err)
	}

	ctx := requestid.WithContext(context.Background(), "inbound-request-id")
	if _, _, err := client.Approval(ctx, "user-1"); err != nil {
		t.Fatal(err)
	}
	if got != "inbound-request-id" {
		t.Fatalf("X-Request-Id = %q, want the inbound request ID", got)
	}
}

func TestApprovalGeneratesRequestIDWithoutInbound(t *testing.T) {
	var got string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get(requestid.Header)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"status":"APPROVED"}}`))
	}))
	defer server.Close()

	client, err := New(server.URL, "token", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := client.Approval(context.Background(), "user-1"); err != nil {
		t.Fatal(err)
	}
	if !requestid.Valid(got) {
		t.Fatalf("X-Request-Id = %q, want a generated valid ID", got)
	}
}

func TestNewRejectsInvalidBaseURL(t *testing.T) {
	if _, err := New("file:///tmp", "token", time.Second); err == nil {
		t.Fatal("invalid base URL was accepted")
	}
}
