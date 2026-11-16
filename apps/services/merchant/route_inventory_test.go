package main

import (
	"io"
	"log/slog"
	"testing"

	"github.com/gin-gonic/gin"

	serviceconfig "github.com/Vineth-R/swiftdrop/apps/services/merchant/internal/config"
	servicehttp "github.com/Vineth-R/swiftdrop/apps/services/merchant/internal/http"
	merchantrepository "github.com/Vineth-R/swiftdrop/apps/services/merchant/internal/repository"
	"github.com/Vineth-R/swiftdrop/packages/go-common/routecontract"
)

// TestRouteInventory includes the routes registered by the HTTP package and the
// webhook helpers that main mounts separately. The zero-value repository makes
// registration possible; handlers fail closed without a session or database.
func TestRouteInventory(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	repository := &merchantrepository.Repository{}
	router := servicehttp.New(logger)
	servicehttp.Mount(router, servicehttp.Options{
		Repository: repository,
		Logger:     logger,
	})
	if err := registerWebhooks(router, repository, nil, serviceconfig.Config{}, logger); err != nil {
		t.Fatalf("registerWebhooks() = %v", err)
	}
	if err := registerInternalWebhooks(router, nil, serviceconfig.Config{}, logger); err != nil {
		t.Fatalf("registerInternalWebhooks() = %v", err)
	}
	routecontract.Verify(t, router, routecontract.Policy{Service: "merchant"})
}
