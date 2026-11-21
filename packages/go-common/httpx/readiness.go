package httpx

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Vineth-R/swiftdrop/packages/go-common/postgres"
)

// ReadinessDependency is one named capability a service needs before it can
// serve its required workload. A nil Ready func is down so a wiring mistake
// fails closed.
type ReadinessDependency struct {
	Name  string
	Ready func(context.Context) bool
}

type readinessResponse struct {
	Status string            `json:"status"`
	Checks map[string]string `json:"checks,omitempty"`
}

// ReadinessHandler reports ready only when every dependency is ready. Liveness
// stays a separate endpoint: a temporary broker or database outage must not
// cause an orchestrator to restart a process that can recover.
func ReadinessHandler(dependencies ...ReadinessDependency) gin.HandlerFunc {
	return func(c *gin.Context) {
		checks := make(map[string]string, len(dependencies))
		ready := true
		for _, dependency := range dependencies {
			ok := dependency.Ready != nil && dependency.Ready(c.Request.Context())
			checks[dependency.Name] = readinessWord(ok)
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

// DatabaseDependency builds the standard bounded PostgreSQL readiness check.
func DatabaseDependency(pinger postgres.Pinger, timeout time.Duration) ReadinessDependency {
	if timeout <= 0 {
		timeout = postgres.DefaultReadyTimeout
	}
	return ReadinessDependency{
		Name: "database",
		Ready: func(ctx context.Context) bool {
			if pinger == nil {
				return false
			}
			pingCtx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()
			return pinger.Ping(pingCtx) == nil
		},
	}
}

// BooleanDependency adapts a cheap in-process health flag to a readiness check.
func BooleanDependency(name string, ready func() bool) ReadinessDependency {
	return ReadinessDependency{Name: name, Ready: func(context.Context) bool {
		return ready != nil && ready()
	}}
}

func readinessWord(ok bool) string {
	if ok {
		return "ok"
	}
	return "down"
}
