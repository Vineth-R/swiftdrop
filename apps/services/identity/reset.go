package main

import (
	"fmt"
	"log/slog"
	"time"

	servicehttp "github.com/Vineth-R/swiftdrop/apps/services/identity/internal/http"
	"github.com/Vineth-R/swiftdrop/apps/services/identity/internal/repository"
	"github.com/Vineth-R/swiftdrop/apps/services/identity/internal/service"
)

// resetRouterOption builds forgot and reset password (API v1.1 sections 2.16
// and 2.17). Reset needs email to be set up (see setUpEmail: an SMTP server and
// a trusted FRONTEND_BASE_URL to build links from). When it is not, it logs a
// warning and returns a nil option, so the service and CI still start without
// the reset routes. The returned function waits for background email delivery;
// it is never nil on success.
func resetRouterOption(
	logger *slog.Logger,
	repo *repository.Repository,
	identity *service.Service,
	cookies servicehttp.CookieConfig,
	passwordResetTTL time.Duration,
	mail emailSetup,
) (servicehttp.Option, func(), error) {
	noop := func() {}
	if mail.Sender == nil {
		logger.Warn("Password reset is disabled: "+mail.DisabledReason,
			"routes_disabled", "/auth/forgot-password, /auth/reset-password")
		return nil, noop, nil
	}

	reset, err := service.NewPasswordReset(identity, repo, mail.Sender, mail.Site.BaseURL, passwordResetTTL,
		service.WithSupportEmail(mail.Site.SupportEmail))
	if err != nil {
		return nil, nil, fmt.Errorf("create password reset: %w", err)
	}

	logger.Info("Password reset is enabled")
	return servicehttp.WithPasswordReset(servicehttp.PasswordResetRoutes{Service: reset, Cookies: cookies}), reset.Wait, nil
}
