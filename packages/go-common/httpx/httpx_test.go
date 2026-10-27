package httpx

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Vineth-R/swiftdrop/packages/go-common/requestid"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func bufferLogger() (*slog.Logger, *bytes.Buffer) {
	buffer := &bytes.Buffer{}
	return slog.New(slog.NewJSONHandler(buffer, nil)), buffer
}

func debugBufferLogger() (*slog.Logger, *bytes.Buffer) {
	buffer := &bytes.Buffer{}
	return slog.New(slog.NewJSONHandler(buffer, &slog.HandlerOptions{Level: slog.LevelDebug})), buffer
}

func logRecords(t *testing.T, buffer *bytes.Buffer) []map[string]any {
	t.Helper()

	var records []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(buffer.String()), "\n") {
		if line == "" {
			continue
		}

		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("invalid log record %q: %v", line, err)
		}
		records = append(records, record)
	}
	return records
}

func findLogRecord(records []map[string]any, message string) map[string]any {
	for _, record := range records {
		if record["msg"] == message {
			return record
		}
	}
	return nil
}

func TestRouterHealthEndpoints(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := NewRouter(discardLogger())
	RegisterHealth(router)

	for _, path := range []string{"/healthz", "/readyz"} {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))

		if recorder.Code != http.StatusOK {
			t.Fatalf("%s status = %d, want %d", path, recorder.Code, http.StatusOK)
		}
		if got := recorder.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
			t.Fatalf("%s content type = %q", path, got)
		}
		if got := recorder.Header().Get(requestid.Header); got == "" {
			t.Fatalf("%s did not include %s", path, requestid.Header)
		}

		var body HealthResponse
		if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
			t.Fatalf("%s returned invalid JSON: %v", path, err)
		}
		if body.Status == "" {
			t.Fatalf("%s returned an empty status", path)
		}
	}
}

func TestRouterNotFoundIsJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := NewRouter(discardLogger())

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/missing", nil))

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNotFound)
	}

	var body ErrorResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if body.Error.Code != "not_found" {
		t.Fatalf("error code = %q, want not_found", body.Error.Code)
	}
}

func TestRouterMethodNotAllowedIsJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := NewRouter(discardLogger())
	RegisterHealth(router)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/healthz", nil))

	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusMethodNotAllowed)
	}
	if got := recorder.Header().Get("Allow"); got != http.MethodGet {
		t.Fatalf("Allow = %q, want GET", got)
	}
	if got := recorder.Header().Get(requestid.Header); got == "" {
		t.Fatalf("response did not include %s", requestid.Header)
	}

	var body ErrorResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if body.Error.Code != "method_not_allowed" {
		t.Fatalf("error code = %q, want method_not_allowed", body.Error.Code)
	}
	if body.Error.Message != "method not allowed" {
		t.Fatalf("error message = %q", body.Error.Message)
	}
}

func TestErrorResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/", func(c *gin.Context) {
		Error(c, http.StatusBadRequest, "invalid_request", "request is invalid")
	})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))

	var body ErrorResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if body.Error.Message != "request is invalid" {
		t.Fatalf("message = %q", body.Error.Message)
	}
}

func TestRequestLoggerSuppressesProbeLogsAtInfo(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logger, buffer := bufferLogger()
	router := NewRouter(logger)
	RegisterHealth(router)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	if buffer.Len() != 0 {
		t.Fatalf("probe request produced info logs: %s", buffer.String())
	}

	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/missing", nil))

	record := findLogRecord(logRecords(t, buffer), "http request")
	if record == nil {
		t.Fatal("business request was not logged at info")
	}
	if record["status"] != float64(http.StatusNotFound) {
		t.Fatalf("status = %v, want %d", record["status"], http.StatusNotFound)
	}
	if record["request_id"] == "" {
		t.Fatal("request log did not include a request_id")
	}
}

func TestRequestLoggerLogsProbesAtDebug(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logger, buffer := debugBufferLogger()
	router := NewRouter(logger)
	RegisterHealth(router)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	record := findLogRecord(logRecords(t, buffer), "http request")
	if record == nil {
		t.Fatal("probe request was not logged at debug")
	}
	if record["level"] != "DEBUG" {
		t.Fatalf("level = %v, want DEBUG", record["level"])
	}
}

