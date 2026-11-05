package servicehttp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Vineth-R/swiftdrop/apps/services/pricing/internal/pricing"
	"github.com/Vineth-R/swiftdrop/packages/go-common/auth"
	"github.com/golang-jwt/jwt/v5"
)

const testOrigin = "11111111-1111-4111-8111-111111111111"
const testDestination = "22222222-2222-4222-8222-222222222222"
const testSecret = "unit-test-only-signing-key-over-thirty-two-bytes"
const validBody = `{"originZoneId":"` + testOrigin + `","destinationZoneId":"` + testDestination + `","weightKg":2,"sizeCategory":"SMALL"}`

type fixtureStore struct {
	quoteErr    error
	mutationErr error
}

func (s fixtureStore) Quote(_ context.Context, in pricing.Input) (pricing.Quote, error) {
	return pricing.Calculate(in, 45000, 10000, 0), s.quoteErr
}
func (s fixtureStore) Zones(context.Context) ([]pricing.Zone, error) {
	return []pricing.Zone{{ID: testOrigin, Code: "CMB", Name: "Colombo", Districts: []string{}, IsActive: true}}, nil
}
func (s fixtureStore) Zone(context.Context, string) (pricing.Zone, error) {
	return pricing.Zone{ID: testOrigin, Code: "CMB", Name: "Colombo", IsActive: true}, nil
}
func (s fixtureStore) Mutate(context.Context, string, string, string, pricing.Mutation) (json.RawMessage, error) {
	return json.RawMessage(`{"id":"` + testOrigin + `"}`), s.mutationErr
}
func (s fixtureStore) List(context.Context, string, pricing.Filter) ([]json.RawMessage, int, error) {
	return []json.RawMessage{}, 0, nil
}
func adminToken(t *testing.T, role auth.Role) string {
	t.Helper()
	manager, err := auth.NewTokenManager(testSecret, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	raw, _, err := manager.Issue(testOrigin, role)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
func TestQuoteContracts(t *testing.T) {
	for _, test := range []struct {
		name, body string
		storeErr   error
		status     int
		code       string
	}{
		{name: "seed", body: validBody, status: 200},
		{name: "syntax", body: "{", status: 400, code: "BAD_REQUEST"},
		{name: "trailing JSON", body: validBody + ` {}`, status: 400, code: "BAD_REQUEST"},
		{name: "zero weight", body: strings.Replace(validBody, `"weightKg":2`, `"weightKg":0`, 1), status: 422, code: "VALIDATION_ERROR"},
		{name: "precision", body: strings.Replace(validBody, `"weightKg":2`, `"weightKg":0.001`, 1), status: 422, code: "VALIDATION_ERROR"},
		{name: "string weight", body: strings.Replace(validBody, `"weightKg":2`, `"weightKg":"2"`, 1), status: 422, code: "VALIDATION_ERROR"},
		{name: "zone", body: validBody, storeErr: pricing.ErrZoneNotFound, status: 404, code: "ZONE_NOT_FOUND"},
		{name: "route", body: validBody, storeErr: pricing.ErrRouteNotServiceable, status: 422, code: "ROUTE_NOT_SERVICEABLE"},
		{name: "band", body: validBody, storeErr: pricing.ErrWeightNotPriced, status: 422, code: "WEIGHT_NOT_PRICED"},
		{name: "database", body: validBody, storeErr: errors.New("private internal database detail"), status: 500, code: "INTERNAL_ERROR"},
	} {
		t.Run(test.name, func(t *testing.T) {
			router, err := New(slog.New(slog.NewTextHandler(io.Discard, nil)), Options{Store: fixtureStore{quoteErr: test.storeErr}})
			if err != nil {
				t.Fatal(err)
			}
			request := httptest.NewRequest("POST", "/pricing/estimate", strings.NewReader(test.body))
			request.Header.Set("Content-Type", "application/json")
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)
			if recorder.Code != test.status {
				t.Fatalf("status %d, body %s", recorder.Code, recorder.Body)
			}
			if test.code != "" && !strings.Contains(recorder.Body.String(), test.code) {
				t.Fatal(recorder.Body.String())
			}
			if strings.Contains(recorder.Body.String(), "private internal") {
				t.Fatal("internal error leaked")
			}
			if test.status == 200 && !strings.Contains(recorder.Body.String(), `"total":550.00`) {
				t.Fatal(recorder.Body.String())
			}
		})
	}
}
func TestInternalQuoteAuthenticationAndParity(t *testing.T) {
	router, err := New(slog.New(slog.NewTextHandler(io.Discard, nil)), Options{Store: fixtureStore{}, InternalToken: "fixture-internal"})
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name, path, token string
		status            int
	}{
		{name: "public", path: "/pricing/estimate", status: 200},
		{name: "missing internal token", path: "/internal/pricing/quote", status: 401},
		{name: "wrong internal token", path: "/internal/pricing/quote", token: "wrong", status: 401},
		{name: "internal", path: "/internal/pricing/quote", token: "fixture-internal", status: 200},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest("POST", test.path, strings.NewReader(validBody))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("X-Internal-Token", test.token)
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)
			if recorder.Code != test.status {
				t.Fatal(recorder.Code, recorder.Body.String())
			}
			if test.status == 200 && !strings.Contains(recorder.Body.String(), `"total":550.00`) {
				t.Fatal("quote mismatch")
			}
		})
	}
}
func TestAdminPricingContracts(t *testing.T) {
	for _, test := range []struct {
		name, method, path, body string
		role                     auth.Role
		status                   int
	}{
		{name: "anonymous", method: "POST", path: "/admin/zones", body: `{"code":"T","name":"Test"}`, status: 401},
		{name: "customer denied", method: "POST", path: "/admin/zones", body: `{"code":"T","name":"Test"}`, role: auth.RoleCustomer, status: 403},
		{name: "create", method: "POST", path: "/admin/zones", body: `{"code":"T","name":"Test"}`, role: auth.RoleAdmin, status: 201},
		{name: "missing name", method: "POST", path: "/admin/zones", body: `{"code":"T"}`, role: auth.RoleAdmin, status: 422},
		{name: "null", method: "POST", path: "/admin/zones", body: `{"code":"T","name":null}`, role: auth.RoleAdmin, status: 422},
		{name: "unknown", method: "POST", path: "/admin/zones", body: `{"code":"T","name":"Test","role":"ADMIN"}`, role: auth.RoleAdmin, status: 422},
		{name: "empty update", method: "PATCH", path: "/admin/zones/" + testOrigin, body: `{}`, role: auth.RoleAdmin, status: 422},
		{name: "invalid identifier", method: "PATCH", path: "/admin/zones/bad", body: `{"name":"Test"}`, role: auth.RoleAdmin, status: 422},
		{name: "list zones anonymous", method: "GET", path: "/admin/zones", status: 401},
		{name: "list zones", method: "GET", path: "/admin/zones", role: auth.RoleAdmin, status: 200},
		{name: "list zones pagination", method: "GET", path: "/admin/zones?pageSize=101", role: auth.RoleAdmin, status: 422},
		{name: "list zones filter", method: "GET", path: "/admin/zones?isActive=maybe", role: auth.RoleAdmin, status: 422},
		{name: "list rules", method: "GET", path: "/admin/pricing/rules", role: auth.RoleAdmin, status: 200},
		{name: "pagination", method: "GET", path: "/admin/pricing/rules?pageSize=101", role: auth.RoleAdmin, status: 422},
		{name: "surcharge", method: "PUT", path: "/admin/pricing/surcharges/SMALL", body: `{"surcharge":0}`, role: auth.RoleAdmin, status: 200},
		{name: "negative fee", method: "POST", path: "/admin/pricing/rules", body: `{"originZoneId":"` + testOrigin + `","destinationZoneId":"` + testDestination + `","baseFee":-1}`, role: auth.RoleAdmin, status: 422},
	} {
		t.Run(test.name, func(t *testing.T) {
			router, err := New(slog.New(slog.NewTextHandler(io.Discard, nil)), Options{Store: fixtureStore{}, AccessSecret: testSecret})
			if err != nil {
				t.Fatal(err)
			}
			request := httptest.NewRequest(test.method, test.path, strings.NewReader(test.body))
			request.Header.Set("Content-Type", "application/json")
			if test.role != "" {
				request.Header.Set("Authorization", "Bearer "+adminToken(t, test.role))
			}
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)
			if recorder.Code != test.status {
				t.Fatalf("status %d, body %s", recorder.Code, recorder.Body.String())
			}
		})
	}
}

