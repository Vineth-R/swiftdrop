package metrics

import (
	"bufio"
	"io"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

var (
	uuidPattern     = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	hexPattern      = regexp.MustCompile(`^[0-9a-fA-F]{24,}$`)
	trackingPattern = regexp.MustCompile(`^SD[KN][A-Z0-9]{8}$`)
)

// NewHTTPHandler wraps next and records request count, status and duration.
// The route label is the path with identifier-like segments normalised, which
// keeps cardinality bounded without depending on the router.
func NewHTTPHandler(registry *Registry, next http.Handler) http.Handler {
	if registry == nil {
		return next
	}
	requests := registry.Counter("swiftdrop_http_requests_total", "HTTP requests by method, route and status.", "method", "route", "status")
	errors := registry.Counter("swiftdrop_http_errors_total", "HTTP responses with status 400 or above.", "method", "route", "status")
	duration := registry.Histogram("swiftdrop_http_request_duration_seconds", "HTTP request duration in seconds.", "method", "route")

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if skipMetricsPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		start := time.Now()
		recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(recorder, r)

		route := NormalizePath(r.URL.Path)
		status := strconv.Itoa(recorder.status)
		requests.With(r.Method, route, status).Inc()
		if recorder.status >= http.StatusBadRequest {
			errors.With(r.Method, route, status).Inc()
		}
		duration.With(r.Method, route).Observe(time.Since(start).Seconds())
	})
}

// NormalizePath replaces identifier-like path segments with ":id".
func NormalizePath(path string) string {
	if path == "" {
		return "/"
	}
	segments := strings.Split(path, "/")
	for i, segment := range segments {
		switch {
		case uuidPattern.MatchString(segment), hexPattern.MatchString(segment):
			segments[i] = ":id"
		case trackingPattern.MatchString(segment):
			segments[i] = ":trackingNumber"
		}
	}
	return strings.Join(segments, "/")
}

// Handler serves the registry as Prometheus text. It is registered on the
// service port only; the gateway never routes /metrics.
func Handler(registry *Registry) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		c.Header("Cache-Control", "no-store")
		c.Status(http.StatusOK)
		if registry != nil {
			_, _ = registry.WriteTo(c.Writer)
		}
	}
}

func skipMetricsPath(path string) bool {
	switch path {
	case "/metrics", "/health", "/ready", "/healthz", "/readyz", "/nginx-health":
		return true
	default:
		return false
	}
}

// statusRecorder captures the status code and preserves the optional
// interfaces the WebSocket upgrade and streaming handlers rely on.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (r *statusRecorder) Write(data []byte) (int, error) {
	return r.ResponseWriter.Write(data)
}

func (r *statusRecorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }

func (r *statusRecorder) Flush() {
	if flusher, ok := r.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (r *statusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := r.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, http.ErrNotSupported
	}
	return hijacker.Hijack()
}

func (r *statusRecorder) ReadFrom(source io.Reader) (int64, error) {
	if readerFrom, ok := r.ResponseWriter.(io.ReaderFrom); ok {
		return readerFrom.ReadFrom(source)
	}
	return io.Copy(struct{ io.Writer }{r}, source)
}
