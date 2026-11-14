package main

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/gin-gonic/gin"

	serviceconfig "github.com/Vineth-R/swiftdrop/apps/services/merchant/internal/config"
	servicehttp "github.com/Vineth-R/swiftdrop/apps/services/merchant/internal/http"
	"github.com/Vineth-R/swiftdrop/apps/services/merchant/internal/identityclient"
	merchantrepository "github.com/Vineth-R/swiftdrop/apps/services/merchant/internal/repository"
	"github.com/Vineth-R/swiftdrop/packages/go-common/secretbox"
	"github.com/Vineth-R/swiftdrop/packages/webhookdelivery"
)

// registerWebhooks adds the webhook settings endpoints (API 8.12 to 8.15).
//
// The routes are always registered so their fail-closed state is explicit:
// without WEBHOOK_SECRET_KEY (no Box) or without JWT_ACCESS_SECRET they answer
// 503 SERVICE_UNAVAILABLE. Config.Load has already rejected a present but
// invalid WEBHOOK_SECRET_KEY, so the NewFromHex call below is a defensive
// check rather than the primary validation.
func registerWebhooks(router *gin.Engine, repo *merchantrepository.Repository, identityClient *identityclient.Client, config serviceconfig.Config, logger *slog.Logger) error {
	var ring *secretbox.Keyring
	if config.WebhookSecretKey == "" {
		logger.Warn("WEBHOOK_SECRET_KEY is not set; webhook settings endpoints will answer 503")
	} else {
		parsed, err := secretbox.NewKeyringFromHex(config.WebhookSecretKeyID, config.WebhookSecretKey, config.WebhookSecretKeys)
		if err != nil {
			return fmt.Errorf("invalid WEBHOOK_SECRET_KEY configuration: %w", err)
		}
		ring = parsed
	}
	if config.AccessSecret == "" {
		logger.Warn("JWT_ACCESS_SECRET is not set; webhook settings endpoints will answer 503")
	}

	webhookConfig := servicehttp.WebhookConfig{
		Repository:   repo,
		Sender:       webhookdelivery.NewSender(),
		AccessSecret: config.AccessSecret,
		CookieName:   config.CookieName,
		Logger:       logger,
		SessionCheck: func(ctx context.Context, userID string) (bool, int, error) {
			state, err := identityClient.SessionState(ctx, userID)
			if err != nil {
				return false, 0, err
			}
			return state.IsActive, state.SessionVersion, nil
		},
	}
	if ring != nil {
		webhookConfig.Cipher = ring
	}
	return servicehttp.RegisterWebhookRoutes(router, webhookConfig)
}
