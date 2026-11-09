package servicehttp

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Vineth-R/swiftdrop/apps/services/parcel/internal/tracking"
	"github.com/Vineth-R/swiftdrop/packages/go-common/auth"
)

type trackingFixture struct{ err error }

func (s trackingFixture) Lookup(context.Context, string) (tracking.Result, error) {
	return tracking.Result{TrackingNumber: "SDK7M2Q9XP4", Status: "CREATED", PickupCity: "Colombo", DropoffCity: "Kandy", LastUpdatedAt: time.Now().UTC(), Timeline: []tracking.Event{}}, s.err
}

func (s trackingFixture) LookupAuthorized(_ context.Context, number, _ string, _ bool) (tracking.Result, error) {
	if s.err != nil {
		return tracking.Result{}, s.err
	}
	return tracking.Result{TrackingNumber: number, Status: "CREATED", PickupCity: "Colombo", DropoffCity: "Kandy", LastUpdatedAt: time.Now().UTC(), Timeline: []tracking.Event{}}, nil
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
	return trackingEventResult(), nil
}

func (trackingEventFixture) LookupAuthorized(context.Context, string, string, bool) (tracking.Result, error) {
	return trackingEventResult(), nil
}

func trackingEventResult() tracking.Result {
	at := time.Now().UTC()
	return tracking.Result{TrackingNumber: "SDK7M2Q9XP4", Status: "CREATED", PickupCity: "Colombo", DropoffCity: "Kandy", LastUpdatedAt: at,
		Timeline: []tracking.Event{{Status: "CREATED", At: at}}}
}

const ownedTrackingSecret = "parcel-owned-tracking-test-secret-32"

type scopeTrackingFixture struct {
	calls int
	user  string
	staff bool
}

func (s *scopeTrackingFixture) Lookup(context.Context, string) (tracking.Result, error) {
	return tracking.Result{}, tracking.ErrNotFound
}

func (s *scopeTrackingFixture) LookupAuthorized(_ context.Context, number, user string, staff bool) (tracking.Result, error) {
	s.calls++
	s.user = user
	s.staff = staff
	if !staff && user != "customer-a" {
		return tracking.Result{}, tracking.ErrNotFound
	}
	return tracking.Result{TrackingNumber: number, Status: "IN_TRANSIT", PickupCity: "Colombo", DropoffCity: "Kandy", LastUpdatedAt: time.Now().UTC(), Timeline: []tracking.Event{}}, nil
}

func trackingToken(t *testing.T, user string, role auth.Role) string {
	t.Helper()
	manager, err := auth.NewTokenManager(ownedTrackingSecret, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	token, _, err := manager.Issue(user, role)
	if err != nil {
		t.Fatal(err)
	}
	return token
}

func ownedTrackingRequest(t *testing.T, router http.Handler, token string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, "/parcels/tracking/SDK7M2Q9XP4", nil)
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}

func TestOwnedTrackingEnforcesIdentityAndRoles(t *testing.T) {
	store := &scopeTrackingFixture{}
	router, err := New(slog.New(slog.NewTextHandler(io.Discard, nil)), Options{
		Tracking: store, AccessSecret: ownedTrackingSecret,
	})
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name       string
		token      string
		wantStatus int
		wantUser   string
		wantStaff  bool
		wantCalls  int
	}{
		{name: "no credentials", wantStatus: http.StatusUnauthorized},
		{name: "invalid token", token: "not-a-valid-token", wantStatus: http.StatusUnauthorized},
		{name: "customer owner", token: trackingToken(t, "customer-a", auth.RoleCustomer), wantStatus: http.StatusOK, wantUser: "customer-a", wantCalls: 1},
		{name: "another customer", token: trackingToken(t, "customer-b", auth.RoleCustomer), wantStatus: http.StatusNotFound, wantUser: "customer-b", wantCalls: 2},
		{name: "dispatcher", token: trackingToken(t, "dispatcher-1", auth.RoleDispatcher), wantStatus: http.StatusOK, wantUser: "dispatcher-1", wantStaff: true, wantCalls: 3},
		{name: "admin", token: trackingToken(t, "admin-1", auth.RoleAdmin), wantStatus: http.StatusOK, wantUser: "admin-1", wantStaff: true, wantCalls: 4},
		{name: "merchant", token: trackingToken(t, "merchant-1", auth.RoleMerchant), wantStatus: http.StatusForbidden, wantCalls: 4},
		{name: "courier", token: trackingToken(t, "courier-1", auth.RoleCourier), wantStatus: http.StatusForbidden, wantCalls: 4},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := ownedTrackingRequest(t, router, test.token)
			if recorder.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d: %s", recorder.Code, test.wantStatus, recorder.Body.String())
			}
			if test.wantUser != "" && (store.user != test.wantUser || store.staff != test.wantStaff) {
				t.Fatalf("lookup identity = %q staff=%v, want %q staff=%v", store.user, store.staff, test.wantUser, test.wantStaff)
			}
			if store.calls != test.wantCalls {
				t.Fatalf("lookup calls = %d, want %d", store.calls, test.wantCalls)
			}
			if test.wantStatus == http.StatusNotFound && strings.Contains(recorder.Body.String(), "customer-a") {
				t.Fatal("not-found response disclosed another customer")
			}
		})
	}
}

func TestOwnedTrackingFailsClosedWithoutSecret(t *testing.T) {
	router, err := New(slog.New(slog.NewTextHandler(io.Discard, nil)), Options{Tracking: &scopeTrackingFixture{}})
	if err != nil {
		t.Fatal(err)
	}
	recorder := ownedTrackingRequest(t, router, "")
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", recorder.Code)
	}
}

func TestOwnedTrackingRateLimit(t *testing.T) {
	router, err := New(slog.New(slog.NewTextHandler(io.Discard, nil)), Options{
		Tracking: &scopeTrackingFixture{}, AccessSecret: ownedTrackingSecret,
	})
	if err != nil {
		t.Fatal(err)
	}
	token := trackingToken(t, "customer-a", auth.RoleCustomer)
	for index := 0; index < 31; index++ {
		recorder := ownedTrackingRequest(t, router, token)
		if index < 30 && recorder.Code != http.StatusOK {
			t.Fatalf("request %d refused too soon: %d", index+1, recorder.Code)
		}
		if index == 30 && (recorder.Code != http.StatusTooManyRequests || recorder.Header().Get("Retry-After") == "") {
			t.Fatalf("rate limit not enforced: %d %v", recorder.Code, recorder.Header())
		}
	}
}

func TestOwnedTrackingResponseExposesOnlyPublicFields(t *testing.T) {
	router, err := New(slog.New(slog.NewTextHandler(io.Discard, nil)), Options{
		Tracking: trackingEventFixture{}, AccessSecret: ownedTrackingSecret,
	})
	if err != nil {
		t.Fatal(err)
	}
	recorder := ownedTrackingRequest(t, router, trackingToken(t, "customer-a", auth.RoleCustomer))
	if recorder.Code != http.StatusOK {
		t.Fatal(recorder.Code, recorder.Body.String())
	}
	var response struct {
		Data map[string]json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	assertKeys(t, response.Data, "dropoffCity", "lastUpdatedAt", "pickupCity", "status", "timeline", "trackingNumber")
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
