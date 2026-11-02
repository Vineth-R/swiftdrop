package servicehttp

import (
	"context"
	"io"
	"log/slog"
	"net/http/httptest"
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
