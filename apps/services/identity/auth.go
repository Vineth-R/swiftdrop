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
// to 2.4) on top of the database. It fails fast on a missing signing secret.
func authRouterOptions(logger *slog.Logger, appEnv string, database *postgres.Pool) (servicehttp.Option, error) {
	authConfig, err := serviceconfig.LoadAuth(appEnv)
	if err != nil {
		return nil, fmt.Errorf("load auth config: %w", err)
	}
	if appEnv == "development" && auth.CheckSecretStrength(authConfig.AccessSecret) != nil {
		logger.Warn("JWT_ACCESS_SECRET is shorter than 32 bytes; this is allowed only in development")
	}

	repo, err := repository.New(database)
	if err != nil {
		return nil, fmt.Errorf("create identity repository: %w", err)
	}
	hasher, err := password.NewHasher(password.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("create password hasher: %w", err)
	}
	tokens, err := auth.NewTokenManager(authConfig.AccessSecret, authConfig.AccessTokenTTL)
	if err != nil {
		return nil, fmt.Errorf("create token manager: %w", err)
	}
	identity, err := service.New(repo, hasher, tokens, service.Config{
		RefreshTTL: authConfig.RefreshTokenTTL,
		Lockout: lockout.Policy{
			MaxFailures: authConfig.MaxFailedLogins,
			Duration:    authConfig.LockoutDuration,
		},
	}, service.WithLogger(logger))
	if err != nil {
		return nil, fmt.Errorf("create identity service: %w", err)
	}

	return servicehttp.WithAuth(servicehttp.AuthRoutes{
		Service:  identity,
		Verifier: tokens,
		Cookies: servicehttp.CookieConfig{
			AccessName:  authConfig.AccessCookieName,
			RefreshName: authConfig.RefreshCookieName,
			Secure:      authConfig.CookieSecure,
			SameSite:    authConfig.CookieSameSite,
		},
	}), nil
}
