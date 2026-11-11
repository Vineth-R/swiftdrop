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

func TestWriteAPIKeyErrorKeepsAuthAndInternalFailuresDistinct(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
		wantLog    bool
	}{
		{
			name:       "unknown key",
			err:        repository.ErrNotFound,
			wantStatus: http.StatusUnauthorized,
			wantCode:   "INVALID_API_KEY",
		},
		{
			name:       "repository outage",
			err:        errors.New("database unavailable"),
			wantStatus: http.StatusInternalServerError,
			wantCode:   "INTERNAL_ERROR",
			wantLog:    true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var logs bytes.Buffer
			logger := slog.New(slog.NewJSONHandler(&logs, nil))

			gin.SetMode(gin.TestMode)
			router := gin.New()
			router.Use(requestid.Middleware())
			router.GET("/", func(c *gin.Context) {
				writeAPIKeyError(c, logger, "lookup_api_key", test.err)
			})

			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))

			if recorder.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d", recorder.Code, test.wantStatus)
			}

			var body httpx.ErrorResponse
			if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
				t.Fatalf("invalid error JSON: %v", err)
			}
			if body.Error.Code != test.wantCode {
				t.Fatalf("error code = %q, want %q", body.Error.Code, test.wantCode)
			}
			if body.Meta.RequestID == "" {
				t.Fatal("error response did not include meta.requestId")
			}

			if test.wantLog {
				for _, want := range []string{"merchant api key request failed", "lookup_api_key", "error_class", "internal"} {
					if !strings.Contains(logs.String(), want) {
						t.Fatalf("internal failure log missing %q: %s", want, logs.String())
					}
				}
				if strings.Contains(logs.String(), "database unavailable") {
					t.Fatalf("internal failure log leaked the raw error text: %s", logs.String())
				}
				if !strings.Contains(logs.String(), `"level":"ERROR"`) {
					t.Fatalf("internal failure was not logged at error level: %s", logs.String())
				}
			} else if strings.Contains(logs.String(), "merchant api key request failed") {
				t.Fatalf("invalid credentials were logged as an internal failure: %s", logs.String())
			}
		})
	}
}
