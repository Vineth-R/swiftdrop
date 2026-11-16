package servicehttp

import (
	"io"
	"log/slog"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/Vineth-R/swiftdrop/packages/go-common/routecontract"
)

// TestRouteInventory constructs the router with zero-value dependencies. The
// handlers fail closed at request time; registration does not need a database.
func TestRouteInventory(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := New(
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		WithAuth(AuthRoutes{}),
		WithGoogle(GoogleRoutes{}),
		WithPasswordReset(PasswordResetRoutes{}),
		WithProfile(ProfileRoutes{}),
		WithInternal(InternalRoutes{}),
		WithAdminUsers(AdminRoutes{}),
	)
	routecontract.Verify(t, router, routecontract.Policy{Service: "identity"})
}
