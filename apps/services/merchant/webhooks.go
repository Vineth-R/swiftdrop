package main

import (
	"fmt"
	"log/slog"

	"github.com/gin-gonic/gin"

	servicehttp "github.com/Vineth-R/swiftdrop/apps/services/merchant/internal/http"
	merchantrepository "github.com/Vineth-R/swiftdrop/apps/services/merchant/internal/repository"
	"github.com/Vineth-R/swiftdrop/packages/go-common/secretbox"
	"github.com/Vineth-R/swiftdrop/packages/go-common/webhookdelivery"
)

// registerWebhooks adds the webhook settings endpoints (API 8.12 to 8.15).
//
// WEBHOOK_SECRET_KEY encrypts the signing secrets. When it is not set the
// endpoints are simply not registered and a warning is logged, so a machine
// without the key still starts. A key that is set but invalid stops the
// service, because running with a wrong key would store unreadable secrets.
func registerWebhooks(router *gin.Engine, repo *merchantrepository.Repository, hexKey string, logger *slog.Logger) error {
	if hexKey == "" {
		logger.Warn("WEBHOOK_SECRET_KEY is not set; webhook settings endpoints are disabled")
		return nil
	}
	box, err := secretbox.NewFromHex(hexKey)
	if err != nil {
		return fmt.Errorf("invalid WEBHOOK_SECRET_KEY: %w", err)
	}
	servicehttp.RegisterWebhookRoutes(router, repo, box, webhookdelivery.NewSender(), logger)
	logger.Info("webhook settings endpoints enabled")
	return nil
}
