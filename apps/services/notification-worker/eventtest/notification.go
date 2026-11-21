//go:build integration

// Package eventtest exposes the notification handler the cross-service
// event-flow integration test needs. It is compiled only with the integration
// tag and is not part of any production build.
package eventtest

import (
	"log/slog"
	"time"

	"github.com/Vineth-R/swiftdrop/apps/services/notification-worker/internal/idempotency"
	"github.com/Vineth-R/swiftdrop/apps/services/notification-worker/internal/identityclient"
	"github.com/Vineth-R/swiftdrop/apps/services/notification-worker/internal/merchantclient"
	"github.com/Vineth-R/swiftdrop/apps/services/notification-worker/internal/worker"
	"github.com/Vineth-R/swiftdrop/packages/go-common/emails"
	"github.com/Vineth-R/swiftdrop/packages/go-common/mailer"
	"github.com/Vineth-R/swiftdrop/packages/go-common/postgres"
)

// NewHandler wires the real notification handler against the isolated database
// and the supplied identity and merchant HTTP endpoints.
func NewHandler(
	logger *slog.Logger,
	sender mailer.Mailer,
	pool *postgres.Pool,
	consumerName string,
	identityURL string,
	merchantURL string,
	internalToken string,
	site emails.Site,
) (*worker.Handler, error) {
	identity, err := identityclient.New(identityURL, internalToken, 2*time.Second)
	if err != nil {
		return nil, err
	}
	merchants, err := merchantclient.New(merchantURL, internalToken, 2*time.Second)
	if err != nil {
		return nil, err
	}
	return worker.New(logger, sender, idempotency.NewPostgres(pool, consumerName), identity, merchants, site), nil
}
