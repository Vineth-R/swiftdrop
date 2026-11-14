package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	serviceconfig "github.com/Vineth-R/swiftdrop/apps/services/merchant/internal/config"
	servicehttp "github.com/Vineth-R/swiftdrop/apps/services/merchant/internal/http"
	"github.com/Vineth-R/swiftdrop/apps/services/merchant/internal/identityclient"
	"github.com/Vineth-R/swiftdrop/apps/services/merchant/internal/merchantsync"
	"github.com/Vineth-R/swiftdrop/apps/services/merchant/internal/onboarding"
	"github.com/Vineth-R/swiftdrop/apps/services/merchant/internal/orders"
	"github.com/Vineth-R/swiftdrop/apps/services/merchant/internal/parcelclient"
	"github.com/Vineth-R/swiftdrop/apps/services/merchant/internal/pricingclient"
	merchantrepository "github.com/Vineth-R/swiftdrop/apps/services/merchant/internal/repository"
	"github.com/Vineth-R/swiftdrop/packages/go-common/httpx"
	"github.com/Vineth-R/swiftdrop/packages/go-common/logging"
	"github.com/Vineth-R/swiftdrop/packages/go-common/messaging"
	"github.com/Vineth-R/swiftdrop/packages/go-common/metrics"
	"github.com/Vineth-R/swiftdrop/packages/go-common/postgres"
)

type zoneNameLookup struct{ client *pricingclient.Client }

func (z zoneNameLookup) ZoneName(ctx context.Context, zoneID string) (string, error) {
	zone, err := z.client.Zone(ctx, zoneID)
	if err != nil {
		return "", err
	}
	return zone.Name, nil
}

const serviceName = "merchant"

func main() {
	if err := run(); err != nil {
		logging.NewBootstrap(serviceName).Error("service stopped", "error", err)
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
	if err := config.ValidateRuntime(); err != nil {
		return err
	}

	if config.AppEnv != "development" {
		gin.SetMode(gin.ReleaseMode)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var database *postgres.Pool
	var pinger postgres.Pinger
	var merchantRepo *merchantrepository.Repository
	databaseConfig, configErr := postgres.LoadConfigFromURL(config.DatabaseURL)
	if configErr != nil {
		return fmt.Errorf("configure database: %w", configErr)
	}
	if databaseConfig.Configured() {
		database, err = postgres.Open(ctx, databaseConfig)
		if err != nil {
			return fmt.Errorf("connect database: %w", err)
		}
		defer database.Close()
		pinger = database
		merchantRepo, err = merchantrepository.New(database)
		if err != nil {
			return fmt.Errorf("create merchant repository: %w", err)
		}
		logger.Info("database connection established")
	} else {
		logger.Warn("DATABASE_URL is not set; readiness will report database down")
	}
	mail, err := setUpEmail(config, logger)
	if err != nil {
		return err
	}
	defer mail.Wait()
	identityClient, err := identityclient.New(config.IdentityBaseURL, config.InternalToken, config.IdentityTimeout)
	if err != nil {
		return fmt.Errorf("create identity client: %w", err)
	}
	pricingClient, err := pricingclient.New(config.PricingBaseURL, config.InternalToken, config.PricingTimeout)
	if err != nil {
		return fmt.Errorf("create pricing client: %w", err)
	}
	parcelClient, err := parcelclient.New(config.ParcelBaseURL, config.InternalToken, config.ParcelTimeout)
	if err != nil {
		return fmt.Errorf("create parcel client: %w", err)
	}
	var onboardingService *onboarding.Service
	var orderService *orders.Service
	if merchantRepo != nil {
		onboardingService, err = onboarding.New(merchantRepo, identityClient, pricingClient, mail.Notifier, logger)
		if err != nil {
			return fmt.Errorf("create onboarding service: %w", err)
		}
		orderService = orders.New(merchantRepo, parcelClient, zoneNameLookup{client: pricingClient})
	}
	router := servicehttp.New(logger)
	registry := metrics.NewRegistry()
	router.GET("/metrics", metrics.Handler(registry))
	servicehttp.Mount(router, servicehttp.Options{
		Repository: merchantRepo, Onboarding: onboardingService, Identity: identityClient,
		Orders:       orderService,
		Notifier:     mail.Notifier,
		AccessSecret: config.AccessSecret, CookieName: config.CookieName,
		InternalToken: config.InternalToken, Logger: logger,
	})
	if merchantRepo != nil {
		if err := registerWebhooks(router, merchantRepo, identityClient, config, logger); err != nil {
			return err
		}
	}
	// The internal routes are always registered so a missing repository fails
	// closed with 503 instead of looking like an unknown path. Authentication is
	// still checked first.
	if err := registerInternalWebhooks(router, merchantRepo, config, logger); err != nil {
		return err
	}

	router.GET("/health", postgres.HealthHandler())
	router.GET("/ready", postgres.ReadyHandler(pinger, postgres.DefaultReadyTimeout))

	server := httpx.NewServer(httpx.ServerConfig{
		Addr:              config.Addr(),
		Handler:           metrics.NewHTTPHandler(registry, router),
		Logger:            logger,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	})

	logger.Info("starting http service", "listen_addr", config.Addr(), "environment", config.AppEnv)
	runSync := config.SyncEnabled() && merchantRepo != nil
	switch {
	case !config.SyncEnabled():
		logger.Warn("RABBITMQ_URL is not set; merchant order status synchronization is disabled")
	case merchantRepo == nil:
		logger.Warn("DATABASE_URL is not set; merchant order status synchronization is disabled")
	}
	if !runSync {
		return httpx.Serve(ctx, server, 10*time.Second)
	}

	logger.Info("starting merchant status consumer",
		"exchange", config.RabbitMQExchange,
		"queue", config.MerchantSyncQueue,
		"routing_key", config.MerchantSyncRoutingKey,
	)
	consumer := messaging.ConsumerConfig{
		URL:            config.RabbitMQURL,
		Exchange:       config.RabbitMQExchange,
		Queue:          config.MerchantSyncQueue,
		RoutingKey:     config.MerchantSyncRoutingKey,
		Prefetch:       config.RabbitMQPrefetch,
		HandlerTimeout: config.RabbitMQHandlerTimeout,
		MaxRetries:     config.RabbitMQMaxRetries,
		RetryDelay:     config.RabbitMQRetryDelay,
		ReconnectMin:   config.RabbitMQReconnectMin,
		ReconnectMax:   config.RabbitMQReconnectMax,
	}
	handler := merchantsync.New(merchantRepo, logger)

	results := make(chan error, 2)
	go func() { results <- httpx.Serve(ctx, server, 10*time.Second) }()
	go func() { results <- messaging.Run(ctx, consumer, handler, logger) }()

	first := <-results
	stop()
	second := <-results
	if first != nil {
		return first
	}
	return second
}
