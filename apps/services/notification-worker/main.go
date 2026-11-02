package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	serviceconfig "github.com/Vineth-R/swiftdrop/apps/services/notification-worker/internal/config"
	"github.com/Vineth-R/swiftdrop/apps/services/notification-worker/internal/worker"
	"github.com/Vineth-R/swiftdrop/packages/go-common/logging"
	"github.com/Vineth-R/swiftdrop/packages/go-common/mailer"
	"github.com/Vineth-R/swiftdrop/packages/go-common/messaging"
)

const serviceName = "notification-worker"

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

	sender, err := mailer.NewSMTP(config.SMTP)
	if err != nil {
		return fmt.Errorf("create smtp mailer: %w", err)
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
	}, worker.New(logger, sender), logger)
	close(stopped)
	return err
}
