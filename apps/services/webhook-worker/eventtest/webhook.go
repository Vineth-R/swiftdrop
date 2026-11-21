//go:build integration

// Package eventtest exposes the webhook handler the cross-service event-flow
// integration test needs. It is compiled only with the integration tag and is
// not part of any production build.
package eventtest

import (
	"log/slog"
	"time"

	"github.com/Vineth-R/swiftdrop/apps/services/webhook-worker/internal/worker"
)

// Sender is the webhook delivery seam.
type Sender = worker.Sender

// Scheduler is the retry scheduling seam.
type Scheduler = worker.Scheduler

// Attempt is one recorded delivery attempt.
type Attempt = worker.Attempt

// NewHandler wires the real webhook handler against the supplied internal APIs.
func NewHandler(
	logger *slog.Logger,
	merchantURL string,
	parcelURL string,
	internalToken string,
	sender Sender,
	scheduler Scheduler,
) *worker.Handler {
	return worker.New(logger, worker.Deps{
		Merchants: worker.NewMerchantClient(merchantURL, internalToken, 2*time.Second),
		Parcels:   worker.NewParcelClient(parcelURL, internalToken, 2*time.Second),
		Sender:    sender,
		Scheduler: scheduler,
	})
}
