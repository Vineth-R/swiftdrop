//go:build integration

package eventtest

import (
	"log/slog"

	"github.com/Vineth-R/swiftdrop/apps/services/merchant/internal/merchantsync"
	"github.com/Vineth-R/swiftdrop/apps/services/merchant/internal/repository"
	"github.com/Vineth-R/swiftdrop/packages/go-common/messaging"
	"github.com/Vineth-R/swiftdrop/packages/go-common/postgres"
)

// NewSyncHandler returns the real merchant order status synchronization
// handler backed by the isolated database.
func NewSyncHandler(pool *postgres.Pool, logger *slog.Logger) (messaging.Handler, error) {
	repo, err := repository.New(pool)
	if err != nil {
		return nil, err
	}
	return merchantsync.New(repo, logger), nil
}
