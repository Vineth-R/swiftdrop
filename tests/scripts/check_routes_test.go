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

// The gateway used to answer a no-slash path with an absolute Location built
// from its own port, which sent a caller on :8080 to a closed port.
func TestCheckRoutesRejectsAnAbsoluteRedirect(t *testing.T) {
	if !routeCheckToolsAvailable(t) {
		return
	}
	server := newRouteCheckServerWith(t, routeStub{validPricing405: true, absoluteRedirects: true})
	defer server.Close()

	output, err := runCheckRoutes(t, server.URL)
	if err == nil {
		t.Fatalf("check-routes accepted a redirect with an absolute Location:\n%s", output)
	}
	if !strings.Contains(output, "FAIL  no-slash redirect is relative") {
		t.Fatalf("check-routes failed without naming the absolute redirect:\n%s", output)
	}
}

// The explicit no-slash rules used to drop the query string, so a filter and a
// page number were lost on the way to the right path.
func TestCheckRoutesRejectsARedirectThatDropsTheQuery(t *testing.T) {
	if !routeCheckToolsAvailable(t) {
		return
	}
	server := newRouteCheckServerWith(t, routeStub{validPricing405: true, dropsQuery: true})
	defer server.Close()

	output, err := runCheckRoutes(t, server.URL)
	if err == nil {
		t.Fatalf("check-routes accepted a redirect that lost the query string:\n%s", output)
	}
	if !strings.Contains(output, "FAIL  no-slash redirect keeps the query") {
		t.Fatalf("check-routes failed without naming the lost query:\n%s", output)
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

// routeStub is how the stub gateway behaves. The zero value (apart from
// validPricing405) is the gateway as it should be: relative redirects that keep
// the query string.
type routeStub struct {
	validPricing405 bool
	// absoluteRedirects answers with a Location that carries a host and no port,
	// as the gateway did before absolute_redirect was turned off.
	absoluteRedirects bool
	// dropsQuery leaves the query string off the Location, as the explicit
	// no-slash rules did before they ended with $is_args$args.
	dropsQuery bool
}

func newRouteCheckServer(t *testing.T, validPricing405 bool) *httptest.Server {
	t.Helper()
	return newRouteCheckServerWith(t, routeStub{validPricing405: validPricing405})
}

// redirect answers the way the gateway does for a path that needs its slash.
func (s routeStub) redirect(w http.ResponseWriter, r *http.Request, status int) {
	location := r.URL.Path + "/"
	if r.URL.RawQuery != "" && !s.dropsQuery {
		location += "?" + r.URL.RawQuery
	}
	if s.absoluteRedirects {
		location = "http://localhost" + location
	}
	w.Header().Set("Location", location)
	w.WriteHeader(status)
}

func newRouteCheckServerWith(t *testing.T, stub routeStub) *httptest.Server {
	t.Helper()
	validPricing405 := stub.validPricing405
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz", "/":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
		case "/api/v1/couriers", "/api/v1/merchants":
			// An explicit no-slash rule in the gateway: 308.
			stub.redirect(w, r, http.StatusPermanentRedirect)
		case "/api/v1/dispatch":
			// A prefix location with no exact rule: Nginx's own 301.
			stub.redirect(w, r, http.StatusMovedPermanently)
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
