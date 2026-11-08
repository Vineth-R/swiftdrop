package scripts_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckRoutesAcceptsPricingMethodNotAllowed(t *testing.T) {
	if !routeCheckToolsAvailable(t) {
		return
	}
	server := newRouteCheckServer(t, true)
	defer server.Close()

	output, err := runCheckRoutes(t, server.URL)
	if err != nil {
		t.Fatalf("check-routes failed for a valid 405 response: %v\n%s", err, output)
	}
	if !strings.Contains(output, "all route checks passed") {
		t.Fatalf("check-routes did not report success:\n%s", output)
	}
}

func TestCheckRoutesRejectsUnexpectedMethodNotAllowed(t *testing.T) {
	if !routeCheckToolsAvailable(t) {
		return
	}
	server := newRouteCheckServer(t, false)
	defer server.Close()

	output, err := runCheckRoutes(t, server.URL)
	if err == nil {
		t.Fatalf("check-routes accepted a 405 with the wrong JSON error code:\n%s", output)
	}
	if !strings.Contains(output, "FAIL  pricing:admin-zones") {
		t.Fatalf("check-routes failed without identifying the bad route:\n%s", output)
	}
}

func routeCheckToolsAvailable(t *testing.T) bool {
	t.Helper()
	for _, tool := range []string{"bash", "curl"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s is not available; skipping route-check regression test", tool)
			return false
		}
	}
	return true
}

func runCheckRoutes(t *testing.T, baseURL string) (string, error) {
	t.Helper()
	script := filepath.Join("..", "..", "scripts", "check-routes.sh")
	cmd := exec.Command("bash", script)
	env := []string{}
	for _, value := range os.Environ() {
		if !strings.HasPrefix(value, "BASE_URL=") {
			env = append(env, value)
		}
	}
	cmd.Env = append(env, "BASE_URL="+baseURL)
	output, err := cmd.CombinedOutput()
	return string(output), err
}

func newRouteCheckServer(t *testing.T, validPricing405 bool) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz", "/":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
		case "/api/v1/admin/zones/probe":
			w.Header().Set("Allow", "PATCH")
			if validPricing405 {
				writeRouteError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED")
				return
			}
			writeRouteError(w, http.StatusMethodNotAllowed, "INTERNAL_ERROR")
		case "/api/v1/admin/users/probe", "/api/v1/admin/notification-logs", "/api/v1/admin/notification-logs/":
			writeRouteError(w, http.StatusUnauthorized, "UNAUTHORIZED")
		case "/api/v1/admin/merchants/probe":
			writeRouteError(w, http.StatusForbidden, "FORBIDDEN")
		case "/ws":
			// A non-WebSocket request is upgraded-required, not missing.
			w.Header().Set("Upgrade", "websocket")
			w.WriteHeader(http.StatusUpgradeRequired)
		case "/api/v1/ai/chat":
			// The AI service itself rejects an empty message with 422, which
			// proves the gateway reached the upstream.
			writeRouteError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR")
		default:
			// All remaining probes are intentionally unimplemented routes.
			writeRouteError(w, http.StatusNotFound, "NOT_FOUND")
		}
	}))
}

func writeRouteError(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = fmt.Fprintf(w, `{"error":{"code":%q,"message":"route check"}}`, code)
}
