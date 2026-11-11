package pricingclient

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestZoneUsesTheInternalToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/zones/zone-1" || r.Header.Get("X-Internal-Token") != "internal-token" || r.Header.Get("X-Request-Id") == "" {
			t.Errorf("request = %s token=%q", r.URL.Path, r.Header.Get("X-Internal-Token"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"id":"zone-1","code":"CMB","name":"Colombo","isActive":true}}`)
	}))
	defer server.Close()

	client, err := New(server.URL, "internal-token", time.Second)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	zone, err := client.Zone(context.Background(), "zone-1")
	if err != nil {
		t.Fatalf("Zone() error = %v", err)
	}
	if zone.Name != "Colombo" || !zone.IsActive {
		t.Fatalf("zone = %+v", zone)
	}
}

func TestZoneFailures(t *testing.T) {
	for _, test := range []struct {
		name   string
		status int
		body   string
	}{
		{"not found", http.StatusNotFound, `{"error":{"code":"NOT_FOUND"}}`},
		{"unavailable", http.StatusServiceUnavailable, `{}`},
		{"invalid envelope", http.StatusOK, `not-json`},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(test.status)
				_, _ = io.WriteString(w, test.body)
			}))
			defer server.Close()
			client, _ := New(server.URL, "", time.Second)
			if _, err := client.Zone(context.Background(), "zone-1"); !errors.Is(err, ErrUnavailable) {
				t.Fatalf("Zone() error = %v, want ErrUnavailable", err)
			}
		})
	}
}

func TestNewValidatesBaseURLAndTimeout(t *testing.T) {
	if _, err := New("parcel:8083", "", time.Second); err == nil {
		t.Fatal("New() accepted a relative URL")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"id":"x","code":"CMB","name":"Colombo","isActive":true}}`)
	}))
	defer server.Close()
	client, err := New(server.URL, "", 0)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if _, err := client.Zone(context.Background(), "x"); err != nil {
		t.Fatalf("Zone() error = %v", err)
	}
}
