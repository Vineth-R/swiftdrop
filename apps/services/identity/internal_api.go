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
// When INTERNAL_TOKEN is missing or too short the endpoints are still mounted
// but closed: every call gets 401. They are not left out, because a 404 would
// look to callers like "user not found" and hide the misconfiguration.
func internalRouterOption(logger *slog.Logger, repo *repository.Repository) (servicehttp.Option, error) {
	api, err := service.NewInternalAPI(repo)
	if err != nil {
		return nil, fmt.Errorf("create internal api: %w", err)
	}

	token, problem := serviceconfig.LoadInternalToken(sharedconfig.Load())
	if problem != "" {
		logger.Warn("Internal APIs are closed and refuse every request: "+problem,
			"routes_closed", "/internal/couriers/{courierId}/eligibility, /internal/users/{userId}/contact")
	} else {
		logger.Info("Internal APIs are enabled")
	}
	return servicehttp.WithInternal(servicehttp.InternalRoutes{Service: api, Token: token}), nil
}
