package pricingclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Vineth-R/swiftdrop/packages/go-common/requestid"
)

func TestZonePropagatesInboundRequestID(t *testing.T) {
	var got string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get(requestid.Header)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"id":"zone-1","code":"CMB","name":"Colombo","isActive":true}}`))
	}))
	defer server.Close()

	client, err := New(server.URL, "token", time.Second)
	if err != nil {
		t.Fatal(err)
	}

	ctx := requestid.WithContext(context.Background(), "inbound-request-id")
	if _, err := client.Zone(ctx, "zone-1"); err != nil {
		t.Fatal(err)
	}
	if got != "inbound-request-id" {
		t.Fatalf("X-Request-Id = %q, want the inbound request ID", got)
	}
}

func TestZoneGeneratesRequestIDWithoutInbound(t *testing.T) {
	var got string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get(requestid.Header)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"id":"zone-1","code":"CMB","name":"Colombo","isActive":true}}`))
	}))
	defer server.Close()

	client, err := New(server.URL, "token", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Zone(context.Background(), "zone-1"); err != nil {
		t.Fatal(err)
	}
	if !requestid.Valid(got) {
		t.Fatalf("X-Request-Id = %q, want a generated valid ID", got)
	}
}
