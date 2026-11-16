package main

import (
	"io"
	"log/slog"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/Vineth-R/swiftdrop/packages/go-common/httpx"
	"github.com/Vineth-R/swiftdrop/packages/go-common/messaging"
	"github.com/Vineth-R/swiftdrop/packages/go-common/routecontract"
)

func TestRouteInventory(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := httpx.NewRouter(slog.New(slog.NewTextHandler(io.Discard, nil)))
	registerHTTPRoutes(router, messaging.NewHealth())
	routecontract.Verify(t, router, routecontract.Policy{Service: "webhook-worker"})
}
