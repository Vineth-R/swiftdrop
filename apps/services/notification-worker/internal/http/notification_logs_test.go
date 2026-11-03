package servicehttp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Vineth-R/swiftdrop/apps/services/notification-worker/internal/idempotency"
	"github.com/Vineth-R/swiftdrop/apps/services/notification-worker/internal/notificationlog"
	"github.com/Vineth-R/swiftdrop/packages/go-common/auth"
)

const notificationLogTestSecret = "unit-test-only-signing-key-over-thirty-two-bytes"

type fakeNotificationLogStore struct {
	logs       []notificationlog.Log
	total      int
	err        error
	calls      int
	lastFilter notificationlog.Filter
}

func (s *fakeNotificationLogStore) List(_ context.Context, filter notificationlog.Filter) ([]notificationlog.Log, int, error) {
	s.calls++
	s.lastFilter = filter
	return s.logs, s.total, s.err
}

type notificationLogListResponse struct {
	Data []map[string]any `json:"data"`
	Meta struct {
		RequestID  string `json:"requestId"`
		Timestamp  string `json:"timestamp"`
		Pagination struct {
			Page       int `json:"page"`
			PageSize   int `json:"pageSize"`
			TotalItems int `json:"totalItems"`
			TotalPages int `json:"totalPages"`
		} `json:"pagination"`
	} `json:"meta"`
}

func newNotificationLogRouter(t *testing.T, store NotificationLogStore, accessSecret string) (*gin.Engine, *bytes.Buffer) {
	t.Helper()
	logs := &bytes.Buffer{}
	logger := slog.New(slog.NewJSONHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	router, err := New(logger, Options{
		Store:        store,
		AccessSecret: accessSecret,
		CookieName:   "sd_access",
	})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	return router, logs
}

func notificationLogToken(t *testing.T, manager *auth.TokenManager, role auth.Role) string {
	t.Helper()
	token, _, err := manager.Issue("3f1c7a52-0000-4000-8000-0000000000a1", role)
	if err != nil {
		t.Fatalf("Issue returned error: %v", err)
	}
	return token
}

func notificationLogManager(t *testing.T) *auth.TokenManager {
	t.Helper()
	manager, err := auth.NewTokenManager(notificationLogTestSecret, time.Minute)
	if err != nil {
		t.Fatalf("NewTokenManager returned error: %v", err)
	}
	return manager
}

func notificationLogRequest(t *testing.T, router *gin.Engine, method, path, token string, cookie bool, requestID string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, path, nil)
	if token != "" {
		if cookie {
			request.AddCookie(&http.Cookie{Name: "sd_access", Value: token})
		} else {
			request.Header.Set("Authorization", "Bearer "+token)
		}
	}
	if requestID != "" {
		request.Header.Set("X-Request-Id", requestID)
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}

func decodeNotificationLogs(t *testing.T, recorder *httptest.ResponseRecorder) notificationLogListResponse {
	t.Helper()
	var response notificationLogListResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v; body %s", err, recorder.Body.String())
	}
	return response
}

func errorCodeFromResponse(t *testing.T, recorder *httptest.ResponseRecorder) string {
	t.Helper()
	var response struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode error response: %v; body %s", err, recorder.Body.String())
	}
	return response.Error.Code
}

func sampleNotificationLog(recipient, channel string) notificationlog.Log {
	subject := "Your parcel has shipped"
	errorMessage := "smtp connection refused"
	sentAt := time.Date(2026, time.November, 2, 10, 0, 1, 0, time.UTC)
	return notificationlog.Log{
		ID:           "11111111-1111-4111-8111-111111111111",
		EventID:      "22222222-2222-4222-8222-222222222222",
		ParcelID:     "33333333-3333-4333-8333-333333333333",
		Channel:      channel,
		Recipient:    recipient,
		Subject:      &subject,
		Status:       idempotency.StatusSent,
		ErrorMessage: &errorMessage,
		CreatedAt:    time.Date(2026, time.November, 2, 10, 0, 0, 0, time.UTC),
		SentAt:       &sentAt,
	}
}

