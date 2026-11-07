package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	serviceconfig "github.com/Vineth-R/swiftdrop/apps/services/webhook-worker/internal/config"
	"github.com/Vineth-R/swiftdrop/apps/services/webhook-worker/internal/worker"
	"github.com/Vineth-R/swiftdrop/packages/go-common/logging"
	"github.com/Vineth-R/swiftdrop/packages/go-common/messaging"
)

const serviceName = "webhook-worker"

func main() {
	if err := run(); err != nil {
		logging.NewBootstrap(serviceName).Error("worker stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	config, err := serviceconfig.Load()
	if err != nil {
		return err
	}

	logger, err := logging.New(serviceName, config.AppEnv, config.LogLevel)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	signals := make(chan os.Signal, 2)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)

	stopped := make(chan struct{})
	go func() {
		defer signal.Stop(signals)

		select {
		case <-stopped:
			return
		case <-signals:
		}

		cancel()

		select {
		case <-stopped:
		case sig := <-signals:
			signal.Stop(signals)
			shutdown, ok := sig.(syscall.Signal)
			if !ok {
				os.Exit(1)
			}
			if err := syscall.Kill(os.Getpid(), shutdown); err != nil {
				os.Exit(1)
			}
			return
		}
	}()

	logger.Info("starting worker",
		"exchange", config.Exchange,
		"queue", config.Queue,
		"routing_key", config.RoutingKey,
		"environment", config.AppEnv,
	)

	if config.UsesLegacyQueue() {
		logger.Warn("legacy webhook queue configuration detected",
			"queue", config.Queue,
			"routing_key", config.RoutingKey,
			"hint", "webhook.delivery.requested.v1 has no producer; set WEBHOOK_QUEUE=webhook.parcel.status_changed.v1 and WEBHOOK_ROUTING_KEY=parcel.status_changed.v1 to receive parcel status events",
		)
	}

	err = messaging.Run(ctx, messaging.ConsumerConfig{
		URL:            config.RabbitMQURL,
		Exchange:       config.Exchange,
		Queue:          config.Queue,
		RoutingKey:     config.RoutingKey,
		Prefetch:       config.Prefetch,
		HandlerTimeout: config.HandlerTimeout,
		MaxRetries:     config.MaxRetries,
		RetryDelay:     config.RetryDelay,
		ReconnectMin:   config.ReconnectMin,
		ReconnectMax:   config.ReconnectMax,
	}, worker.New(logger), logger)
	close(stopped)
	return err
}
