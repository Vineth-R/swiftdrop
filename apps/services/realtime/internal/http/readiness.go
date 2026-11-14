package servicehttp

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Vineth-R/swiftdrop/packages/go-common/postgres"
)

// Dependency is one named readiness check. A nil Ready check is treated as
// down so a wiring mistake fails closed instead of reporting a false ready.
type Dependency struct {
	Name  string
	Ready func() bool
}

type readinessResponse struct {
	Status string            `json:"status"`
	Checks map[string]string `json:"checks,omitempty"`
}

// ReadyHandler reports readiness only when PostgreSQL and every required
// message consumer can do their job. Liveness stays the separate /health
// endpoint: a consumer that is reconnecting must not cause the orchestrator to
// restart a process that is still capable of reconnecting.
func ReadyHandler(pinger postgres.Pinger, timeout time.Duration, dependencies ...Dependency) gin.HandlerFunc {
	if timeout <= 0 {
		timeout = postgres.DefaultReadyTimeout
	}

	return func(c *gin.Context) {
		checks := make(map[string]string, len(dependencies)+1)
		ready := true

		databaseReady := false
		if pinger != nil {
			ctx, cancel := context.WithTimeout(c.Request.Context(), timeout)
			defer cancel()
			databaseReady = pinger.Ping(ctx) == nil
		}
		checks["database"] = statusWord(databaseReady)
		if !databaseReady {
			ready = false
		}

		for _, dependency := range dependencies {
			ok := dependency.Ready != nil && dependency.Ready()
			checks[dependency.Name] = statusWord(ok)
			if !ok {
				ready = false
			}
		}

		if !ready {
			c.JSON(http.StatusServiceUnavailable, readinessResponse{Status: "unavailable", Checks: checks})
			return
		}
		c.JSON(http.StatusOK, readinessResponse{Status: "ready", Checks: checks})
	}
}

func statusWord(ok bool) string {
	if ok {
		return "ok"
	}
	return "down"
}
