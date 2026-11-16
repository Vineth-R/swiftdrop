package servicehttp

import (
	"io"
	"log/slog"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/Vineth-R/swiftdrop/packages/go-common/routecontract"
)

func TestRouteInventory(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := New(slog.New(slog.NewTextHandler(io.Discard, nil)))
	RegisterWebSocket(router, http.NotFoundHandler())
	routecontract.Verify(t, router, routecontract.Policy{Service: "realtime"})
}
