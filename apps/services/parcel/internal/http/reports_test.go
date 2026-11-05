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

type reportAuth struct {
	role       auth.Role
	token      string
	cookieName string
	bearer     bool
}

func issueReportToken(t *testing.T, role auth.Role) string {
	t.Helper()
	manager, err := auth.NewTokenManager(reportTestSecret, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	token, _, err := manager.Issue("00000000-0000-0000-0000-000000000001", role)
	if err != nil {
		t.Fatal(err)
	}
	return token
}

func reportRequest(t *testing.T, router *gin.Engine, path string, options reportAuth) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, path, nil)
	token := options.token
	if token == "" && options.role != "" {
		token = issueReportToken(t, options.role)
	}
	if token != "" {
		if options.bearer {
			request.Header.Set("Authorization", "Bearer "+token)
		} else {
			cookieName := options.cookieName
			if cookieName == "" {
				cookieName = "sd_access"
			}
			request.AddCookie(&http.Cookie{Name: cookieName, Value: token})
		}
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
				recorder := reportRequest(t, router, path, reportAuth{role: tc.role})
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
	recorder := reportRequest(t, router, "/reports/daily", reportAuth{role: auth.RoleAdmin})

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
	recorder := reportRequest(t, router, "/reports/outcomes", reportAuth{role: auth.RoleAdmin})

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
		recorder := reportRequest(t, router, path, reportAuth{role: auth.RoleAdmin})
		if recorder.Code != http.StatusUnprocessableEntity || !strings.Contains(recorder.Body.String(), "VALIDATION_ERROR") {
			t.Fatalf("%s: status %d body %s", path, recorder.Code, recorder.Body.String())
		}
		if got := recorder.Header().Get("Cache-Control"); got != "no-store" {
			t.Fatalf("%s: Cache-Control = %q, want no-store", path, got)
		}
	}
}

func TestReportsDatabaseUnavailable(t *testing.T) {
	router := reportRouter(t, Options{Reports: reportFixture{err: reports.ErrUnavailable}, AccessSecret: reportTestSecret})
	recorder := reportRequest(t, router, "/reports/daily", reportAuth{role: auth.RoleAdmin})
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", recorder.Code)
	}
}

func TestReportsFailClosedWithoutSecret(t *testing.T) {
	router := reportRouter(t, Options{Reports: reportFixture{}})
	recorder := reportRequest(t, router, "/reports/daily", reportAuth{role: auth.RoleAdmin})
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", recorder.Code)
	}
}

func reportErrorCode(t *testing.T, recorder *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not an error envelope: %v (%s)", err, recorder.Body.String())
	}
	return body.Error.Code
}

func TestReportsCustomCookieName(t *testing.T) {
	router := reportRouter(t, Options{
		Reports:      reportFixture{daily: []reports.DailyRow{{Date: "2026-11-01", Created: 1}}},
		AccessSecret: reportTestSecret,
		CookieName:   "sd_ops",
	})
	token := issueReportToken(t, auth.RoleDispatcher)

	recorder := reportRequest(t, router, "/reports/daily", reportAuth{token: token, cookieName: "sd_ops"})
	if recorder.Code != http.StatusOK {
		t.Fatalf("custom cookie status = %d, want 200; body %s", recorder.Code, recorder.Body.String())
	}

	recorder = reportRequest(t, router, "/reports/daily", reportAuth{token: token, cookieName: "sd_access"})
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("default cookie status = %d, want 401; body %s", recorder.Code, recorder.Body.String())
	}
}

func TestReportsBearerAuthentication(t *testing.T) {
	router := reportRouter(t, Options{
		Reports:      reportFixture{daily: []reports.DailyRow{{Date: "2026-11-01", Created: 1}}},
		AccessSecret: reportTestSecret,
	})
	recorder := reportRequest(t, router, "/reports/daily", reportAuth{role: auth.RoleDispatcher, bearer: true})
	if recorder.Code != http.StatusOK {
		t.Fatalf("bearer status = %d, want 200; body %s", recorder.Code, recorder.Body.String())
	}
}

func TestReportsRejectsInvalidAndExpiredTokens(t *testing.T) {
	router := reportRouter(t, Options{
		Reports:      reportFixture{daily: []reports.DailyRow{{Date: "2026-11-01", Created: 1}}},
		AccessSecret: reportTestSecret,
	})

	invalid := reportRequest(t, router, "/reports/daily", reportAuth{token: "not-a-token"})
	if invalid.Code != http.StatusUnauthorized || reportErrorCode(t, invalid) != "UNAUTHENTICATED" {
		t.Fatalf("invalid token = %d %s, want 401 UNAUTHENTICATED", invalid.Code, reportErrorCode(t, invalid))
	}

	expiredManager, err := auth.NewTokenManager(reportTestSecret, time.Hour, auth.WithClock(func() time.Time {
		return time.Now().Add(-2 * time.Hour)
	}))
	if err != nil {
		t.Fatal(err)
	}
	expiredToken, _, err := expiredManager.Issue("00000000-0000-0000-0000-000000000001", auth.RoleAdmin)
	if err != nil {
		t.Fatal(err)
	}
	expired := reportRequest(t, router, "/reports/daily", reportAuth{token: expiredToken})
	if expired.Code != http.StatusUnauthorized || reportErrorCode(t, expired) != "TOKEN_EXPIRED" {
		t.Fatalf("expired token = %d %s, want 401 TOKEN_EXPIRED", expired.Code, reportErrorCode(t, expired))
	}
}

type recordingReportStore struct {
	last reports.Range
}

func (s *recordingReportStore) Daily(_ context.Context, rng reports.Range) ([]reports.DailyRow, error) {
	s.last = rng
	return []reports.DailyRow{{Date: "2026-11-01", Created: 1}}, nil
}

func (s *recordingReportStore) Outcomes(_ context.Context, rng reports.Range) (reports.Outcomes, error) {
	s.last = rng
	return reports.Outcomes{From: "2026-11-01", To: "2026-11-03"}, nil
}

func TestReportsPassParsedRangeToStore(t *testing.T) {
	store := &recordingReportStore{}
	router := reportRouter(t, Options{Reports: store, AccessSecret: reportTestSecret})

	for _, path := range []string{"/reports/daily", "/reports/outcomes"} {
		recorder := reportRequest(t, router, path+"?from=2026-11-01&to=2026-11-03", reportAuth{role: auth.RoleAdmin})
		if recorder.Code != http.StatusOK {
			t.Fatalf("%s: status = %d, want 200; body %s", path, recorder.Code, recorder.Body.String())
		}
		if got := store.last.From.Format("2006-01-02"); got != "2026-11-01" {
			t.Fatalf("%s: from = %s, want 2026-11-01", path, got)
		}
		if got := store.last.To.Format("2006-01-02"); got != "2026-11-03" {
			t.Fatalf("%s: to = %s, want 2026-11-03", path, got)
		}
	}
}
