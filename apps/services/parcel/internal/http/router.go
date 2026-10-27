package servicehttp

import (
	"log/slog"

	"github.com/gin-gonic/gin"

	"github.com/Vineth-R/swiftdrop/packages/go-common/httpx"
)

func New(logger *slog.Logger) *gin.Engine {
	router := httpx.NewRouter(logger)
	httpx.RegisterHealth(router)
	return router
}