// Every admin route must refuse each non-admin role, not only the zones route.
func TestAdminRoutesRefuseEveryNonAdminRole(t *testing.T) {
	routes := []struct{ method, path, body string }{
		{"GET", "/admin/zones", ""},
		{"POST", "/admin/zones", `{"code":"T","name":"Test"}`},
		{"PATCH", "/admin/zones/" + testOrigin, `{"name":"Test"}`},
		{"GET", "/admin/pricing/rules", ""},
		{"POST", "/admin/pricing/rules", `{"originZoneId":"` + testOrigin + `","destinationZoneId":"` + testDestination + `","baseFee":1}`},
		{"PATCH", "/admin/pricing/rules/" + testOrigin, `{"baseFee":1}`},
		{"GET", "/admin/pricing/bands", ""},
		{"POST", "/admin/pricing/bands", `{"minWeightKg":1,"maxWeightKg":2,"charge":1}`},
		{"PATCH", "/admin/pricing/bands/" + testOrigin, `{"charge":1}`},
		{"GET", "/admin/pricing/surcharges", ""},
		{"PUT", "/admin/pricing/surcharges/SMALL", `{"surcharge":1}`},
	}
	router, err := New(slog.New(slog.NewTextHandler(io.Discard, nil)), Options{Store: fixtureStore{}, AccessSecret: testSecret})
	if err != nil {
		t.Fatal(err)
	}
	for _, route := range routes {
		for _, role := range []auth.Role{auth.RoleCustomer, auth.RoleMerchant, auth.RoleCourier, auth.RoleDispatcher} {
			t.Run(route.method+" "+route.path+" "+string(role), func(t *testing.T) {
				request := httptest.NewRequest(route.method, route.path, strings.NewReader(route.body))
				request.Header.Set("Content-Type", "application/json")
				request.Header.Set("Authorization", "Bearer "+adminToken(t, role))
				recorder := httptest.NewRecorder()
				router.ServeHTTP(recorder, request)
				if recorder.Code != http.StatusForbidden {
					t.Fatalf("status %d, body %s", recorder.Code, recorder.Body.String())
				}
			})
		}
	}
}

