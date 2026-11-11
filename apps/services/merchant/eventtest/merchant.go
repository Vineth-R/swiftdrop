//go:build integration

// Package eventtest exposes the merchant internal contact router the
// cross-service event-flow integration test needs. It is compiled only with the
// integration tag and is not part of any production build.
package eventtest

import (
	"log/slog"
	"net/http"

	"github.com/Vineth-R/swiftdrop/apps/services/merchant/internal/http"
	"github.com/Vineth-R/swiftdrop/apps/services/merchant/internal/repository"
	"github.com/Vineth-R/swiftdrop/packages/go-common/postgres"
)

// NewContactRouter returns the real merchant router with the internal contact
// lookup backed by the isolated database.
func NewContactRouter(pool *postgres.Pool, internalToken string, logger *slog.Logger) (http.Handler, error) {
	repo, err := repository.New(pool)
	if err != nil {
		return nil, err
	}
	return servicehttp.NewWithOptions(servicehttp.Options{
		Repository: repo, InternalToken: internalToken, Logger: logger,
	}), nil
}
