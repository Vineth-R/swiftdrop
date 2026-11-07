package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	serviceconfig "github.com/Vineth-R/swiftdrop/apps/services/parcel/internal/config"
	"github.com/Vineth-R/swiftdrop/apps/services/parcel/internal/courier"
	"github.com/Vineth-R/swiftdrop/apps/services/parcel/internal/dispatch"
	servicehttp "github.com/Vineth-R/swiftdrop/apps/services/parcel/internal/http"
	"github.com/Vineth-R/swiftdrop/apps/services/parcel/internal/identityclient"
	"github.com/Vineth-R/swiftdrop/apps/services/parcel/internal/outbox"
	"github.com/Vineth-R/swiftdrop/apps/services/parcel/internal/reports"
	"github.com/Vineth-R/swiftdrop/apps/services/parcel/internal/tracking"
	"github.com/Vineth-R/swiftdrop/packages/go-common/httpx"
	"github.com/Vineth-R/swiftdrop/packages/go-common/logging"
	"github.com/Vineth-R/swiftdrop/packages/go-common/postgres"
)

const serviceName = "parcel"

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

	if config.AppEnv != "development" {
		gin.SetMode(gin.ReleaseMode)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var database *postgres.Pool
	var pinger postgres.Pinger
	if config.DatabaseURL != "" {
		databaseConfig := postgres.DefaultConfig()
		databaseConfig.URL = config.DatabaseURL

		database, err = postgres.Open(ctx, databaseConfig)
		if err != nil {
			return fmt.Errorf("connect database: %w", err)
		}
		defer database.Close()
		pinger = database
		logger.Info("database connection established")
	} else {
		logger.Warn("DATABASE_URL is not set; readiness will report database down")
	}
	options := servicehttp.Options{
		TrustedProxies: config.TrustedProxies,
		AccessSecret:   config.AccessSecret,
		CookieName:     config.CookieName,
	}
	if config.AccessSecret == "" {
		logger.Warn("JWT_ACCESS_SECRET is not set; the reports and courier delivery APIs will return 503")
	}
	var courierClient identityclient.CourierClient
	if config.IdentityBaseURL != "" {
		client, err := identityclient.New(config.IdentityBaseURL, config.InternalToken, config.IdentityTimeout)
		if err != nil {
			logger.Warn("identity client configuration error", "error", err)
		} else {
			courierClient = client
		}
	}

	if database != nil {
		options.Courier = courier.NewRepository(database)
		options.Tracking = tracking.NewRepository(database)
		options.Reports = reports.NewRepository(database)

		dispatchRepo := dispatch.NewRepository(database)
		dispatchService := dispatch.NewService(dispatchRepo, courierClient)
		options.Dispatch = dispatchService

		outboxStore := outbox.NewSQLStore(database)
		var publisher outbox.Publisher
		if config.RabbitMQURL != "" {
			amqpPub := outbox.NewAMQPPublisher(config.RabbitMQURL, config.RabbitMQExchange)
			defer amqpPub.Close()
			publisher = amqpPub
		} else {
			logger.Warn("RABBITMQ_URL is not set; outbox events will remain pending")
		}
		relay := outbox.NewRelay(outboxStore, publisher, logger)
		go relay.Run(ctx)
	}
	router, err := servicehttp.New(logger, options)
	if err != nil {
		return fmt.Errorf("configure parcel router: %w", err)
	}

	router.GET("/health", postgres.HealthHandler())
	router.GET("/ready", postgres.ReadyHandler(pinger, postgres.DefaultReadyTimeout))

	server := httpx.NewServer(httpx.ServerConfig{
		Addr:              config.Addr(),
		Handler:           router,
		Logger:            logger,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	})

	logger.Info("starting http service", "address", config.Addr(), "environment", config.AppEnv)
	return httpx.Serve(ctx, server, 10*time.Second)
}
