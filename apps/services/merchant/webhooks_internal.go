package main

import (
	"fmt"
	"log/slog"

	"github.com/gin-gonic/gin"

	serviceconfig "github.com/Vineth-R/swiftdrop/apps/services/merchant/internal/config"
	servicehttp "github.com/Vineth-R/swiftdrop/apps/services/merchant/internal/http"
	merchantrepository "github.com/Vineth-R/swiftdrop/apps/services/merchant/internal/repository"
	"github.com/Vineth-R/swiftdrop/packages/go-common/secretbox"
)

// registerInternalWebhooks adds the two service-to-service endpoints the
// webhook worker uses (API v1.1 section 13): read a merchant's webhook with its
// signing secret, and record a delivery attempt.
//
// The routes are always registered so their fail-closed state is explicit:
// without INTERNAL_TOKEN every call answers 401, and without WEBHOOK_SECRET_KEY
// a call with the right token answers 503. Nginx never routes /internal, so only
// services on the Compose network can reach them. Config.Load has already
// rejected a present but invalid WEBHOOK_SECRET_KEY, so the NewFromHex call is a
// defensive check rather than the primary validation.
func registerInternalWebhooks(router *gin.Engine, repo *merchantrepository.Repository, config serviceconfig.Config, logger *slog.Logger) error {
	var ring *secretbox.Keyring
	if config.WebhookSecretKey != "" {
		parsed, err := secretbox.NewKeyringFromHex(config.WebhookSecretKeyID, config.WebhookSecretKey, config.WebhookSecretKeys)
		if err != nil {
			return fmt.Errorf("invalid WEBHOOK_SECRET_KEY configuration: %w", err)
		}
		ring = parsed
	}

	switch {
	case config.InternalToken == "":
		// The problem text never contains the token itself.
		logger.Warn("internal webhook endpoints are closed", "reason", config.InternalTokenProblem)
	case ring == nil:
		logger.Warn("WEBHOOK_SECRET_KEY is not set; internal webhook endpoints will answer 503")
	}

	internalConfig := servicehttp.InternalWebhookConfig{
		Repository: repo,
		Token:      config.InternalToken,
		Logger:     logger,
	}
	if ring != nil {
		internalConfig.Cipher = ring
	}
	servicehttp.RegisterInternalWebhookRoutes(router, internalConfig)
	return nil
}
