package httpx

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Vineth-R/swiftdrop/packages/go-common/requestid"
)

func NewRouter(logger *slog.Logger) *gin.Engine {
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(requestid.Middleware())
	router.Use(RequestLogger(logger))
	router.NoRoute(func(c *gin.Context) {
		Error(c, http.StatusNotFound, "not_found", "route not found")
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

		logger.InfoContext(c.Request.Context(), "http request",
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status", c.Writer.Status(),
			"latency_ms", time.Since(start).Milliseconds(),
			"request_id", requestid.FromGin(c),
		)
	}
}