func TestNotificationLogs_authenticationAndAuthorization(t *testing.T) {
	store := &fakeNotificationLogStore{}
	router, _ := newNotificationLogRouter(t, store, notificationLogTestSecret)
	manager := notificationLogManager(t)

	t.Run("missing credentials", func(t *testing.T) {
		store.calls = 0
		response := notificationLogRequest(t, router, http.MethodGet, "/admin/notification-logs", "", false, "")
		if response.Code != http.StatusUnauthorized || errorCodeFromResponse(t, response) != "UNAUTHENTICATED" {
			t.Fatalf("status=%d body=%s", response.Code, response.Body)
		}
		if store.calls != 0 {
			t.Fatal("unauthenticated request reached the store")
		}
	})

	t.Run("admin bearer", func(t *testing.T) {
		response := notificationLogRequest(t, router, http.MethodGet, "/admin/notification-logs", notificationLogToken(t, manager, auth.RoleAdmin), false, "")
		if response.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", response.Code, response.Body)
		}
	})

	t.Run("admin cookie", func(t *testing.T) {
		response := notificationLogRequest(t, router, http.MethodGet, "/admin/notification-logs", notificationLogToken(t, manager, auth.RoleAdmin), true, "")
		if response.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", response.Code, response.Body)
		}
	})

	for _, role := range []auth.Role{auth.RoleCustomer, auth.RoleMerchant, auth.RoleCourier, auth.RoleDispatcher} {
		t.Run(string(role)+" is denied", func(t *testing.T) {
			store.calls = 0
			response := notificationLogRequest(t, router, http.MethodGet, "/admin/notification-logs", notificationLogToken(t, manager, role), false, "")
			if response.Code != http.StatusForbidden || errorCodeFromResponse(t, response) != "FORBIDDEN" {
				t.Fatalf("status=%d body=%s", response.Code, response.Body)
			}
			if store.calls != 0 {
				t.Fatal("non-admin request reached the store")
			}
		})
	}

	t.Run("expired token", func(t *testing.T) {
		expired, err := auth.NewTokenManager(notificationLogTestSecret, time.Minute, auth.WithClock(func() time.Time {
			return time.Now().Add(-time.Hour)
		}))
		if err != nil {
			t.Fatal(err)
		}
		response := notificationLogRequest(t, router, http.MethodGet, "/admin/notification-logs", notificationLogToken(t, expired, auth.RoleAdmin), false, "")
		if response.Code != http.StatusUnauthorized || errorCodeFromResponse(t, response) != "TOKEN_EXPIRED" {
			t.Fatalf("status=%d body=%s", response.Code, response.Body)
		}
	})

	t.Run("invalid signature", func(t *testing.T) {
		other, err := auth.NewTokenManager("another-unit-test-secret-over-thirty-two-bytes", time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		response := notificationLogRequest(t, router, http.MethodGet, "/admin/notification-logs", notificationLogToken(t, other, auth.RoleAdmin), false, "")
		if response.Code != http.StatusUnauthorized || errorCodeFromResponse(t, response) != "UNAUTHENTICATED" {
			t.Fatalf("status=%d body=%s", response.Code, response.Body)
		}
	})

	t.Run("missing secret fails closed", func(t *testing.T) {
		closedStore := &fakeNotificationLogStore{}
		closedRouter, _ := newNotificationLogRouter(t, closedStore, "")
		response := notificationLogRequest(t, closedRouter, http.MethodGet, "/admin/notification-logs", "", false, "")
		if response.Code != http.StatusServiceUnavailable || errorCodeFromResponse(t, response) != "SERVICE_UNAVAILABLE" {
			t.Fatalf("status=%d body=%s", response.Code, response.Body)
		}
		if closedStore.calls != 0 {
			t.Fatal("unconfigured auth reached the store")
		}
	})
}

func TestNotificationLogs_queryAndResponse(t *testing.T) {
	store := &fakeNotificationLogStore{
		logs:  []notificationlog.Log{sampleNotificationLog("john.doe@example.com", "EMAIL")},
		total: 12,
	}
	router, _ := newNotificationLogRouter(t, store, notificationLogTestSecret)
	token := notificationLogToken(t, notificationLogManager(t), auth.RoleAdmin)

	response := notificationLogRequest(t, router, http.MethodGet, "/admin/notification-logs?page=2&pageSize=5&status=SENT", token, false, "req-notification-logs")
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body)
	}
	if store.lastFilter.Status != idempotency.StatusSent || store.lastFilter.Limit != 5 || store.lastFilter.Offset != 5 {
		t.Fatalf("filter = %+v", store.lastFilter)
	}

	body := decodeNotificationLogs(t, response)
	if len(body.Data) != 1 {
		t.Fatalf("data = %+v", body.Data)
	}
	if body.Meta.RequestID != "req-notification-logs" {
		t.Fatalf("requestId = %q", body.Meta.RequestID)
	}
	if body.Meta.Pagination.Page != 2 || body.Meta.Pagination.PageSize != 5 ||
		body.Meta.Pagination.TotalItems != 12 || body.Meta.Pagination.TotalPages != 3 {
		t.Fatalf("pagination = %+v", body.Meta.Pagination)
	}
	if response.Header().Get("X-Request-Id") != "req-notification-logs" {
		t.Fatalf("X-Request-Id header = %q", response.Header().Get("X-Request-Id"))
	}
	if body.Data[0]["recipient"] != "j***@example.com" {
		t.Fatalf("recipient = %v", body.Data[0]["recipient"])
	}
}

