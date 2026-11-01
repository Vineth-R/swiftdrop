package main

import (
	"fmt"
	"log/slog"

	servicehttp "github.com/Vineth-R/swiftdrop/apps/services/identity/internal/http"
	"github.com/Vineth-R/swiftdrop/apps/services/identity/internal/repository"
	"github.com/Vineth-R/swiftdrop/apps/services/identity/internal/service"
	sharedconfig "github.com/Vineth-R/swiftdrop/packages/go-common/config"
	"github.com/Vineth-R/swiftdrop/packages/go-common/mailer"
)

// resetRouterOption builds forgot and reset password (API v1.1 sections 2.16
// and 2.17). Reset needs an SMTP server (SMTP_HOST and friends) and a trusted
// FRONTEND_BASE_URL to build links from. When either is missing or unusable it
// logs a warning and returns a nil option, so the service and CI still start
// without the reset routes. The returned function waits for background email
// delivery; it is never nil on success.
func resetRouterOption(
	logger *slog.Logger,
	repo *repository.Repository,
	identity *service.Service,
	cookies servicehttp.CookieConfig,
) (servicehttp.Option, func(), error) {
	noop := func() {}
	loader := sharedconfig.Load()

	mailConfig, err := mailer.LoadConfig(loader)
	if err != nil {
		return nil, nil, fmt.Errorf("load mailer config: %w", err)
	}
	if err := mailConfig.Validate(); err != nil {
		logger.Warn("Password reset is disabled: "+err.Error(),
			"routes_disabled", "/auth/forgot-password, /auth/reset-password")
		return nil, noop, nil
	}
	sender, err := mailer.NewSMTP(mailConfig)
	if err != nil {
		return nil, nil, fmt.Errorf("create mailer: %w", err)
	}

	reset, err := service.NewPasswordReset(identity, repo, sender, loader.String("FRONTEND_BASE_URL", ""))
	if err != nil {
		logger.Warn("Password reset is disabled: FRONTEND_BASE_URL must be an absolute http or https URL",
			"routes_disabled", "/auth/forgot-password, /auth/reset-password")
		return nil, noop, nil
	}

	logger.Info("Password reset is enabled")
	return servicehttp.WithPasswordReset(servicehttp.PasswordResetRoutes{Service: reset, Cookies: cookies}), reset.Wait, nil
}
