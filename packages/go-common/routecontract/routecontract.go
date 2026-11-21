// Package routecontract snapshots the routes a Gin engine registers so the
// contract tests can compare them with the OpenAPI specifications and the
// Nginx gateway. It contains infrastructure only: no service business rules.
package routecontract

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

const (
	// ClassificationPublic is a business route exposed through Nginx.
	ClassificationPublic = "public"
	// ClassificationInternal is a service-to-service route that Nginx must not expose.
	ClassificationInternal = "internal"
	// ClassificationOperational is a probe or metrics endpoint registered by the
	// service entrypoint rather than the business router.
	ClassificationOperational = "operational"
	// ClassificationProbe is an access-test probe that is intentionally
	// undocumented and deliberately reachable through the gateway.
	ClassificationProbe = "probe"

	updateEnvironment  = "UPDATE_ROUTES"
	inventoryDirectory = "contracts/routes"
)

var operationalReasons = map[string]string{
	"/health":  "service liveness probe registered by the service entrypoint; not routed through the gateway",
	"/ready":   "service readiness probe registered by the service entrypoint; not routed through the gateway",
	"/metrics": "internal Prometheus scrape endpoint; not routed through the gateway",
	"/healthz": "deprecated liveness alias registered by httpx.RegisterHealth; not routed through the gateway",
	"/readyz":  "deprecated readiness alias registered by httpx.RegisterHealth; not routed through the gateway",
}

// Route is one registered HTTP method and path.
type Route struct {
	Method         string `json:"method"`
	Path           string `json:"path"`
	Classification string `json:"classification"`
	Documented     bool   `json:"documented"`
	Gateway        bool   `json:"gateway"`
	Reason         string `json:"reason,omitempty"`
}

// Inventory is the committed route snapshot for one service.
type Inventory struct {
	Service string  `json:"service"`
	Routes  []Route `json:"routes"`
}

// Probe describes a registered access-test probe that is deliberately kept out
// of the OpenAPI contract but remains reachable through the gateway.
type Probe struct {
	Method string
	Path   string
	Reason string
}

// Policy describes the explicit exceptions for one service inventory.
type Policy struct {
	Service string
	Probes  []Probe
}

// Snapshot returns the deterministic route inventory for a Gin engine.
func Snapshot(router *gin.Engine, policy Policy) Inventory {
	routes := make([]Route, 0)
	seen := make(map[string]struct{})
	for _, registered := range router.Routes() {
		method := strings.ToUpper(strings.TrimSpace(registered.Method))
		if !allowedMethod(method) {
			continue
		}
		path := normalizePath(registered.Path)
		key := method + " " + path
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		routes = append(routes, classify(method, path, policy))
	}
	sort.Slice(routes, func(left, right int) bool {
		if routes[left].Path == routes[right].Path {
			return routes[left].Method < routes[right].Method
		}
		return routes[left].Path < routes[right].Path
	})
	return Inventory{Service: policy.Service, Routes: routes}
}

// Verify compares the runtime inventory with the committed inventory. Run the
// test with UPDATE_ROUTES=1 to regenerate the file after an intentional change.
func Verify(t testing.TB, router *gin.Engine, policy Policy) {
	t.Helper()
	got := Snapshot(router, policy)
	path, err := GoldenPath(policy.Service)
	if err != nil {
		t.Fatalf("resolve route inventory path: %v", err)
	}
	if os.Getenv(updateEnvironment) == "1" {
		if err := Save(path, got); err != nil {
			t.Fatalf("update route inventory %s: %v", path, err)
		}
		t.Logf("updated %s with %d routes", path, len(got.Routes))
		return
	}
	want, err := Load(path)
	if err != nil {
		t.Fatalf("read route inventory %s: %v", path, err)
	}
	if err := Compare(want, got); err != nil {
		t.Fatalf("%v\n\nRun UPDATE_ROUTES=1 go test -run TestRouteInventory ./apps/services/... from the repository root to regenerate the committed inventory.", err)
	}
}

// GoldenPath returns the committed inventory path for a service.
func GoldenPath(service string) (string, error) {
	root, err := repositoryRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, inventoryDirectory, service+".json"), nil
}

// Load reads a committed inventory.
func Load(path string) (Inventory, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Inventory{}, err
	}
	var inventory Inventory
	if err := json.Unmarshal(data, &inventory); err != nil {
		return Inventory{}, fmt.Errorf("decode %s: %w", path, err)
	}
	return inventory, nil
}

