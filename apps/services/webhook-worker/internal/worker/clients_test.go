package worker

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Vineth-R/swiftdrop/packages/go-common/messaging"
	"github.com/Vineth-R/swiftdrop/packages/go-common/requestid"
)

func messageStub() messaging.Message { return messaging.Message{Body: []byte(`{}`)} }

func TestMerchantWebhook(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Internal-Token") != "tok" || r.Header.Get("X-Request-Id") == "" {
			t.Errorf("missing internal headers")
		}
		if r.URL.Path == "/internal/merchants/m-404/webhook" {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":{"code":"WEBHOOK_NOT_CONFIGURED"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"data":{"url":"https://x.example/h","signingSecret":"s","eventTypes":["parcel.status_changed"],"isActive":true,"merchantStatus":"APPROVED"}}`))
	}))
	defer srv.Close()
	c := NewMerchantClient(srv.URL, "tok", time.Second)

	w, err := c.Webhook(context.Background(), "m-1")
	if err != nil || w.URL != "https://x.example/h" || w.SigningSecret != "s" || !w.IsActive || w.MerchantStatus != "APPROVED" {
		t.Fatalf("Webhook() = %+v, %v", w, err)
	}
	if _, err := c.Webhook(context.Background(), "m-404"); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("404 error = %v, want ErrNotConfigured", err)
	}
}

func TestMerchantWebhookErrorsHideBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`secret-body`))
	}))
	defer srv.Close()
	_, err := NewMerchantClient(srv.URL, "tok", time.Second).Webhook(context.Background(), "m")
	if err == nil || strings.Contains(err.Error(), "secret-body") {
		t.Fatalf("error = %v", err)
	}
}

func TestMerchantRecordAttempt(t *testing.T) {
	var got Attempt
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/internal/merchants/m-1/webhook/attempts" {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"data":{"recorded":true}}`))
	}))
	defer srv.Close()

	err := NewMerchantClient(srv.URL, "tok", time.Second).RecordAttempt(context.Background(), "m-1",
		Attempt{EventID: "e", ParcelID: "p", TrackingNumber: "T", AttemptNumber: 2, DurationMs: 5})
	if err != nil || got.EventID != "e" || got.AttemptNumber != 2 {
		t.Fatalf("RecordAttempt() = %v, body %+v", err, got)
	}
}

func TestParcelExternalOrderID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/internal/parcels/p-1/summary":
			_, _ = w.Write([]byte(`{"data":{"externalOrderId":"ORD-1"}}`))
		case "/internal/parcels/p-null/summary":
			_, _ = w.Write([]byte(`{"data":{"externalOrderId":null}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	c := NewParcelClient(srv.URL, "tok", time.Second)

	if got, err := c.ExternalOrderID(context.Background(), "p-1"); err != nil || got == nil || *got != "ORD-1" {
		t.Fatalf("p-1 = %v, %v", got, err)
	}
	if got, err := c.ExternalOrderID(context.Background(), "p-null"); err != nil || got != nil {
		t.Fatalf("p-null = %v, %v", got, err)
	}
	if got, err := c.ExternalOrderID(context.Background(), "p-gone"); err != nil || got != nil {
		t.Fatalf("404 = %v, %v", got, err)
	}
}

func TestMerchantClientPropagatesInboundRequestID(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get(requestid.Header)
		_, _ = w.Write([]byte(`{"data":{"url":"https://x.example/h","signingSecret":"s","eventTypes":[],"isActive":true,"merchantStatus":"APPROVED"}}`))
	}))
	defer srv.Close()

	ctx := requestid.WithContext(context.Background(), "inbound-request-id")
	if _, err := NewMerchantClient(srv.URL, "tok", time.Second).Webhook(ctx, "m-1"); err != nil {
		t.Fatal(err)
	}
	if got != "inbound-request-id" {
		t.Fatalf("X-Request-Id = %q, want the inbound request ID", got)
	}
}

func TestMerchantClientGeneratesRequestIDWithoutInbound(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get(requestid.Header)
		_, _ = w.Write([]byte(`{"data":{"url":"https://x.example/h","signingSecret":"s","eventTypes":[],"isActive":true,"merchantStatus":"APPROVED"}}`))
	}))
	defer srv.Close()

	if _, err := NewMerchantClient(srv.URL, "tok", time.Second).Webhook(context.Background(), "m-1"); err != nil {
		t.Fatal(err)
	}
	if !requestid.Valid(got) {
		t.Fatalf("X-Request-Id = %q, want a generated valid ID", got)
	}
}

func TestClientDoesNotFollowRedirects(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://127.0.0.1:1/steal", http.StatusFound)
	}))
	defer srv.Close()
	if _, err := NewParcelClient(srv.URL, "tok", time.Second).ExternalOrderID(context.Background(), "p"); err == nil {
		t.Fatal("a redirect must be an error")
	}
}
