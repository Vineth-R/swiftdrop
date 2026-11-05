package main

import (
	"fmt"
	"log/slog"

	serviceconfig "github.com/Vineth-R/swiftdrop/apps/services/identity/internal/config"
	servicehttp "github.com/Vineth-R/swiftdrop/apps/services/identity/internal/http"
	"github.com/Vineth-R/swiftdrop/apps/services/identity/internal/repository"
	"github.com/Vineth-R/swiftdrop/apps/services/identity/internal/service"
	sharedconfig "github.com/Vineth-R/swiftdrop/packages/go-common/config"
)

// internalRouterOption builds the service-to-service endpoints (API v1.1
// section 13), protected by X-Internal-Token from INTERNAL_TOKEN.
//
// In development, when INTERNAL_TOKEN is missing or too short the endpoints
// are still mounted but closed: every call gets 401. They are not left out,
// because a 404 would look to callers like "user not found" and hide the
// misconfiguration. Outside development an invalid token is a startup error.
func internalRouterOption(logger *slog.Logger, repo *repository.Repository, appEnv string) (servicehttp.Option, error) {
	token, problem := serviceconfig.LoadInternalToken(sharedconfig.Load(), appEnv)
	if problem != "" {
		if appEnv != "development" {
			return nil, fmt.Errorf("load internal token: %s", problem)
		}
		logger.Warn("Internal APIs are closed and refuse every request: "+problem,
			"routes_closed", "/internal/couriers/{courierId}/eligibility, /internal/users/{userId}/contact")
	} else {
		logger.Info("Internal APIs are enabled")
	}

	api, err := service.NewInternalAPI(repo)
	if err != nil {
		return nil, fmt.Errorf("create internal api: %w", err)
	}
	return servicehttp.WithInternal(servicehttp.InternalRoutes{Service: api, Token: token}), nil
}