// Save writes a deterministic inventory file.
func Save(path string, inventory Inventory) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(inventory, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

// Compare reports missing, unexpected and changed routes with a readable diff.
func Compare(want, got Inventory) error {
	if want.Service != got.Service {
		return fmt.Errorf("route inventory service is %q, want %q", got.Service, want.Service)
	}
	wantRoutes := indexRoutes(want.Routes)
	gotRoutes := indexRoutes(got.Routes)

	var missing, unexpected, changed []string
	for key, route := range wantRoutes {
		current, exists := gotRoutes[key]
		if !exists {
			missing = append(missing, key)
			continue
		}
		if !reflect.DeepEqual(route, current) {
			changed = append(changed, describeChange(key, route, current))
		}
	}
	for key := range gotRoutes {
		if _, exists := wantRoutes[key]; !exists {
			unexpected = append(unexpected, key)
		}
	}
	if len(missing) == 0 && len(unexpected) == 0 && len(changed) == 0 {
		return nil
	}
	sort.Strings(missing)
	sort.Strings(unexpected)
	sort.Strings(changed)

	var b strings.Builder
	fmt.Fprintf(&b, "route inventory for %s is out of date:", got.Service)
	writeSection(&b, "missing (committed but not registered)", missing)
	writeSection(&b, "unexpected (registered but not committed)", unexpected)
	writeSection(&b, "changed", changed)
	return errors.New(strings.TrimRight(b.String(), "\n"))
}

func classify(method, path string, policy Policy) Route {
	if reason, ok := probeReason(method, path, policy.Probes); ok {
		return Route{
			Method: method, Path: path, Classification: ClassificationProbe,
			Documented: false, Gateway: true, Reason: reason,
		}
	}
	if reason, ok := operationalReasons[path]; ok {
		return Route{
			Method: method, Path: path, Classification: ClassificationOperational,
			Documented: false, Gateway: false, Reason: reason,
		}
	}
	if path == "/internal" || strings.HasPrefix(path, "/internal/") {
		return Route{
			Method: method, Path: path, Classification: ClassificationInternal,
			Documented: true, Gateway: false,
		}
	}
	return Route{
		Method: method, Path: path, Classification: ClassificationPublic,
		Documented: true, Gateway: true,
	}
}

func probeReason(method, path string, probes []Probe) (string, bool) {
	for _, probe := range probes {
		if strings.EqualFold(probe.Method, method) && normalizePath(probe.Path) == path {
			return probe.Reason, true
		}
	}
	return "", false
}

func normalizePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return "/"
	}
	segments := strings.Split(path, "/")
	for index, segment := range segments {
		switch {
		case strings.HasPrefix(segment, ":"):
			segments[index] = "{" + strings.TrimPrefix(segment, ":") + "}"
		case strings.HasPrefix(segment, "*"):
			segments[index] = "{" + strings.TrimPrefix(segment, "*") + "}"
		}
	}
	normalized := strings.Join(segments, "/")
	if len(normalized) > 1 {
		normalized = strings.TrimRight(normalized, "/")
	}
	if normalized == "" {
		return "/"
	}
	return normalized
}

func allowedMethod(method string) bool {
	switch method {
	case "GET", "POST", "PUT", "PATCH", "DELETE":
		return true
	default:
		return false
	}
}

func indexRoutes(routes []Route) map[string]Route {
	indexed := make(map[string]Route, len(routes))
	for _, route := range routes {
		indexed[route.Method+" "+route.Path] = route
	}
	return indexed
}

func describeChange(key string, want, got Route) string {
	var changes []string
	if want.Classification != got.Classification {
		changes = append(changes, fmt.Sprintf("classification %s -> %s", want.Classification, got.Classification))
	}
	if want.Documented != got.Documented {
		changes = append(changes, fmt.Sprintf("documented %t -> %t", want.Documented, got.Documented))
	}
	if want.Gateway != got.Gateway {
		changes = append(changes, fmt.Sprintf("gateway %t -> %t", want.Gateway, got.Gateway))
	}
	if want.Reason != got.Reason {
		changes = append(changes, "reason changed")
	}
	return fmt.Sprintf("%s (%s)", key, strings.Join(changes, ", "))
}

func writeSection(b *strings.Builder, heading string, entries []string) {
	if len(entries) == 0 {
		return
	}
	fmt.Fprintf(b, "\n\n%s:", heading)
	for _, entry := range entries {
		fmt.Fprintf(b, "\n  - %s", entry)
	}
}

func repositoryRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("could not find the repository root (go.mod)")
		}
		dir = parent
	}
}
