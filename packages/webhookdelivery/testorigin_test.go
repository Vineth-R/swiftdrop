package webhookdelivery

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNormalizeTestOrigin(t *testing.T) {
	origin, err := NormalizeTestOrigin(" http://webhook-sink:8099/ ")
	if err != nil || origin != "http://webhook-sink:8099" {
		t.Fatalf("NormalizeTestOrigin = %q, %v", origin, err)
	}
	for _, raw := range []string{
		"",
		"webhook-sink:8099",
		"ftp://webhook-sink:8099",
		"http://user:pass@webhook-sink:8099",
		"http://webhook-sink:8099/path",
	} {
		if _, err := NormalizeTestOrigin(raw); err == nil {
			t.Fatalf("NormalizeTestOrigin(%q) was accepted", raw)
		}
	}
}

func TestTestOnlySenderAllowsTheConfiguredOriginOnly(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	sender, err := NewTestOnlySender(server.URL, "")
	if err != nil {
		t.Fatalf("NewTestOnlySender: %v", err)
	}
	event := Event{ID: "event", Type: EventTypeStatusChanged}
	result := sender.Send(context.Background(), Target{URL: server.URL + "/hook", Secret: "whsec_test"}, event)
	if !result.Success || result.StatusCode != http.StatusNoContent || calls != 1 {
		t.Fatalf("test-origin send = %+v, calls=%d", result, calls)
	}

	other := sender.Send(context.Background(), Target{URL: strings.Replace(server.URL, "127.0.0.1", "localhost", 1) + "/hook"}, event)
	if other.Success {
		t.Fatalf("a different origin was accepted: %+v", other)
	}
}
