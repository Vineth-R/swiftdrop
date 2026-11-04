package servicehttp

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Vineth-R/swiftdrop/apps/services/parcel/internal/reports"
	"github.com/Vineth-R/swiftdrop/packages/go-common/auth"
)

const reportTestSecret = "report-test-secret-at-least-thirty-two-bytes"

type reportFixture struct {
	daily    []reports.DailyRow
	outcomes reports.Outcomes
	err      error
}

func (f reportFixture) Daily(context.Context, reports.Range) ([]reports.DailyRow, error) {
	return f.daily, f.err
}

func (f reportFixture) Outcomes(context.Context, reports.Range) (reports.Outcomes, error) {
	return f.outcomes, f.err
}

func reportRouter(t *testing.T, options Options) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router, err := New(slog.New(slog.NewTextHandler(io.Discard, nil)), options)
	if err != nil {
		t.Fatal(err)
	}
	return router
}

func reportRequest(t *testing.T, router *gin.Engine, path string, role auth.Role) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, path, nil)
	if role != "" {
		manager, err := auth.NewTokenManager(reportTestSecret, time.Hour)
		if err != nil {
			t.Fatal(err)
		}
		token, _, err := manager.Issue("00000000-0000-0000-0000-000000000001", role)
		if err != nil {
			t.Fatal(err)
		}
		request.AddCookie(&http.Cookie{Name: "sd_access", Value: token})
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}

func TestReportsAccessByRole(t *testing.T) {
	router := reportRouter(t, Options{
		Reports:      reportFixture{daily: []reports.DailyRow{{Date: "2026-11-01", Created: 4, Delivered: 3, Failed: 1}}},
		AccessSecret: reportTestSecret,
	})

	cases := []struct {
		name string
		role auth.Role
		want int
	}{
		{"no token", "", http.StatusUnauthorized},
		{"customer", auth.RoleCustomer, http.StatusForbidden},
		{"courier", auth.RoleCourier, http.StatusForbidden},
		{"merchant", auth.RoleMerchant, http.StatusForbidden},
		{"dispatcher", auth.RoleDispatcher, http.StatusOK},
		{"admin", auth.RoleAdmin, http.StatusOK},
	}
	for _, path := range []string{"/reports/daily", "/reports/outcomes"} {
		for _, tc := range cases {
			t.Run(path+" "+tc.name, func(t *testing.T) {
				recorder := reportRequest(t, router, path, tc.role)
				if recorder.Code != tc.want {
					t.Fatalf("status = %d, want %d; body %s", recorder.Code, tc.want, recorder.Body.String())
				}
				if got := recorder.Header().Get("Cache-Control"); got != "no-store" {
					t.Fatalf("Cache-Control = %q, want no-store", got)
				}
			})
		}
	}
}

func TestDailyResponseShape(t *testing.T) {
	router := reportRouter(t, Options{
		Reports:      reportFixture{daily: []reports.DailyRow{{Date: "2026-11-01", Created: 4, Delivered: 3, Failed: 1}}},
		AccessSecret: reportTestSecret,
	})
	recorder := reportRequest(t, router, "/reports/daily", auth.RoleAdmin)

	var body struct {
		Data []map[string]any `json:"data"`
		Meta map[string]any   `json:"meta"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Data) != 1 || body.Data[0]["date"] != "2026-11-01" || body.Data[0]["created"] != float64(4) {
		t.Fatalf("unexpected data %v", body.Data)
	}
	if body.Meta["requestId"] == "" || body.Meta["timestamp"] == nil {
		t.Fatalf("meta is incomplete: %v", body.Meta)
	}
}

func TestOutcomesResponseShape(t *testing.T) {
	totals, success, failure := reports.Summarise(10, 6, 2)
	router := reportRouter(t, Options{
		Reports: reportFixture{outcomes: reports.Outcomes{
			From: "2026-10-07", To: "2026-11-05",
			Totals: totals, SuccessRate: success, FailureRate: failure,
			TopFailureReasons: []reports.FailureReason{{Reason: "Recipient not available", Count: 2}},
		}},
		AccessSecret: reportTestSecret,
	})
	recorder := reportRequest(t, router, "/reports/outcomes", auth.RoleAdmin)

	body := recorder.Body.String()
	for _, want := range []string{`"successRate":75`, `"failureRate":25`, `"inProgress":2`, `"topFailureReasons":[{"reason":"Recipient not available","count":2}]`} {
		if !strings.Contains(body, want) {
			t.Fatalf("body is missing %s: %s", want, body)
		}
	}
}

func TestReportsRejectBadRange(t *testing.T) {
	router := reportRouter(t, Options{Reports: reportFixture{}, AccessSecret: reportTestSecret})
	for _, path := range []string{
		"/reports/daily?from=2026-11-05&to=2026-11-01",
		"/reports/daily?from=not-a-date",
		"/reports/outcomes?from=2026-01-01&to=2026-11-01",
	} {
		recorder := reportRequest(t, router, path, auth.RoleAdmin)
		if recorder.Code != http.StatusUnprocessableEntity || !strings.Contains(recorder.Body.String(), "VALIDATION_ERROR") {
			t.Fatalf("%s: status %d body %s", path, recorder.Code, recorder.Body.String())
		}
	}
}

func TestReportsDatabaseUnavailable(t *testing.T) {
	router := reportRouter(t, Options{Reports: reportFixture{err: reports.ErrUnavailable}, AccessSecret: reportTestSecret})
	recorder := reportRequest(t, router, "/reports/daily", auth.RoleAdmin)
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", recorder.Code)
	}
}

func TestReportsFailClosedWithoutSecret(t *testing.T) {
	router := reportRouter(t, Options{Reports: reportFixture{}})
	recorder := reportRequest(t, router, "/reports/daily", auth.RoleAdmin)
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", recorder.Code)
	}
}
