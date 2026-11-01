package main

import (
	"fmt"
	"log/slog"

	serviceconfig "github.com/Vineth-R/swiftdrop/apps/services/identity/internal/config"
	servicehttp "github.com/Vineth-R/swiftdrop/apps/services/identity/internal/http"
	"github.com/Vineth-R/swiftdrop/apps/services/identity/internal/lockout"
	"github.com/Vineth-R/swiftdrop/apps/services/identity/internal/password"
	"github.com/Vineth-R/swiftdrop/apps/services/identity/internal/repository"
	"github.com/Vineth-R/swiftdrop/apps/services/identity/internal/service"
	"github.com/Vineth-R/swiftdrop/packages/go-common/auth"
	"github.com/Vineth-R/swiftdrop/packages/go-common/postgres"
)

// authRouterOptions builds the authentication endpoints (API v1.1 sections 2.1
// to 2.4) on top of the database, and the optional Google sign-in endpoints
// (sections 2.5 and 2.6), password reset (sections 2.16 and 2.17), the internal
// APIs (section 13) and admin users with GET /couriers (sections 2.10 to 2.15). It fails
// fast on a missing signing secret. The returned function waits for background
// email delivery and must be called on shutdown; it is never nil on success.
func authRouterOptions(logger *slog.Logger, config serviceconfig.Config, database *postgres.Pool) ([]servicehttp.Option, func(), error) {
	appEnv := config.AppEnv
	authConfig, err := serviceconfig.LoadAuth(appEnv)
	if err != nil {
		return nil, nil, fmt.Errorf("load auth config: %w", err)
	}
	if appEnv == "development" && auth.CheckSecretStrength(authConfig.AccessSecret) != nil {
		logger.Warn("JWT_ACCESS_SECRET is shorter than 32 bytes; this is allowed only in development")
	}

	repo, err := repository.New(database)
	if err != nil {
		return nil, nil, fmt.Errorf("create identity repository: %w", err)
	}
	hasher, err := password.NewHasher(password.DefaultCost)
	if err != nil {
		return nil, nil, fmt.Errorf("create password hasher: %w", err)
	}
	tokens, err := auth.NewTokenManager(authConfig.AccessSecret, authConfig.AccessTokenTTL)
	if err != nil {
		return nil, nil, fmt.Errorf("create token manager: %w", err)
	}
	identity, err := service.New(repo, hasher, tokens, service.Config{
		RefreshTTL: authConfig.RefreshTokenTTL,
		Lockout: lockout.Policy{
			MaxFailures: authConfig.MaxFailedLogins,
			Duration:    authConfig.LockoutDuration,
		},
	}, service.WithLogger(logger))
	if err != nil {
		return nil, nil, fmt.Errorf("create identity service: %w", err)
	}

	cookies := servicehttp.CookieConfig{
		AccessName:  authConfig.AccessCookieName,
		RefreshName: authConfig.RefreshCookieName,
		Secure:      authConfig.CookieSecure,
		SameSite:    authConfig.CookieSameSite,
	}
	options := []servicehttp.Option{
		servicehttp.WithAuth(servicehttp.AuthRoutes{Service: identity, Verifier: tokens, Cookies: cookies}),
	}

	googleOption, err := googleRouterOption(logger, config, repo, identity, cookies)
	if err != nil {
		return nil, nil, err
	}
	if googleOption != nil {
		options = append(options, googleOption)
	}

	resetOption, drain, err := resetRouterOption(logger, repo, identity, cookies)
	if err != nil {
		return nil, nil, err
	}
	if resetOption != nil {
		options = append(options, resetOption)
	}

	internalOption, err := internalRouterOption(logger, repo)
	if err != nil {
		return nil, nil, err
	}
	options = append(options, internalOption)

	adminOption, err := adminRouterOption(repo, identity, tokens, cookies)
	if err != nil {
		return nil, nil, err
	}
	options = append(options, adminOption)
	return options, drain, nil
}
