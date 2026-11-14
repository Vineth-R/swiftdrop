package metrics

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func TestHTTPHandlerRecordsNormalisedRoute(t *testing.T) {
	registry := NewRegistry()
	handler := NewHTTPHandler(registry, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/parcels/11111111-1111-4111-8111-111111111111/history", nil))
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/health", nil))

	var output bytes.Buffer
	if _, err := registry.WriteTo(&output); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	if !strings.Contains(text, `swiftdrop_http_requests_total{method="GET",route="/parcels/:id/history",status="418"} 1`) {
		t.Fatalf("requests metric missing:\n%s", text)
	}
	if !strings.Contains(text, `swiftdrop_http_errors_total{method="GET",route="/parcels/:id/history",status="418"} 1`) {
		t.Fatalf("errors metric missing:\n%s", text)
	}
	if strings.Contains(text, "/health") {
		t.Fatalf("probe request was recorded:\n%s", text)
	}
}

func TestNormalizePath(t *testing.T) {
	tests := map[string]string{
		"/parcels/11111111-1111-4111-8111-111111111111": "/parcels/:id",
		"/ws":                   "/ws",
		"/merchants/me/webhook": "/merchants/me/webhook",
		"/files/0123456789abcdef0123456789abcdef": "/files/:id",
	}
	for input, want := range tests {
		if got := NormalizePath(input); got != want {
			t.Fatalf("NormalizePath(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestHTTPHandlerPreservesWebSocketUpgrade(t *testing.T) {
	registry := NewRegistry()
	server := httptest.NewServer(NewHTTPHandler(registry, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer conn.CloseNow()
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = conn.Write(ctx, websocket.MessageText, []byte("ok"))
	})))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatalf("Dial() through metrics wrapper = %v", err)
	}
	defer conn.CloseNow()
	_, data, err := conn.Read(ctx)
	if err != nil || string(data) != "ok" {
		t.Fatalf("Read() = %q, %v", data, err)
	}
}
