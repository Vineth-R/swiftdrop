package main

import (
	"fmt"

	servicehttp "github.com/Vineth-R/swiftdrop/apps/services/identity/internal/http"
	"github.com/Vineth-R/swiftdrop/apps/services/identity/internal/repository"
	"github.com/Vineth-R/swiftdrop/apps/services/identity/internal/service"
	"github.com/Vineth-R/swiftdrop/packages/go-common/auth"
)

// adminRouterOption builds the admin user endpoints (API v1.1 sections 2.10 to
// 2.14) and GET /couriers (section 2.15). They reuse the access-token verifier
// and cookies of the auth endpoints, so a caller is authenticated exactly as
// elsewhere, and the role checks run inside the router.
func adminRouterOption(
	repo *repository.Repository,
	identity *service.Service,
	verifier auth.Verifier,
	cookies servicehttp.CookieConfig,
) (servicehttp.Option, error) {
	admin, err := service.NewAdminUsers(identity, repo)
	if err != nil {
		return nil, fmt.Errorf("create admin users service: %w", err)
	}
	return servicehttp.WithAdminUsers(servicehttp.AdminRoutes{Service: admin, Verifier: verifier, Cookies: cookies}), nil
}