func TestNotificationLogs_emptyResultsAndNullableFields(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		store := &fakeNotificationLogStore{logs: []notificationlog.Log{}, total: 0}
		router, _ := newNotificationLogRouter(t, store, notificationLogTestSecret)
		response := notificationLogRequest(t, router, http.MethodGet, "/admin/notification-logs", notificationLogToken(t, notificationLogManager(t), auth.RoleAdmin), false, "")
		body := decodeNotificationLogs(t, response)
		if response.Code != http.StatusOK || body.Data == nil || len(body.Data) != 0 {
			t.Fatalf("status=%d data=%+v", response.Code, body.Data)
		}
		if !strings.Contains(response.Body.String(), `"data":[]`) {
			t.Fatalf("empty data is not an array: %s", response.Body)
		}
	})

	t.Run("nullable fields", func(t *testing.T) {
		entry := sampleNotificationLog("john@example.com", "EMAIL")
		entry.Subject, entry.ErrorMessage, entry.SentAt = nil, nil, nil
		store := &fakeNotificationLogStore{logs: []notificationlog.Log{entry}, total: 1}
		router, _ := newNotificationLogRouter(t, store, notificationLogTestSecret)
		response := notificationLogRequest(t, router, http.MethodGet, "/admin/notification-logs", notificationLogToken(t, notificationLogManager(t), auth.RoleAdmin), false, "")
		body := decodeNotificationLogs(t, response)
		if body.Data[0]["subject"] != nil || body.Data[0]["errorMessage"] != nil || body.Data[0]["sentAt"] != nil {
			t.Fatalf("nullable fields = %+v", body.Data[0])
		}
	})
}

func TestNotificationLogs_validation(t *testing.T) {
	tests := []struct {
		name  string
		query string
		field string
	}{
		{name: "page zero", query: "page=0", field: "page"},
		{name: "page malformed", query: "page=abc", field: "page"},
		{name: "page too large", query: "page=10001", field: "page"},
		{name: "page size zero", query: "pageSize=0", field: "pageSize"},
		{name: "page size malformed", query: "pageSize=1.5", field: "pageSize"},
		{name: "page size too large", query: "pageSize=101", field: "pageSize"},
		{name: "unknown status", query: "status=UNKNOWN", field: "status"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := &fakeNotificationLogStore{}
			router, _ := newNotificationLogRouter(t, store, notificationLogTestSecret)
			response := notificationLogRequest(t, router, http.MethodGet, "/admin/notification-logs?"+test.query, notificationLogToken(t, notificationLogManager(t), auth.RoleAdmin), false, "")
			if response.Code != http.StatusUnprocessableEntity || errorCodeFromResponse(t, response) != "VALIDATION_ERROR" {
				t.Fatalf("status=%d body=%s", response.Code, response.Body)
			}
			if !strings.Contains(response.Body.String(), `"field":"`+test.field+`"`) {
				t.Fatalf("missing field %q: %s", test.field, response.Body)
			}
			if store.calls != 0 {
				t.Fatal("invalid query reached the store")
			}
		})
	}
}

