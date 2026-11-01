package httpx

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Vineth-R/swiftdrop/packages/go-common/requestid"
)

func NewRouter(logger *slog.Logger) *gin.Engine {
	router := gin.New()
	router.HandleMethodNotAllowed = true
	router.Use(requestid.Middleware())
	router.Use(RequestLogger(logger))
	router.Use(Recovery(logger))
	router.NoRoute(func(c *gin.Context) {
		Error(c, http.StatusNotFound, "NOT_FOUND", "route not found")
	})
	router.NoMethod(func(c *gin.Context) {
		Error(c, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
	})
	return router
}

func RegisterHealth(router *gin.Engine) {
	router.GET("/healthz", Healthz)
	router.GET("/readyz", Readyz)
}

func RequestLogger(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		attributes := []any{
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status", c.Writer.Status(),
			"latency_ms", time.Since(start).Milliseconds(),
			"request_id", requestid.FromGin(c),
		}

		if isProbePath(c.Request.URL.Path) && c.Writer.Status() < http.StatusBadRequest {
			logger.DebugContext(c.Request.Context(), "http request", attributes...)
			return
		}
		logger.InfoContext(c.Request.Context(), "http request", attributes...)
	}
}

func Recovery(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			recovered := recover()
			if recovered == nil {
				return
			}

			recoveredErr, isError := recovered.(error)
			if isError && errors.Is(recoveredErr, http.ErrAbortHandler) {
				c.Abort()
				return
			}

			requestID := requestid.FromGin(c)
			if isError && (errors.Is(recoveredErr, syscall.EPIPE) || errors.Is(recoveredErr, syscall.ECONNRESET)) {
				logger.WarnContext(c.Request.Context(), "http connection broken",
					"request_id", requestID,
					"method", c.Request.Method,
					"path", c.Request.URL.Path,
				)
				_ = c.Error(recoveredErr)
				c.Abort()
				return
			}

			logger.ErrorContext(c.Request.Context(), "panic recovered",
				"request_id", requestID,
				"method", c.Request.Method,
				"path", c.Request.URL.Path,
				"panic", fmt.Sprint(recovered),
				"stack", string(debug.Stack()),
			)

			if c.Writer.Written() {
				c.Abort()
				return
			}

			Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		}()

		c.Next()
	}
}

func isProbePath(path string) bool {
	switch strings.TrimSuffix(path, "/") {
	case "/health", "/ready", "/healthz", "/readyz":
		return true
	default:
		return false
	}
}
