package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	serviceconfig "github.com/Vineth-R/swiftdrop/apps/services/notification-worker/internal/config"
	servicehttp "github.com/Vineth-R/swiftdrop/apps/services/notification-worker/internal/http"
	"github.com/Vineth-R/swiftdrop/apps/services/notification-worker/internal/idempotency"
	"github.com/Vineth-R/swiftdrop/apps/services/notification-worker/internal/identityclient"
	"github.com/Vineth-R/swiftdrop/apps/services/notification-worker/internal/merchantclient"
	"github.com/Vineth-R/swiftdrop/apps/services/notification-worker/internal/notificationlog"
	"github.com/Vineth-R/swiftdrop/apps/services/notification-worker/internal/retention"
	"github.com/Vineth-R/swiftdrop/apps/services/notification-worker/internal/worker"
	"github.com/Vineth-R/swiftdrop/packages/go-common/emails"
	"github.com/Vineth-R/swiftdrop/packages/go-common/httpx"
	"github.com/Vineth-R/swiftdrop/packages/go-common/logging"
	"github.com/Vineth-R/swiftdrop/packages/go-common/mailer"
	"github.com/Vineth-R/swiftdrop/packages/go-common/messaging"
	"github.com/Vineth-R/swiftdrop/packages/go-common/metrics"
	"github.com/Vineth-R/swiftdrop/packages/go-common/postgres"
)

const serviceName = "notification-worker"

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

	if config.AppEnv != "development" {
		gin.SetMode(gin.ReleaseMode)
	}

	sender, err := mailer.NewSMTP(config.SMTP)
	if err != nil {
		return fmt.Errorf("create smtp mailer: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	databaseConfig, err := postgres.LoadConfigFromURL(config.DatabaseURL)
	if err != nil {
		return fmt.Errorf("configure database: %w", err)
	}
	database, err := postgres.Open(ctx, databaseConfig)
	if err != nil {
		return fmt.Errorf("connect database: %w", err)
	}
	defer database.Close()
	logger.Info("database connection established")

	store := idempotency.NewPostgres(database, idempotency.ConsumerName)

	identity, err := identityclient.New(config.IdentityBaseURL, config.InternalToken, config.IdentityTimeout)
	if err != nil {
		return fmt.Errorf("configure identity client: %w", err)
	}
	merchants, err := merchantclient.New(config.MerchantBaseURL, config.InternalToken, config.MerchantTimeout)
	if err != nil {
		return fmt.Errorf("configure merchant client: %w", err)
	}
	if config.InternalTokenProblem != "" {
		logger.Warn("internal contact lookups are not fully configured", "problem", config.InternalTokenProblem)
	}
	site := emails.Site{BaseURL: config.FrontendBaseURL, SupportEmail: config.SupportEmail}
	handler := worker.New(logger, sender, store, identity, merchants, site)

	router, err := servicehttp.New(logger, servicehttp.Options{
		Store:        notificationlog.NewRepository(database),
		AccessSecret: config.AccessSecret,
		CookieName:   config.CookieName,
	})
	if err != nil {
		return fmt.Errorf("configure notification admin router: %w", err)
	}
	registry := metrics.NewRegistry()
	router.GET("/metrics", metrics.Handler(registry))
	router.GET("/health", postgres.HealthHandler())
	router.GET("/ready", postgres.ReadyHandler(database, postgres.DefaultReadyTimeout))

	if config.AccessSecret == "" {
		logger.Warn("JWT_ACCESS_SECRET is not set; the admin notification-log API will return 503")
	}

	server := httpx.NewServer(httpx.ServerConfig{
		Addr:              config.Addr(),
		Handler:           metrics.NewHTTPHandler(registry, router),
		Logger:            logger,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	})

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
		"legacy_queue", config.Queue,
		"legacy_routing_key", config.RoutingKey,
		"parcel_created_queue", config.ParcelCreatedQueue,
		"parcel_created_routing_key", config.ParcelCreatedRoutingKey,
		"parcel_status_queue", config.ParcelStatusQueue,
		"parcel_status_routing_key", config.ParcelStatusRoutingKey,
		"environment", config.AppEnv,
	)
	logger.Info("starting http service",
		"listen_addr", config.Addr(),
		"environment", config.AppEnv,
	)
	logger.Info("processed-event retention enabled",
		"retention", config.ProcessedEventsRetention,
		"cleanup_interval", config.ProcessedEventsCleanupInterval,
	)

	cleanupDone := make(chan struct{})
	go func() {
		defer close(cleanupDone)
		retention.Run(ctx, logger, store, config.ProcessedEventsRetention, config.ProcessedEventsCleanupInterval)
	}()

	consumers := []messaging.ConsumerConfig{
		{
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
		},
		{
			URL:            config.RabbitMQURL,
			Exchange:       config.Exchange,
			Queue:          config.ParcelCreatedQueue,
			RoutingKey:     config.ParcelCreatedRoutingKey,
			Prefetch:       config.Prefetch,
			HandlerTimeout: config.HandlerTimeout,
			MaxRetries:     config.MaxRetries,
			RetryDelay:     config.RetryDelay,
			ReconnectMin:   config.ReconnectMin,
			ReconnectMax:   config.ReconnectMax,
		},
		{
			URL:            config.RabbitMQURL,
			Exchange:       config.Exchange,
			Queue:          config.ParcelStatusQueue,
			RoutingKey:     config.ParcelStatusRoutingKey,
			Prefetch:       config.Prefetch,
			HandlerTimeout: config.HandlerTimeout,
			MaxRetries:     config.MaxRetries,
			RetryDelay:     config.RetryDelay,
			ReconnectMin:   config.ReconnectMin,
			ReconnectMax:   config.ReconnectMax,
		},
	}

	results := make(chan error, len(consumers)+1)
	go func() {
		results <- httpx.Serve(ctx, server, 10*time.Second)
	}()
	for _, consumer := range consumers {
		go func(consumer messaging.ConsumerConfig) {
			results <- messaging.Run(ctx, consumer, handler, logger)
		}(consumer)
	}

	first := <-results
	cancel()
	firstErr := first
	for range consumers {
		if err := <-results; err != nil && firstErr == nil {
			firstErr = err
		}
	}
	close(stopped)
	<-cleanupDone
	return firstErr
}