func TestNotificationLogs_maskingAndSensitiveFields(t *testing.T) {
	store := &fakeNotificationLogStore{
		logs: []notificationlog.Log{
			sampleNotificationLog("john.doe@example.com", "EMAIL"),
			sampleNotificationLog("+94771234567", "SMS"),
		},
		total: 2,
	}
	router, logs := newNotificationLogRouter(t, store, notificationLogTestSecret)
	response := notificationLogRequest(t, router, http.MethodGet, "/admin/notification-logs", notificationLogToken(t, notificationLogManager(t), auth.RoleAdmin), false, "")
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body)
	}
	body := decodeNotificationLogs(t, response)
	wantRecipients := []string{"j***@example.com", "+94*******67"}
	for index, want := range wantRecipients {
		if body.Data[index]["recipient"] != want {
			t.Fatalf("recipient[%d] = %v, want %q", index, body.Data[index]["recipient"], want)
		}
	}

	allowed := map[string]bool{
		"id": true, "eventId": true, "parcelId": true, "channel": true, "recipient": true,
		"subject": true, "status": true, "errorMessage": true, "createdAt": true, "sentAt": true,
	}
	for _, item := range body.Data {
		for key := range item {
			if !allowed[key] {
				t.Fatalf("unexpected response field %q", key)
			}
		}
	}

	for _, value := range []string{"john.doe@example.com", "+94771234567", "SELECT", "DSN", "password", "token"} {
		if strings.Contains(response.Body.String(), value) {
			t.Fatalf("response contains %q: %s", value, response.Body)
		}
		if strings.Contains(logs.String(), value) {
			t.Fatalf("captured logs contain %q: %s", value, logs.String())
		}
	}
}

func TestNotificationLogs_errorsAreSanitized(t *testing.T) {
	store := &fakeNotificationLogStore{
		err: fmt.Errorf("%w: pq: SELECT recipient FROM notification.notification_logs host=10.0.0.5", notificationlog.ErrListFailed),
	}
	router, logs := newNotificationLogRouter(t, store, notificationLogTestSecret)
	response := notificationLogRequest(t, router, http.MethodGet, "/admin/notification-logs", notificationLogToken(t, notificationLogManager(t), auth.RoleAdmin), false, "")
	if response.Code != http.StatusInternalServerError || errorCodeFromResponse(t, response) != "INTERNAL_ERROR" {
		t.Fatalf("status=%d body=%s", response.Code, response.Body)
	}
	for _, value := range []string{"SELECT", "10.0.0.5", "notification.notification_logs", "pq:"} {
		if strings.Contains(response.Body.String(), value) {
			t.Fatalf("response leaked %q: %s", value, response.Body)
		}
		if strings.Contains(logs.String(), value) {
			t.Fatalf("logs leaked %q: %s", value, logs.String())
		}
	}

	unavailable := &fakeNotificationLogStore{err: notificationlog.ErrUnavailable}
	unavailableRouter, _ := newNotificationLogRouter(t, unavailable, notificationLogTestSecret)
	response = notificationLogRequest(t, unavailableRouter, http.MethodGet, "/admin/notification-logs", notificationLogToken(t, notificationLogManager(t), auth.RoleAdmin), false, "")
	if response.Code != http.StatusServiceUnavailable || errorCodeFromResponse(t, response) != "SERVICE_UNAVAILABLE" {
		t.Fatalf("status=%d body=%s", response.Code, response.Body)
	}
}

func TestNotificationLogs_routesAndTrailingSlash(t *testing.T) {
	store := &fakeNotificationLogStore{}
	router, _ := newNotificationLogRouter(t, store, notificationLogTestSecret)

	routes := map[string]bool{}
	for _, route := range router.Routes() {
		routes[route.Method+" "+route.Path] = true
	}
	for _, route := range []string{"GET /admin/notification-logs", "GET /admin/notification-logs/"} {
		if !routes[route] {
			t.Fatalf("route %q is not registered", route)
		}
	}

	for _, path := range []string{"/admin/notification-logs", "/admin/notification-logs/"} {
		response := notificationLogRequest(t, router, http.MethodGet, path, "", false, "")
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("%s status=%d", path, response.Code)
		}
	}
}

