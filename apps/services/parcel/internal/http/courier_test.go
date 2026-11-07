package servicehttp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Vineth-R/swiftdrop/apps/services/parcel/internal/courier"
	"github.com/Vineth-R/swiftdrop/packages/go-common/auth"
	"github.com/Vineth-R/swiftdrop/packages/go-common/httpx"
)

const courierTestID = "00000000-0000-4000-8000-000000000071"
const courierParcelID = "00000000-0000-4000-8000-000000000072"

type courierFixture struct {
	filter           courier.Filter
	userID, parcelID string
	err              error
	calls            int
}

func (f *courierFixture) List(ctx context.Context, filter courier.Filter) (courier.Page, error) {
	f.filter = filter
	f.calls++
	if err := ctx.Err(); err != nil {
		return courier.Page{}, err
	}
	return courier.Page{Items: []courier.Delivery{}}, f.err
}

func (f *courierFixture) Detail(ctx context.Context, userID, parcelID string) (courier.Detail, error) {
	f.userID = userID
	f.parcelID = parcelID
	f.calls++
	if err := ctx.Err(); err != nil {
		return courier.Detail{}, err
	}
	return courier.Detail{History: []courier.History{}}, f.err
}

func courierRequest(t *testing.T, router *gin.Engine, path, userID string, role auth.Role, bearer bool) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if role != "" {
		manager, err := auth.NewTokenManager(reportTestSecret, time.Hour)
		if err != nil {
			t.Fatal(err)
		}
		token, _, err := manager.Issue(userID, role)
		if err != nil {
			t.Fatal(err)
		}
		if bearer {
			req.Header.Set("Authorization", "Bearer "+token)
		} else {
			req.AddCookie(&http.Cookie{Name: "sd_access", Value: token})
		}
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestCourierAccess(t *testing.T) {
	for _, path := range []string{"/courier/deliveries", "/courier/deliveries/" + courierParcelID} {
		for _, tc := range []struct {
			name string
			role auth.Role
			want int
		}{
			{name: "signed out", want: 401},
			{name: "customer", role: auth.RoleCustomer, want: 403},
			{name: "merchant", role: auth.RoleMerchant, want: 403},
			{name: "dispatcher", role: auth.RoleDispatcher, want: 403},
			{name: "admin is not a courier", role: auth.RoleAdmin, want: 403},
			{name: "courier", role: auth.RoleCourier, want: 200},
		} {
			t.Run(path+"/"+tc.name, func(t *testing.T) {
				store := &courierFixture{}
				router := reportRouter(t, Options{AccessSecret: reportTestSecret, Courier: store})
				rec := courierRequest(t, router, path, courierTestID, tc.role, false)
				if rec.Code != tc.want {
					t.Fatalf("status %d: %s", rec.Code, rec.Body)
				}
				if rec.Header().Get("Cache-Control") != "no-store" {
					t.Fatal("private responses must not be cached")
				}
				if tc.want != 200 && store.calls != 0 {
					t.Fatal("unauthorized call reached store")
				}
			})
		}
	}
}

func TestCourierQueryValidation(t *testing.T) {
	for _, query := range []string{
		"date=2026-02-30", "date=2026-11-07T00:00:00Z", "date=", "date=2026-11-07&date=2026-11-08",
		"status=UNKNOWN", "status=delivered", "status=", "status=FAILED&status=DELIVERED",
		"page=0", "page=-1", "page=abc", "page=1.5", "page=", "page=9223372036854775807",
		"pageSize=0", "pageSize=101", "pageSize=-1", "pageSize=abc", "pageSize=", "page=1&page=2",
		"courierId=" + courierTestID,
	} {
		t.Run(query, func(t *testing.T) {
			store := &courierFixture{}
			router := reportRouter(t, Options{AccessSecret: reportTestSecret, Courier: store})
			rec := courierRequest(t, router, "/courier/deliveries?"+query, courierTestID, auth.RoleCourier, false)
			if rec.Code != 422 || !strings.Contains(rec.Body.String(), `"code":"VALIDATION_ERROR"`) {
				t.Fatalf("invalid query: %d %s", rec.Code, rec.Body)
			}
			if store.calls != 0 {
				t.Fatal("invalid query reached store")
			}
		})
	}
	store := &courierFixture{}
	router := reportRouter(t, Options{AccessSecret: reportTestSecret, Courier: store})
	rec := courierRequest(t, router, "/courier/deliveries?date=%zz", courierTestID, auth.RoleCourier, false)
	if rec.Code != 400 || store.calls != 0 {
		t.Fatalf("malformed query: %d %s", rec.Code, rec.Body)
	}
}

func TestCourierListClaimsAndEnvelope(t *testing.T) {
	for _, status := range []string{"", "CREATED", "PICKED_UP", "IN_TRANSIT", "OUT_FOR_DELIVERY", "DELIVERED", "FAILED"} {
		t.Run("status="+status, func(t *testing.T) {
			store := &courierFixture{}
			router := reportRouter(t, Options{AccessSecret: reportTestSecret, Courier: store})
			path := "/courier/deliveries?date=2026-11-07&page=2&pageSize=5"
			if status != "" {
				path += "&status=" + status
			}
			rec := courierRequest(t, router, path, courierTestID, auth.RoleCourier, true)
			if rec.Code != 200 {
				t.Fatalf("%d %s", rec.Code, rec.Body)
			}
			f := store.filter
			if f.CourierID != courierTestID || f.Page != 2 || f.PageSize != 5 || f.Status != status || f.Start.Format(time.RFC3339) != "2026-11-06T18:30:00Z" {
				t.Fatalf("incorrect filter %+v", f)
			}
			var response struct {
				Data []courier.Delivery `json:"data"`
				Meta httpx.SuccessMeta  `json:"meta"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			if response.Data == nil || len(response.Data) != 0 || response.Meta.RequestID == "" || response.Meta.Timestamp == "" {
				t.Fatalf("invalid envelope %s", rec.Body)
			}
			p := response.Meta.Pagination
			if p == nil || p.Page != 2 || p.PageSize != 5 || p.TotalItems != 0 || p.TotalPages != 0 {
				t.Fatalf("pagination %+v", p)
			}
		})
	}
	store := &courierFixture{}
	router := reportRouter(t, Options{AccessSecret: reportTestSecret, Courier: store})
	before := time.Now()
	rec := courierRequest(t, router, "/courier/deliveries", courierTestID, auth.RoleCourier, false)
	startBefore, _, err := courier.DayBounds("", before)
	if err != nil {
		t.Fatal(err)
	}
	startAfter, _, err := courier.DayBounds("", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if rec.Code != 200 || store.filter.Page != 1 || store.filter.PageSize != 20 {
		t.Fatalf("defaults: %d %+v", rec.Code, store.filter)
	}
	if !store.filter.Start.Equal(startBefore) && !store.filter.Start.Equal(startAfter) {
		t.Fatal("default date is not today in Colombo")
	}
}

func TestCourierDetailValidationAndErrors(t *testing.T) {
	for _, tc := range []struct {
		name, path, code string
		err              error
		want             int
	}{
		{name: "own detail", path: courierParcelID, want: 200},
		{name: "bad UUID", path: "bad-id", want: 400, code: "BAD_REQUEST"},
		{name: "missing", path: courierParcelID, err: courier.ErrNotFound, want: 404, code: "PARCEL_NOT_FOUND"},
		{name: "another courier", path: courierParcelID, err: courier.ErrForbidden, want: 403, code: "FORBIDDEN"},
		{name: "database unconfigured", path: courierParcelID, err: courier.ErrUnavailable, want: 503, code: "SERVICE_UNAVAILABLE"},
		{name: "safe server error", path: courierParcelID, err: errors.New("SELECT password from secret_table contact@example.com"), want: 500, code: "INTERNAL_ERROR"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &courierFixture{err: tc.err}
			router := reportRouter(t, Options{AccessSecret: reportTestSecret, Courier: store})
			rec := courierRequest(t, router, "/courier/deliveries/"+tc.path, courierTestID, auth.RoleCourier, false)
			if rec.Code != tc.want {
				t.Fatalf("%d %s", rec.Code, rec.Body)
			}
			if tc.code != "" && !strings.Contains(rec.Body.String(), `"code":"`+tc.code+`"`) {
				t.Fatalf("wrong code %s", rec.Body)
			}
			if strings.Contains(rec.Body.String(), "secret_table") || strings.Contains(rec.Body.String(), "contact@example.com") {
				t.Fatal("internal error leaked")
			}
			if tc.want == 400 && store.calls != 0 {
				t.Fatal("bad ID reached store")
			}
			if tc.want == 200 && (store.userID != courierTestID || store.parcelID != courierParcelID) {
				t.Fatal("verified claims not used")
			}
		})
	}
}

func TestCourierInvalidTokensAndCustomCookie(t *testing.T) {
	store := &courierFixture{}
	router := reportRouter(t, Options{AccessSecret: reportTestSecret, Courier: store, CookieName: "courier_access"})
	manager, err := auth.NewTokenManager(reportTestSecret, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	valid, _, err := manager.Issue(courierTestID, auth.RoleCourier)
	if err != nil {
		t.Fatal(err)
	}
	expiredManager, err := auth.NewTokenManager(reportTestSecret, time.Minute,
		auth.WithClock(func() time.Time { return time.Now().Add(-time.Hour) }))
	if err != nil {
		t.Fatal(err)
	}
	expired, _, err := expiredManager.Issue(courierTestID, auth.RoleCourier)
	if err != nil {
		t.Fatal(err)
	}
	invalidSubject, _, err := manager.Issue("not-a-uuid", auth.RoleCourier)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, token, cookie string
		want                int
	}{
		{name: "custom cookie", token: valid, cookie: "courier_access", want: 200},
		{name: "wrong cookie", token: valid, cookie: "sd_access", want: 401},
		{name: "tampered token", token: valid + "x", cookie: "courier_access", want: 401},
		{name: "expired token", token: expired, cookie: "courier_access", want: 401},
		{name: "invalid verified subject", token: invalidSubject, cookie: "courier_access", want: 401},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := store.calls
			req := httptest.NewRequest(http.MethodGet, "/courier/deliveries", nil)
			req.AddCookie(&http.Cookie{Name: tc.cookie, Value: tc.token})
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if rec.Code != tc.want {
				t.Fatalf("%d %s", rec.Code, rec.Body)
			}
			if tc.want != 200 && store.calls != before {
				t.Fatal("bad credentials reached store")
			}
		})
	}
}

func TestCourierFailClosed(t *testing.T) {
	for _, path := range []string{"/courier/deliveries", "/courier/deliveries/" + courierParcelID} {
		t.Run(path, func(t *testing.T) {
			store := &courierFixture{}
			router := reportRouter(t, Options{Courier: store})
			if rec := courierRequest(t, router, path, courierTestID, auth.RoleCourier, false); rec.Code != 503 || store.calls != 0 {
				t.Fatalf("missing secret: %d", rec.Code)
			}
			router = reportRouter(t, Options{AccessSecret: reportTestSecret})
			if rec := courierRequest(t, router, path, courierTestID, auth.RoleCourier, false); rec.Code != 503 {
				t.Fatalf("missing database: %d", rec.Code)
			}
		})
	}
}