func TestAdminPricingCookieAuthentication(t *testing.T) {
	router, err := New(slog.New(slog.NewTextHandler(io.Discard, nil)), Options{Store: fixtureStore{}, AccessSecret: testSecret})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest("POST", "/admin/zones", strings.NewReader(`{"code":"T","name":"Test"}`))
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(&http.Cookie{Name: "sd_access", Value: adminToken(t, auth.RoleAdmin)})
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != 201 {
		t.Fatalf("status %d, body %s", recorder.Code, recorder.Body.String())
	}
}

func TestAdminPricingRequiresConfiguredSecret(t *testing.T) {
	router, err := New(slog.New(slog.NewTextHandler(io.Discard, nil)), Options{Store: fixtureStore{}})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest("POST", "/admin/zones", strings.NewReader(`{"code":"T","name":"Test"}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != 503 || !strings.Contains(recorder.Body.String(), "SERVICE_UNAVAILABLE") {
		t.Fatalf("status %d, body %s", recorder.Code, recorder.Body.String())
	}
}

func TestAdminPricingRejectsInvalidTokens(t *testing.T) {
	for _, test := range []struct {
		name  string
		token string
	}{
		{name: "malformed", token: "not-a-token"},
		{name: "wrong issuer", token: signedToken(t, jwt.MapClaims{
			"iss": "other-issuer", "sub": testOrigin, "role": "ADMIN", "jti": "fixture",
			"iat": time.Now().Add(-time.Minute).Unix(), "exp": time.Now().Add(5 * time.Minute).Unix(),
		})},
		{name: "expired", token: signedToken(t, jwt.MapClaims{
			"iss": auth.Issuer, "sub": testOrigin, "role": "ADMIN", "jti": "fixture",
			"iat": time.Now().Add(-2 * time.Hour).Unix(), "exp": time.Now().Add(-time.Hour).Unix(),
		})},
	} {
		t.Run(test.name, func(t *testing.T) {
			router, err := New(slog.New(slog.NewTextHandler(io.Discard, nil)), Options{Store: fixtureStore{}, AccessSecret: testSecret})
			if err != nil {
				t.Fatal(err)
			}
			request := httptest.NewRequest("POST", "/admin/zones", strings.NewReader(`{"code":"T","name":"Test"}`))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Authorization", "Bearer "+test.token)
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)
			if recorder.Code != 401 {
				t.Fatalf("status %d, body %s", recorder.Code, recorder.Body.String())
			}
		})
	}
}

func signedToken(t *testing.T, claims jwt.MapClaims) string {
	t.Helper()
	raw, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(testSecret))
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
func TestPublicZoneProjection(t *testing.T) {
	router, err := New(slog.New(slog.NewTextHandler(io.Discard, nil)), Options{Store: fixtureStore{}})
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest("GET", "/zones", nil))
	if recorder.Code != 200 || strings.Contains(recorder.Body.String(), "isActive") {
		t.Fatal(recorder.Body.String())
	}
}

func TestPricingBodyLimit(t *testing.T) {
	router, err := New(slog.New(slog.NewTextHandler(io.Discard, nil)), Options{Store: fixtureStore{}})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest("POST", "/pricing/estimate", strings.NewReader(`{"originZoneId":"`+strings.Repeat("A", 1<<20)+`"}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != 413 {
		t.Fatalf("status %d body %s", recorder.Code, recorder.Body)
	}
}

func TestInternalZoneProjection(t *testing.T) {
	router, err := New(slog.New(slog.NewTextHandler(io.Discard, nil)), Options{Store: fixtureStore{}, InternalToken: "fixture-token"})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest("GET", "/internal/zones/"+testOrigin, nil)
	request.Header.Set("X-Internal-Token", "fixture-token")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != 200 || !strings.Contains(recorder.Body.String(), `"isActive":true`) || strings.Contains(recorder.Body.String(), "districts") {
		t.Fatal(recorder.Code, recorder.Body.String())
	}
}
