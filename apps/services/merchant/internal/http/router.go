package servicehttp

import (
	"log/slog"

	"github.com/gin-gonic/gin"

	"github.com/Vineth-R/swiftdrop/apps/services/merchant/internal/repository"
	"github.com/Vineth-R/swiftdrop/packages/go-common/httpx"
)

func New(logger *slog.Logger, repositories ...*repository.Repository) *gin.Engine {
	router := httpx.NewRouter(logger)
	httpx.RegisterHealth(router)
	if len(repositories) > 0 && repositories[0] != nil {
		registerMerchantRoutes(router, repositories[0])
	}
	return router
}
