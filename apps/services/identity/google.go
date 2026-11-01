package main

import (
	"fmt"
	"log/slog"
	"strings"

	serviceconfig "github.com/Vineth-R/swiftdrop/apps/services/identity/internal/config"
	"github.com/Vineth-R/swiftdrop/apps/services/identity/internal/google"
	servicehttp "github.com/Vineth-R/swiftdrop/apps/services/identity/internal/http"
	"github.com/Vineth-R/swiftdrop/apps/services/identity/internal/repository"
	"github.com/Vineth-R/swiftdrop/apps/services/identity/internal/service"
)

// googleRouterOption builds the Google sign-in endpoints (API v1.1 sections 2.5
// and 2.6). Google sign-in is optional: when any of GOOGLE_CLIENT_ID,
// GOOGLE_CLIENT_SECRET or GOOGLE_REDIRECT_URI is missing or unusable it logs a
// warning and returns a nil option, so the service and CI still start without
// the Google routes.
func googleRouterOption(
	logger *slog.Logger,
	config serviceconfig.Config,
	repo *repository.Repository,
	identity *service.Service,
	cookies servicehttp.CookieConfig,
) (servicehttp.Option, error) {
	if problems := config.GoogleProblems(); len(problems) > 0 {
		logger.Warn("Google sign-in is disabled: "+strings.Join(problems, "; "),
			"routes_disabled", "/auth/google/login, /auth/google/callback")
		return nil, nil
	}

	client, err := google.New(google.Config{
		ClientID:     config.GoogleClientID,
		ClientSecret: config.GoogleClientSecret,
		RedirectURI:  config.GoogleRedirectURI,
	})
	if err != nil {
		return nil, fmt.Errorf("create google client: %w", err)
	}
	googleAuth, err := service.NewGoogleAuth(identity, repo, client)
	if err != nil {
		return nil, fmt.Errorf("create google auth service: %w", err)
	}

	logger.Info("Google sign-in is enabled")
	return servicehttp.WithGoogle(servicehttp.GoogleRoutes{Service: googleAuth, Cookies: cookies}), nil
}
