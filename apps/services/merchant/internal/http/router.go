package servicehttp

import (
	"log/slog"

	"github.com/gin-gonic/gin"

	"github.com/Vineth-R/swiftdrop/apps/services/merchant/internal/identityclient"
	"github.com/Vineth-R/swiftdrop/apps/services/merchant/internal/onboarding"
	"github.com/Vineth-R/swiftdrop/apps/services/merchant/internal/repository"
	"github.com/Vineth-R/swiftdrop/packages/go-common/httpx"
)

// Options configures the merchant router.
type Options struct {
	Repository    *repository.Repository
	Onboarding    *onboarding.Service
	Identity      *identityclient.Client
	AccessSecret  string
	CookieName    string
	InternalToken string
	Logger        *slog.Logger
}

func New(logger *slog.Logger, repositories ...*repository.Repository) *gin.Engine {
	router := httpx.NewRouter(logger)
	httpx.RegisterHealth(router)
	if len(repositories) > 0 && repositories[0] != nil {
		Mount(router, Options{Repository: repositories[0], Logger: logger})
	}
	return router
}

// Mount registers the configured merchant routes on an existing router. The
// internal merchant lookup is always registered so a missing repository fails
// closed with 503 instead of looking like an unknown path.
func Mount(router *gin.Engine, options Options) {
	RegisterInternalMerchantRoutes(router, options.Repository, options.InternalToken)
	if options.Repository != nil {
		registerMerchantRoutes(router, options)
	}
}

// NewWithOptions builds the merchant router with optional authenticated
// registration, lifecycle and merchant business routes.
func NewWithOptions(options Options) *gin.Engine {
	router := httpx.NewRouter(options.Logger)
	httpx.RegisterHealth(router)
	Mount(router, options)
	return router
}
