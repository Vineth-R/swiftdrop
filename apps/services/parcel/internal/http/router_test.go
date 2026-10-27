package servicehttp

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/Vineth-R/swiftdrop/packages/go-common/requestid"
)

func TestHealthEndpoints(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := New(slog.New(slog.NewTextHandler(io.Discard, nil)))

	for _, path := range []string{"/healthz", "/readyz"} {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))

		if recorder.Code != http.StatusOK {
			t.Fatalf("%s status = %d, want %d", path, recorder.Code, http.StatusOK)
		}
		if recorder.Header().Get(requestid.Header) == "" {
			t.Fatalf("%s did not include a request ID", path)
		}
	}
}
