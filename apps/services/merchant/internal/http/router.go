package servicehttp

import (
	"log/slog"

	"github.com/gin-gonic/gin"

	"github.com/Vineth-R/swiftdrop/apps/services/merchant/internal/identityclient"
	"github.com/Vineth-R/swiftdrop/apps/services/merchant/internal/onboarding"
	"github.com/Vineth-R/swiftdrop/apps/services/merchant/internal/orders"
	"github.com/Vineth-R/swiftdrop/apps/services/merchant/internal/repository"
	"github.com/Vineth-R/swiftdrop/packages/go-common/httpx"
)

// Options configures the merchant router.
type Options struct {
	Repository    *repository.Repository
	Onboarding    *onboarding.Service
	Identity      *identityclient.Client
	Orders        *orders.Service
	AccessSecret  string
	CookieName    string
	InternalToken string
	Logger        *slog.Logger
}

func New(logger *slog.Logger, repositories ...*repository.Repository) *gin.Engine {
	router := httpx.NewRouter(logger)
	httpx.RegisterHealth(router)
	if len(repositories) > 0 && repositories[0] != nil {
		registerMerchantRoutes(router, Options{Repository: repositories[0], Logger: logger})
	}
	return router
}

// Mount registers the configured merchant routes on an existing router.
func Mount(router *gin.Engine, options Options) {
	if options.Repository != nil {
		registerMerchantRoutes(router, options)
	}
}

// NewWithOptions builds the merchant router with optional authenticated
// registration, lifecycle and merchant business routes.
func NewWithOptions(options Options) *gin.Engine {
	router := New(options.Logger)
	Mount(router, options)
	return router
}
