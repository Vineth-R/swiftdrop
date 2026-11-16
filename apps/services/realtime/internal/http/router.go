package servicehttp

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/Vineth-R/swiftdrop/packages/go-common/httpx"
)

func New(logger *slog.Logger) *gin.Engine {
	router := httpx.NewRouter(logger)
	httpx.RegisterHealth(router)
	return router
}

// RegisterWebSocket mounts the public /ws route. Keeping the registration here
// lets the route-inventory test enumerate the public surface without building
// the live hub or opening a broker connection.
func RegisterWebSocket(router *gin.Engine, handler http.Handler) {
	router.GET("/ws", gin.WrapH(handler))
}
