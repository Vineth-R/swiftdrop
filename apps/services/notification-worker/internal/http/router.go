package servicehttp

import (
	"log/slog"

	"github.com/gin-gonic/gin"

	"github.com/Vineth-R/swiftdrop/packages/go-common/httpx"
	"github.com/Vineth-R/swiftdrop/packages/go-common/sessioncheck"
)

// Options configures the notification-worker HTTP surface.
type Options struct {
	Store        NotificationLogStore
	AccessSecret string
	CookieName   string
	// SessionCheck re-checks the ADMIN session against identity. A nil value
	// fails closed when authentication is configured.
	SessionCheck sessioncheck.Guard
}

// New returns the notification-worker HTTP router. The operator endpoints are
// mounted only when their dependencies are configured; the worker's delivery
// loop is unaffected by this router.
func New(logger *slog.Logger, configurations ...Options) (*gin.Engine, error) {
	router := httpx.NewRouter(logger)
	httpx.RegisterHealth(router)

	options := Options{}
	if len(configurations) > 0 {
		options = configurations[0]
	}
	if err := registerNotificationLogs(router, logger, options); err != nil {
		return nil, err
	}
	return router, nil
}
