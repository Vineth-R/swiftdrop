package servicehttp

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

const merchantContactTestToken = "internal-token-for-tests"

func merchantContactRouter() http.Handler {
	return NewWithOptions(Options{
		InternalToken: merchantContactTestToken,
		Logger:        slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
}

func merchantContactRequest(t *testing.T, router http.Handler, merchantID, token, requestID string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, "/internal/merchants/"+merchantID+"/contact", nil)
	if token != "" {
		request.Header.Set("X-Internal-Token", token)
	}
	if requestID != "" {
		request.Header.Set("X-Request-Id", requestID)
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}

func TestInternalMerchantContactFailsClosed(t *testing.T) {
	const merchantID = "4f1c7a52-0000-4000-8000-000000000002"
	router := merchantContactRouter()

	t.Run("missing token", func(t *testing.T) {
		recorder := merchantContactRequest(t, router, merchantID, "", "req-1")
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body)
		}
	})

	t.Run("wrong token", func(t *testing.T) {
		recorder := merchantContactRequest(t, router, merchantID, "wrong-token", "req-1")
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body)
		}
	})

	t.Run("missing repository", func(t *testing.T) {
		recorder := merchantContactRequest(t, router, merchantID, merchantContactTestToken, "req-1")
		if recorder.Code != http.StatusServiceUnavailable {
			t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body)
		}
		var body map[string]any
		if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
	})

	t.Run("missing request id", func(t *testing.T) {
		recorder := merchantContactRequest(t, router, merchantID, merchantContactTestToken, "")
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body)
		}
	})

	t.Run("malformed request id", func(t *testing.T) {
		recorder := merchantContactRequest(t, router, merchantID, merchantContactTestToken, "bad request id")
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body)
		}
	})
}
