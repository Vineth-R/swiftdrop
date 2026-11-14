package servicehttp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Vineth-R/swiftdrop/apps/services/parcel/internal/courier"
	"github.com/Vineth-R/swiftdrop/apps/services/parcel/internal/statemachine"
	"github.com/Vineth-R/swiftdrop/packages/go-common/auth"
	"github.com/Vineth-R/swiftdrop/packages/go-common/httpx"
)

type courierStatusFixture struct {
	courierFixture
	input      courier.StatusInput
	writeErr   error
	writeCalls int
	ctx        context.Context
}

func (f *courierStatusFixture) UpdateStatus(ctx context.Context, input courier.StatusInput) (courier.StatusResult, error) {
	f.input, f.ctx = input, ctx
	f.writeCalls++
	return courier.StatusResult{ParcelID: input.ParcelID, TrackingNumber: "SDK7M2Q9XP4",
		PreviousStatus: statemachine.StatusOutForDelivery, Status: input.Status,
		UpdatedAt: time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)}, f.writeErr
}

func courierStatusRequest(t *testing.T, router *gin.Engine, parcelID, body, userID string, role auth.Role, cookie bool) *httptest.ResponseRecorder {
	t.Helper()
	return courierStatusRequestPath(t, router, "/courier/deliveries/"+parcelID+"/status", body, userID, role, cookie)
}

func courierStatusRequestPath(t *testing.T, router *gin.Engine, path, body, userID string, role auth.Role, cookie bool) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if role != "" {
		manager, err := auth.NewTokenManager(reportTestSecret, time.Hour)
		if err != nil {
			t.Fatal(err)
		}
		token, _, err := manager.Issue(userID, role)
		if err != nil {
			t.Fatal(err)
		}
		if cookie {
			req.AddCookie(&http.Cookie{Name: "sd_access", Value: token})
		} else {
			req.Header.Set("Authorization", "Bearer "+token)
		}
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestCourierStatusAuthorization(t *testing.T) {
	for _, tc := range []struct {
		name string
		role auth.Role
		want int
	}{
		{name: "signed out", want: 401},
		{name: "customer", role: auth.RoleCustomer, want: 403},
		{name: "merchant", role: auth.RoleMerchant, want: 403},
		{name: "dispatcher", role: auth.RoleDispatcher, want: 403},
		{name: "admin", role: auth.RoleAdmin, want: 403},
		{name: "courier", role: auth.RoleCourier, want: 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &courierStatusFixture{}
			router := reportRouter(t, Options{AccessSecret: reportTestSecret, SessionCheck: allowSessions(), Courier: store, CourierStatus: store})
			rec := courierStatusRequest(t, router, courierParcelID, `{"status":"PICKED_UP"}`, courierTestID, tc.role, false)
			if rec.Code != tc.want || rec.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("response %d %s", rec.Code, rec.Body)
			}
			if tc.want != 200 && store.writeCalls != 0 {
				t.Fatal("unauthorized caller reached write store")
			}
		})
	}
	for _, tc := range []struct {
		name string
		opts Options
		id   string
		want int
	}{
		{name: "no JWT secret", id: courierTestID, want: 503},
		{name: "missing database", opts: Options{AccessSecret: reportTestSecret, SessionCheck: allowSessions()}, id: courierTestID, want: 503},
		{name: "read-only implementation", opts: Options{AccessSecret: reportTestSecret, SessionCheck: allowSessions(), Courier: &courierFixture{}}, id: courierTestID, want: 503},
		{name: "invalid JWT subject", opts: Options{AccessSecret: reportTestSecret, SessionCheck: allowSessions()}, id: "not-a-uuid", want: 401},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := courierStatusRequest(t, reportRouter(t, tc.opts), courierParcelID,
				`{"status":"PICKED_UP"}`, tc.id, auth.RoleCourier, true)
			if rec.Code != tc.want {
				t.Fatalf("response %d %s", rec.Code, rec.Body)
			}
		})
	}
}

func TestCourierStatusRequestValidation(t *testing.T) {
	for _, tc := range []struct{ name, id, body string }{
		{name: "invalid UUID", id: "invalid", body: `{"status":"PICKED_UP"}`},
		{name: "empty", id: courierParcelID},
		{name: "null", id: courierParcelID, body: `null`},
		{name: "array", id: courierParcelID, body: `[]`},
		{name: "string", id: courierParcelID, body: `"PICKED_UP"`},
		{name: "numeric status", id: courierParcelID, body: `{"status":1}`},
		{name: "numeric recipient", id: courierParcelID, body: `{"recipientName":1}`},
		{name: "numeric reason", id: courierParcelID, body: `{"failureReason":1}`},
		{name: "numeric note", id: courierParcelID, body: `{"note":1}`},
		{name: "malformed", id: courierParcelID, body: `{"status":`},
		{name: "multiple objects", id: courierParcelID, body: `{"status":"PICKED_UP"} {}`},
		{name: "trailing garbage", id: courierParcelID, body: `{"status":"PICKED_UP"} invalid`},
		{name: "forged courier", id: courierParcelID, body: `{"status":"PICKED_UP","courierId":"another"}`},
		{name: "over body limit", id: courierParcelID, body: `{"status":"PICKED_UP","note":"` + strings.Repeat("x", 17000) + `"}`},
		{name: "over limit trailing whitespace", id: courierParcelID, body: `{"status":"PICKED_UP"}` + strings.Repeat(" ", 17000)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &courierStatusFixture{}
			router := reportRouter(t, Options{AccessSecret: reportTestSecret, SessionCheck: allowSessions(), Courier: store, CourierStatus: store})
			rec := courierStatusRequest(t, router, tc.id, tc.body, courierTestID, auth.RoleCourier, true)
			if rec.Code != 400 || store.writeCalls != 0 || !strings.Contains(rec.Body.String(), `"code":"BAD_REQUEST"`) {
				t.Fatalf("response %d %s; calls=%d", rec.Code, rec.Body, store.writeCalls)
			}
		})
	}
}

