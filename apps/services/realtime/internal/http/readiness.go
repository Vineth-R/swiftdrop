package servicehttp

import (
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Vineth-R/swiftdrop/packages/go-common/httpx"
	"github.com/Vineth-R/swiftdrop/packages/go-common/postgres"
)

// Dependency is one named readiness check.
type Dependency struct {
	Name  string
	Ready func() bool
}

// ReadyHandler reports readiness only when PostgreSQL and every required
// message consumer can do their job. Liveness stays the separate /health
// endpoint. It delegates the response shape and fail-closed rules to the shared
// readiness helper.
func ReadyHandler(pinger postgres.Pinger, timeout time.Duration, dependencies ...Dependency) gin.HandlerFunc {
	shared := make([]httpx.ReadinessDependency, 0, len(dependencies)+1)
	shared = append(shared, httpx.DatabaseDependency(pinger, timeout))
	for _, dependency := range dependencies {
		shared = append(shared, httpx.BooleanDependency(dependency.Name, dependency.Ready))
	}
	return httpx.ReadinessHandler(shared...)
}
