package servicehttp

import (
	"log/slog"

	"github.com/gin-gonic/gin"

	"github.com/Vineth-R/swiftdrop/packages/go-common/httpx"
)

// Option configures the identity router.
type Option func(*routerOptions)

type routerOptions struct {
	auth *AuthRoutes
}

// WithAuth mounts the authentication endpoints (API v1.1 sections 2.1 to 2.4).
func WithAuth(routes AuthRoutes) Option {
	return func(options *routerOptions) {
		options.auth = &routes
	}
}

func New(logger *slog.Logger, opts ...Option) *gin.Engine {
	options := routerOptions{}
	for _, opt := range opts {
		opt(&options)
	}

	router := httpx.NewRouter(logger)
	trustGatewayClientAddress(router)
	httpx.RegisterHealth(router)

	if options.auth != nil {
		registerAuthRoutes(router, *options.auth, logger)
	}
	return router
}

// trustGatewayClientAddress makes c.ClientIP() return the address Nginx sets
// in X-Real-IP (it overwrites any client-supplied value) and ignore
// X-Forwarded-For, which a client can forge. Only the gateway can reach this
// service, so the header is trustworthy; rate limits and the stored login
// address depend on it.
func trustGatewayClientAddress(router *gin.Engine) {
	_ = router.SetTrustedProxies(nil)
	router.TrustedPlatform = "X-Real-IP"
}
