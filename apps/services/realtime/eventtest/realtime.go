//go:build integration

// Package eventtest exposes the realtime consumer the cross-service event-flow
// integration test needs. It is compiled only with the integration tag and is
// not part of any production build.
package eventtest

import (
	"github.com/Vineth-R/swiftdrop/apps/services/realtime/internal/consumer"
	"github.com/Vineth-R/swiftdrop/apps/services/realtime/internal/live"
	"github.com/Vineth-R/swiftdrop/packages/go-common/messaging"
)

// Update is the public realtime projection.
type Update = live.Update

// Broadcaster delivers a live update.
type Broadcaster = consumer.Broadcaster

// NewConsumer returns the real realtime event consumer.
func NewConsumer(broadcaster Broadcaster) messaging.Handler { return consumer.New(broadcaster) }
