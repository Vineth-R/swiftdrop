//go:build integration

// Package eventtest exposes the parcel components the cross-service event-flow
// integration test needs. It is compiled only with the integration tag and is
// not part of any production build.
package eventtest

import (
	"log/slog"

	"github.com/Vineth-R/swiftdrop/apps/services/parcel/internal/courier"
	"github.com/Vineth-R/swiftdrop/apps/services/parcel/internal/outbox"
	"github.com/Vineth-R/swiftdrop/apps/services/parcel/internal/statemachine"
	"github.com/Vineth-R/swiftdrop/packages/go-common/postgres"
)

// Publisher is the outbox publish seam.
type Publisher = outbox.Publisher

// Store is the outbox storage seam.
type Store = outbox.Store

// RelayOptions configures the real outbox relay.
type RelayOptions = outbox.RelayOptions

// StatusInput is one courier status transition.
type StatusInput = courier.StatusInput

// StatusPickedUp is the CREATED -> PICKED_UP transition used by the test.
const StatusPickedUp = statemachine.StatusPickedUp

// NewSQLStore returns the real outbox store over the isolated database.
func NewSQLStore(pool *postgres.Pool) *outbox.SQLStore { return outbox.NewSQLStore(pool) }

// NewAMQPPublisher returns the real publisher-confirm AMQP publisher.
func NewAMQPPublisher(amqpURL, exchange string) *outbox.AMQPPublisher {
	return outbox.NewAMQPPublisher(amqpURL, exchange)
}

// NewRelay returns the real outbox relay.
func NewRelay(store Store, publisher Publisher, logger *slog.Logger, options ...RelayOptions) *outbox.Relay {
	return outbox.NewRelay(store, publisher, logger, options...)
}

// NewCourierRepository returns the real courier status repository.
func NewCourierRepository(pool *postgres.Pool) *courier.Repository {
	return courier.NewRepository(pool)
}
