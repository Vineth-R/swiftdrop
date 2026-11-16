package routecontract

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestSnapshotNormalizesAliasesAndClassifiesRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := func(*gin.Context) {}
	router.GET("/parcels/:parcelId", handler)
	router.GET("/parcels/:parcelId/", handler)
	router.GET("/healthz", handler)
	router.GET("/internal/parcels", handler)
	router.GET("/dispatch/probe", handler)
	router.DELETE("/parcels/:parcelId", handler)

	inventory := Snapshot(router, Policy{
		Service: "parcel",
		Probes: []Probe{{
			Method: "GET", Path: "/dispatch/probe", Reason: "access-test probe",
		}},
	})

	if inventory.Service != "parcel" {
		t.Fatalf("service = %q, want parcel", inventory.Service)
	}
	if len(inventory.Routes) != 5 {
		t.Fatalf("routes = %d, want 5: %+v", len(inventory.Routes), inventory.Routes)
	}

	want := map[string]Route{
		"GET /parcels/{parcelId}": {
			Method: "GET", Path: "/parcels/{parcelId}",
			Classification: ClassificationPublic, Documented: true, Gateway: true,
		},
		"DELETE /parcels/{parcelId}": {
			Method: "DELETE", Path: "/parcels/{parcelId}",
			Classification: ClassificationPublic, Documented: true, Gateway: true,
		},
		"GET /healthz": {
			Method: "GET", Path: "/healthz",
			Classification: ClassificationOperational, Documented: false, Gateway: false,
			Reason: operationalReasons["/healthz"],
		},
		"GET /internal/parcels": {
			Method: "GET", Path: "/internal/parcels",
			Classification: ClassificationInternal, Documented: true, Gateway: false,
		},
		"GET /dispatch/probe": {
			Method: "GET", Path: "/dispatch/probe",
			Classification: ClassificationProbe, Documented: false, Gateway: true,
			Reason: "access-test probe",
		},
	}
	for _, route := range inventory.Routes {
		key := route.Method + " " + route.Path
		expected, ok := want[key]
		if !ok {
			t.Fatalf("unexpected route %s", key)
		}
		if route != expected {
			t.Fatalf("%s = %+v, want %+v", key, route, expected)
		}
	}

	if inventory.Routes[0].Path != "/dispatch/probe" {
		t.Fatalf("routes are not sorted by path: %+v", inventory.Routes)
	}
}

func TestCompareReportsMissingUnexpectedAndChangedRoutes(t *testing.T) {
	want := Inventory{Service: "parcel", Routes: []Route{
		{Method: "GET", Path: "/parcels", Classification: ClassificationPublic, Documented: true, Gateway: true},
		{Method: "GET", Path: "/zones", Classification: ClassificationPublic, Documented: true, Gateway: true},
	}}
	got := Inventory{Service: "parcel", Routes: []Route{
		{Method: "GET", Path: "/parcels", Classification: ClassificationPublic, Documented: true, Gateway: true},
		{Method: "POST", Path: "/parcels", Classification: ClassificationPublic, Documented: true, Gateway: true},
		{Method: "GET", Path: "/internal/parcels", Classification: ClassificationInternal, Documented: true, Gateway: false},
	}}
	got.Routes[0].Gateway = false

	err := Compare(want, got)
	if err == nil {
		t.Fatal("Compare() = nil, want a diff")
	}
	message := err.Error()
	for _, fragment := range []string{
		"route inventory for parcel is out of date",
		"missing (committed but not registered)",
		"GET /zones",
		"unexpected (registered but not committed)",
		"POST /parcels",
		"GET /internal/parcels",
		"changed",
		"GET /parcels (gateway true -> false)",
	} {
		if !strings.Contains(message, fragment) {
			t.Fatalf("Compare() message missing %q:\n%s", fragment, message)
		}
	}
}

func TestCompareAcceptsIdenticalInventories(t *testing.T) {
	inventory := Inventory{Service: "parcel", Routes: []Route{{
		Method: "GET", Path: "/parcels", Classification: ClassificationPublic,
		Documented: true, Gateway: true,
	}}}
	if err := Compare(inventory, inventory); err != nil {
		t.Fatalf("Compare() = %v, want nil", err)
	}
}

func TestSaveIsDeterministicAndLoadable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "parcel.json")
	inventory := Inventory{Service: "parcel", Routes: []Route{
		{Method: "GET", Path: "/parcels", Classification: ClassificationPublic, Documented: true, Gateway: true},
		{Method: "GET", Path: "/healthz", Classification: ClassificationOperational, Documented: false, Gateway: false, Reason: operationalReasons["/healthz"]},
	}}
	if err := Save(path, inventory); err != nil {
		t.Fatalf("Save() = %v", err)
	}
	first, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := Save(path, inventory); err != nil {
		t.Fatalf("second Save() = %v", err)
	}
	second, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatalf("Save() is not deterministic:\n%s\n---\n%s", first, second)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load() = %v", err)
	}
	if err := Compare(inventory, loaded); err != nil {
		t.Fatalf("loaded inventory differs: %v", err)
	}
	if !strings.HasSuffix(string(first), "\n") {
		t.Fatal("inventory file does not end with a newline")
	}
}

func TestRepositoryRootFindsModule(t *testing.T) {
	root, err := repositoryRoot()
	if err != nil {
		t.Fatalf("repositoryRoot() = %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("repositoryRoot() = %q without go.mod: %v", root, err)
	}
}
