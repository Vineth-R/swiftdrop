package main

import (
	"context"
	"log/slog"
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
		slog.New(slog.NewJSONHandler(os.Stderr, nil)).Error("worker stopped", "service", serviceName, "error", err)
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

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	logger.Info("starting worker",
		"exchange", config.Exchange,
		"queue", config.Queue,
		"routing_key", config.RoutingKey,
		"environment", config.AppEnv,
	)

	return messaging.Run(ctx, messaging.ConsumerConfig{
		URL:            config.RabbitMQURL,
		Exchange:       config.Exchange,
		Queue:          config.Queue,
		RoutingKey:     config.RoutingKey,
		Prefetch:       config.Prefetch,
		HandlerTimeout: config.HandlerTimeout,
		ReconnectMin:   config.ReconnectMin,
		ReconnectMax:   config.ReconnectMax,
	}, worker.New(logger), logger)
}
