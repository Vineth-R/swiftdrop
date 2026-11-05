package servicehttp

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Vineth-R/swiftdrop/apps/services/parcel/internal/tracking"
)

type trackingFixture struct{ err error }

func (s trackingFixture) Lookup(context.Context, string) (tracking.Result, error) {
	return tracking.Result{TrackingNumber: "SDK7M2Q9XP4", Status: "CREATED", PickupCity: "Colombo", DropoffCity: "Kandy", LastUpdatedAt: time.Now().UTC(), Timeline: []tracking.Event{}}, s.err
}
func TestPublicTrackingContract(t *testing.T) {
	for _, test := range []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{name: "anonymous", status: 200}, {name: "unknown", err: tracking.ErrNotFound, status: 404, code: "TRACKING_NOT_FOUND"},
		{name: "invalid", err: tracking.ErrInvalid, status: 422, code: "VALIDATION_ERROR"},
		{name: "unavailable", err: tracking.ErrUnavailable, status: 503, code: "SERVICE_UNAVAILABLE"},
	} {
		t.Run(test.name, func(t *testing.T) {
			router, err := New(slog.New(slog.NewTextHandler(io.Discard, nil)), Options{Tracking: trackingFixture{err: test.err}})
			if err != nil {
				t.Fatal(err)
			}
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest("GET", "/tracking/SDK7M2Q9XP4", nil))
			if recorder.Code != test.status || (test.code != "" && !strings.Contains(recorder.Body.String(), test.code)) {
				t.Fatal(recorder.Code, recorder.Body.String())
			}
		})
	}
}
func TestTrackingRateLimitIgnoresUntrustedHeaders(t *testing.T) {
	router, err := New(slog.New(slog.NewTextHandler(io.Discard, nil)), Options{Tracking: trackingFixture{}})
	if err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 31; index++ {
		request := httptest.NewRequest("GET", "/tracking/SDK7M2Q9XP4", nil)
		request.RemoteAddr = "198.51.100.1:1234"
		request.Header.Set("X-Real-IP", "203.0.113."+string(rune('A'+index)))
		request.Header.Set("X-Forwarded-For", "203.0.113.99")
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		if index < 30 && recorder.Code != 200 {
			t.Fatal("request refused too soon")
		}
		if index == 30 && (recorder.Code != 429 || recorder.Header().Get("Retry-After") == "") {
			t.Fatal("rate limit bypassed")
		}
	}
}

// An allowlist, so a new field fails here whatever it is called: a name, phone
// or address can only reach the public response by changing this test.
func TestPublicTrackingResponseExposesOnlyPublicFields(t *testing.T) {
	router, err := New(slog.New(slog.NewTextHandler(io.Discard, nil)), Options{Tracking: trackingEventFixture{}})
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest("GET", "/tracking/SDK7M2Q9XP4", nil))
	if recorder.Code != 200 {
		t.Fatal(recorder.Code, recorder.Body.String())
	}
	var response struct {
		Data map[string]json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	assertKeys(t, response.Data, "dropoffCity", "lastUpdatedAt", "pickupCity", "status", "timeline", "trackingNumber")
	var timeline []map[string]json.RawMessage
	if err := json.Unmarshal(response.Data["timeline"], &timeline); err != nil {
		t.Fatal(err)
	}
	if len(timeline) != 1 {
		t.Fatalf("timeline has %d events, want 1", len(timeline))
	}
	assertKeys(t, timeline[0], "at", "status")
}

type trackingEventFixture struct{}

func (trackingEventFixture) Lookup(context.Context, string) (tracking.Result, error) {
	at := time.Now().UTC()
	return tracking.Result{TrackingNumber: "SDK7M2Q9XP4", Status: "CREATED", PickupCity: "Colombo", DropoffCity: "Kandy", LastUpdatedAt: at,
		Timeline: []tracking.Event{{Status: "CREATED", At: at}}}, nil
}

func assertKeys(t *testing.T, object map[string]json.RawMessage, want ...string) {
	t.Helper()
	got := make([]string, 0, len(object))
	for key := range object {
		got = append(got, key)
	}
	slices.Sort(got)
	if !slices.Equal(got, want) {
		t.Fatalf("public fields %v, want exactly %v", got, want)
	}
}