func TestRecoveryWritesJSONEnvelopeAndLogsContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logger, buffer := bufferLogger()
	router := NewRouter(logger)
	router.POST("/panic", func(*gin.Context) {
		panic("boom")
	})

	request := httptest.NewRequest(http.MethodPost, "/panic", strings.NewReader("password=super-secret-body"))
	request.Header.Set(requestid.Header, "req-123")
	request.Header.Set("Authorization", "Bearer super-secret-token")
	request.Header.Set("Cookie", "session=super-secret-cookie")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusInternalServerError)
	}
	if got := recorder.Header().Get(requestid.Header); got != "req-123" {
		t.Fatalf("request ID header = %q, want req-123", got)
	}

	var body ErrorResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if body.Error.Code != "internal_error" {
		t.Fatalf("error code = %q, want internal_error", body.Error.Code)
	}
	if body.Error.Message != "internal server error" {
		t.Fatalf("error message = %q", body.Error.Message)
	}

	records := logRecords(t, buffer)
	panicRecord := findLogRecord(records, "panic recovered")
	if panicRecord == nil {
		t.Fatal("panic was not logged with structured logging")
	}
	if panicRecord["request_id"] != "req-123" {
		t.Fatalf("panic log request_id = %v, want req-123", panicRecord["request_id"])
	}
	if panicRecord["method"] != http.MethodPost {
		t.Fatalf("panic log method = %v, want POST", panicRecord["method"])
	}
	if panicRecord["path"] != "/panic" {
		t.Fatalf("panic log path = %v, want /panic", panicRecord["path"])
	}
	if stack, _ := panicRecord["stack"].(string); !strings.Contains(stack, "httpx_test.go") {
		t.Fatal("panic log did not include a useful stack trace")
	}

	requestRecord := findLogRecord(records, "http request")
	if requestRecord == nil {
		t.Fatal("request was not logged after recovery")
	}
	if requestRecord["status"] != float64(http.StatusInternalServerError) {
		t.Fatalf("request log status = %v, want 500", requestRecord["status"])
	}
	if requestRecord["request_id"] != "req-123" {
		t.Fatalf("request log request_id = %v, want req-123", requestRecord["request_id"])
	}

	logs := buffer.String()
	for _, secret := range []string{"super-secret-token", "super-secret-cookie", "super-secret-body"} {
		if strings.Contains(logs, secret) {
			t.Fatalf("logs leaked sensitive request data %q", secret)
		}
	}
}

func TestRecoveryDoesNotDoubleWrite(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logger, buffer := bufferLogger()
	router := NewRouter(logger)
	router.GET("/partial", func(c *gin.Context) {
		c.String(http.StatusOK, "partial response")
		panic("boom")
	})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/partial", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if got := recorder.Body.String(); got != "partial response" {
		t.Fatalf("body = %q, want partial response", got)
	}
	if strings.Contains(recorder.Body.String(), "internal server error") {
		t.Fatal("recovery wrote an error envelope after the response was already written")
	}
	if findLogRecord(logRecords(t, buffer), "panic recovered") == nil {
		t.Fatal("panic was not logged")
	}
}

func TestRecoveryHandlesBrokenPipe(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logger, buffer := bufferLogger()
	router := NewRouter(logger)
	router.GET("/broken", func(*gin.Context) {
		panic(syscall.EPIPE)
	})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/broken", nil))

	if recorder.Body.Len() != 0 {
		t.Fatalf("broken pipe wrote a response body: %q", recorder.Body.String())
	}

	records := logRecords(t, buffer)
	brokenRecord := findLogRecord(records, "http connection broken")
	if brokenRecord == nil {
		t.Fatal("broken pipe was not logged")
	}
	if brokenRecord["level"] != "WARN" {
		t.Fatalf("broken pipe level = %v, want WARN", brokenRecord["level"])
	}
	if findLogRecord(records, "panic recovered") != nil {
		t.Fatal("broken pipe was logged as a panic")
	}
}

func TestNewServerHardening(t *testing.T) {
	logger, buffer := bufferLogger()
	handler := http.NotFoundHandler()

	server := NewServer(ServerConfig{
		Addr:              ":8080",
		Handler:           handler,
		Logger:            logger,
		ReadHeaderTimeout: time.Second,
		ReadTimeout:       2 * time.Second,
		WriteTimeout:      3 * time.Second,
		IdleTimeout:       4 * time.Second,
	})

	if server.MaxHeaderBytes != DefaultMaxHeaderBytes {
		t.Fatalf("MaxHeaderBytes = %d, want %d", server.MaxHeaderBytes, DefaultMaxHeaderBytes)
	}
	if server.ReadHeaderTimeout != time.Second || server.ReadTimeout != 2*time.Second ||
		server.WriteTimeout != 3*time.Second || server.IdleTimeout != 4*time.Second {
		t.Fatal("server timeouts were not preserved")
	}
	if server.ErrorLog == nil {
		t.Fatal("ErrorLog was not wired to slog")
	}

	server.ErrorLog.Print("server error")
	if !strings.Contains(buffer.String(), "server error") {
		t.Fatal("ErrorLog did not write through the slog handler")
	}

	custom := NewServer(ServerConfig{MaxHeaderBytes: 2 << 20})
	if custom.MaxHeaderBytes != 2<<20 {
		t.Fatalf("MaxHeaderBytes = %d, want %d", custom.MaxHeaderBytes, 2<<20)
	}
}
