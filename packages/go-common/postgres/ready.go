package postgres

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

type readinessResponse struct {
	Status string            `json:"status"`
	Checks map[string]string `json:"checks,omitempty"`
}

func HealthHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	}
}

func ReadyHandler(pinger Pinger, timeout time.Duration) gin.HandlerFunc {
	if timeout <= 0 {
		timeout = DefaultReadyTimeout
	}

	return func(c *gin.Context) {
		if pinger == nil {
			c.JSON(http.StatusServiceUnavailable, readinessResponse{
				Status: "unavailable",
				Checks: map[string]string{"database": "down"},
			})
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), timeout)
		defer cancel()

		if err := pinger.Ping(ctx); err != nil {
			c.JSON(http.StatusServiceUnavailable, readinessResponse{
				Status: "unavailable",
				Checks: map[string]string{"database": "down"},
			})
			return
		}

		c.JSON(http.StatusOK, readinessResponse{
			Status: "ready",
			Checks: map[string]string{"database": "ok"},
		})
	}
}
