package servicehttp

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/Vineth-R/swiftdrop/apps/services/merchant/internal/repository"
	"github.com/Vineth-R/swiftdrop/packages/go-common/httpx"
	"github.com/Vineth-R/swiftdrop/packages/go-common/requestid"
)

func newErrorTestRouter(handler gin.HandlerFunc) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(requestid.Middleware())
	router.GET("/", handler)
	return router
}

func decodeErrorResponse(t *testing.T, recorder *httptest.ResponseRecorder) httpx.ErrorResponse {
	t.Helper()

	var body httpx.ErrorResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid error JSON: %v (%s)", err, recorder.Body.String())
	}
	if body.Meta.RequestID == "" {
		t.Fatal("error response did not include meta.requestId")
	}
	return body
}

func TestRepositoryErrorsUseAPIv11Codes(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{name: "not found", err: repository.ErrNotFound, wantStatus: http.StatusNotFound, wantCode: "NOT_FOUND"},
		{name: "invalid state", err: repository.ErrInvalidState, wantStatus: http.StatusConflict, wantCode: "INVALID_MERCHANT_STATE"},
		{name: "unexpected", err: errors.New("database down"), wantStatus: http.StatusInternalServerError, wantCode: "INTERNAL_ERROR"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var logs bytes.Buffer
			handler := &merchantHandler{logger: slog.New(slog.NewJSONHandler(&logs, nil))}
			router := newErrorTestRouter(func(c *gin.Context) {
				handler.handleRepositoryError(c, test.err)
			})

			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))

			if recorder.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d", recorder.Code, test.wantStatus)
			}
			body := decodeErrorResponse(t, recorder)
			if body.Error.Code != test.wantCode {
				t.Fatalf("error code = %q, want %q", body.Error.Code, test.wantCode)
			}

			if test.wantStatus == http.StatusInternalServerError {
				if !strings.Contains(logs.String(), "merchant request failed") || !strings.Contains(logs.String(), "merchant_repository") {
					t.Fatalf("unexpected repository error was not logged: %s", logs.String())
				}
			} else if strings.Contains(logs.String(), "merchant request failed") {
				t.Fatalf("expected error was logged as unexpected: %s", logs.String())
			}
		})
	}
}

func TestMerchantIdentityRejectsMissingIdentityWithUpperCode(t *testing.T) {
	router := newErrorTestRouter(func(c *gin.Context) {
		if _, ok := merchantIdentity(c); !ok {
			return
		}
		t.Fatal("merchantIdentity accepted a missing identity")
	})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusUnauthorized)
	}
	body := decodeErrorResponse(t, recorder)
	if body.Error.Code != "UNAUTHORIZED" {
		t.Fatalf("error code = %q, want UNAUTHORIZED", body.Error.Code)
	}
}

func TestRequireAdminIdentityUsesForbiddenCode(t *testing.T) {
	router := newErrorTestRouter(func(c *gin.Context) {
		WithIdentity(c, Identity{UserID: "user-1", Role: "CUSTOMER"})
		if _, ok := requireAdminIdentity(c); !ok {
			return
		}
		t.Fatal("requireAdminIdentity accepted a non-admin")
	})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusForbidden)
	}
	body := decodeErrorResponse(t, recorder)
	if body.Error.Code != "FORBIDDEN" {
		t.Fatalf("error code = %q, want FORBIDDEN", body.Error.Code)
	}
}