func TestCourierStatusResultAndClaims(t *testing.T) {
	for _, cookie := range []bool{false, true} {
		t.Run(fmt.Sprintf("cookie=%t", cookie), func(t *testing.T) {
			store := &courierStatusFixture{}
			router := reportRouter(t, Options{AccessSecret: reportTestSecret, SessionCheck: allowSessions(), Courier: store, CourierStatus: store})
			rec := courierStatusRequest(t, router, courierParcelID,
				`{"status":"DELIVERED","recipientName":"Test Recipient","note":"At door"}`, strings.ToUpper(courierTestID), auth.RoleCourier, cookie)
			if rec.Code != 200 || store.writeCalls != 1 {
				t.Fatalf("response %d %s", rec.Code, rec.Body)
			}
			if store.input.CourierID != courierTestID || store.input.ParcelID != courierParcelID || store.input.RecipientName != "Test Recipient" || store.input.Note != "At door" || store.ctx == nil {
				t.Fatalf("input %+v", store.input)
			}
			var body struct {
				Data courier.StatusResult `json:"data"`
				Meta httpx.SuccessMeta    `json:"meta"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body.Data.Status != statemachine.StatusDelivered || body.Data.PreviousStatus != statemachine.StatusOutForDelivery || body.Data.ParcelID != courierParcelID || body.Data.UpdatedAt.IsZero() || body.Meta.RequestID == "" {
				t.Fatalf("envelope %s", rec.Body)
			}
		})
	}
}

func TestCourierStatusTrailingSlash(t *testing.T) {
	for _, path := range []string{
		"/courier/deliveries/" + courierParcelID + "/status",
		"/courier/deliveries/" + courierParcelID + "/status/",
	} {
		t.Run(path, func(t *testing.T) {
			store := &courierStatusFixture{}
			router := reportRouter(t, Options{AccessSecret: reportTestSecret, SessionCheck: allowSessions(), Courier: store, CourierStatus: store})
			rec := courierStatusRequestPath(t, router, path, `{"status":"PICKED_UP"}`, courierTestID, auth.RoleCourier, true)
			if rec.Code != http.StatusOK {
				t.Fatalf("response %d %s", rec.Code, rec.Body)
			}
			if location := rec.Header().Get("Location"); location != "" {
				t.Fatalf("unexpected redirect to %q", location)
			}
			if rec.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("no-store header missing")
			}
			if store.writeCalls != 1 {
				t.Fatalf("handler calls = %d, want 1", store.writeCalls)
			}
		})
	}
	t.Run("trailing slash still requires the courier role", func(t *testing.T) {
		store := &courierStatusFixture{}
		router := reportRouter(t, Options{AccessSecret: reportTestSecret, SessionCheck: allowSessions(), Courier: store, CourierStatus: store})
		rec := courierStatusRequestPath(t, router, "/courier/deliveries/"+courierParcelID+"/status/",
			`{"status":"PICKED_UP"}`, courierTestID, auth.RoleCustomer, true)
		if rec.Code != http.StatusForbidden || store.writeCalls != 0 {
			t.Fatalf("response %d %s; calls=%d", rec.Code, rec.Body, store.writeCalls)
		}
	})
}

func TestCourierStatusErrors(t *testing.T) {
	for _, tc := range []struct {
		name, code string
		err        error
		want       int
	}{
		{name: "missing", err: courier.ErrNotFound, want: 404, code: "PARCEL_NOT_FOUND"},
		{name: "foreign", err: statemachine.ErrForbidden, want: 403, code: "FORBIDDEN"},
		{name: "transition", err: statemachine.ErrInvalidTransition, want: 409, code: "INVALID_TRANSITION"},
		{name: "recipient", err: statemachine.ErrRecipientRequired, want: 422, code: "RECIPIENT_REQUIRED"},
		{name: "failure reason", err: statemachine.ErrFailureReasonRequired, want: 422, code: "FAILURE_REASON_REQUIRED"},
		{name: "validation", err: statemachine.ErrValidationError, want: 422, code: "VALIDATION_ERROR"},
		{name: "unavailable", err: courier.ErrUnavailable, want: 503, code: "SERVICE_UNAVAILABLE"},
		{name: "inconsistent assignment", err: courier.ErrInconsistentAssignment, want: 500, code: "INTERNAL_ERROR"},
		{name: "database", err: errors.New("SQL with secret recipient information"), want: 500, code: "INTERNAL_ERROR"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &courierStatusFixture{writeErr: fmt.Errorf("wrapped: %w", tc.err)}
			router := reportRouter(t, Options{AccessSecret: reportTestSecret, SessionCheck: allowSessions(), Courier: store, CourierStatus: store})
			rec := courierStatusRequest(t, router, courierParcelID, `{"status":"PICKED_UP"}`, courierTestID, auth.RoleCourier, false)
			if rec.Code != tc.want || !strings.Contains(rec.Body.String(), `"code":"`+tc.code+`"`) {
				t.Fatalf("response %d %s", rec.Code, rec.Body)
			}
			if strings.Contains(rec.Body.String(), "secret") || strings.Contains(rec.Body.String(), "SQL") {
				t.Fatal("database diagnostics leaked")
			}
		})
	}
}