func TestNotificationLogs_noStore(t *testing.T) {
	store := &fakeNotificationLogStore{}
	router, _ := newNotificationLogRouter(t, store, notificationLogTestSecret)
	manager := notificationLogManager(t)
	admin := notificationLogToken(t, manager, auth.RoleAdmin)
	customer := notificationLogToken(t, manager, auth.RoleCustomer)

	tests := []struct {
		name   string
		path   string
		token  string
		status int
	}{
		{name: "success", path: "/admin/notification-logs", token: admin, status: http.StatusOK},
		{name: "missing credentials", path: "/admin/notification-logs", status: http.StatusUnauthorized},
		{name: "non-admin", path: "/admin/notification-logs", token: customer, status: http.StatusForbidden},
		{name: "validation", path: "/admin/notification-logs?page=0", token: admin, status: http.StatusUnprocessableEntity},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := notificationLogRequest(t, router, http.MethodGet, test.path, test.token, false, "")
			if response.Code != test.status {
				t.Fatalf("status=%d, want %d (%s)", response.Code, test.status, response.Body)
			}
			if got := response.Header().Get("Cache-Control"); got != "no-store" {
				t.Fatalf("Cache-Control = %q, want no-store", got)
			}
		})
	}

	t.Run("query failure", func(t *testing.T) {
		failingStore := &fakeNotificationLogStore{err: notificationlog.ErrListFailed}
		failingRouter, _ := newNotificationLogRouter(t, failingStore, notificationLogTestSecret)
		response := notificationLogRequest(t, failingRouter, http.MethodGet, "/admin/notification-logs", admin, false, "")
		if response.Code != http.StatusInternalServerError {
			t.Fatalf("status=%d, want 500", response.Code)
		}
		if got := response.Header().Get("Cache-Control"); got != "no-store" {
			t.Fatalf("Cache-Control = %q, want no-store", got)
		}
	})

	t.Run("auth not configured", func(t *testing.T) {
		closedRouter, _ := newNotificationLogRouter(t, &fakeNotificationLogStore{}, "")
		response := notificationLogRequest(t, closedRouter, http.MethodGet, "/admin/notification-logs", "", false, "")
		if response.Code != http.StatusServiceUnavailable {
			t.Fatalf("status=%d, want 503", response.Code)
		}
		if got := response.Header().Get("Cache-Control"); got != "no-store" {
			t.Fatalf("Cache-Control = %q, want no-store", got)
		}
	})
}

func TestNotificationLogs_neverLogsSensitiveNotificationData(t *testing.T) {
	const (
		emailCanary   = "privacy-email-canary-1a2b@example.com"
		phoneCanary   = "+94770001122"
		subjectCanary = "SUBJECT-CANARY-6f2a"
		errorCanary   = "ERROR-CANARY-3b9d"
		driverCanary  = "DRIVER-CANARY-8c1e"
	)

	emailEntry := sampleNotificationLog(emailCanary, "EMAIL")
	emailEntry.Subject = stringPointer(subjectCanary)
	emailEntry.ErrorMessage = stringPointer(errorCanary)
	phoneEntry := sampleNotificationLog(phoneCanary, "SMS")
	phoneEntry.Subject = stringPointer(subjectCanary)
	phoneEntry.ErrorMessage = stringPointer(errorCanary)

	store := &fakeNotificationLogStore{logs: []notificationlog.Log{emailEntry, phoneEntry}, total: 2}
	router, captured := newNotificationLogRouter(t, store, notificationLogTestSecret)
	response := notificationLogRequest(t, router, http.MethodGet, "/admin/notification-logs", notificationLogToken(t, notificationLogManager(t), auth.RoleAdmin), false, "")
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body)
	}

	for _, canary := range []string{emailCanary, phoneCanary, subjectCanary, errorCanary} {
		if strings.Contains(captured.String(), canary) {
			t.Fatalf("application logs contain sensitive value %q: %s", canary, captured.String())
		}
	}

	driverStore := &fakeNotificationLogStore{
		err: fmt.Errorf("%w: %s", notificationlog.ErrListFailed, driverCanary),
	}
	driverRouter, driverLogs := newNotificationLogRouter(t, driverStore, notificationLogTestSecret)
	response = notificationLogRequest(t, driverRouter, http.MethodGet, "/admin/notification-logs", notificationLogToken(t, notificationLogManager(t), auth.RoleAdmin), false, "")
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d body=%s", response.Code, response.Body)
	}
	if strings.Contains(driverLogs.String(), driverCanary) {
		t.Fatalf("application logs contain raw driver error %q: %s", driverCanary, driverLogs.String())
	}
	if strings.Contains(response.Body.String(), driverCanary) {
		t.Fatalf("response contains raw driver error %q: %s", driverCanary, response.Body)
	}
}

func stringPointer(value string) *string {
	return &value
}

func TestNotificationLogs_routerWithoutStoreFailsClosed(t *testing.T) {
	router, err := New(slog.New(slog.NewTextHandler(io.Discard, nil)), Options{AccessSecret: notificationLogTestSecret})
	if err != nil {
		t.Fatal(err)
	}
	response := notificationLogRequest(t, router, http.MethodGet, "/admin/notification-logs", notificationLogToken(t, notificationLogManager(t), auth.RoleAdmin), false, "")
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d body=%s", response.Code, response.Body)
	}
}
