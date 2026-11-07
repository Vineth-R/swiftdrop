package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	serviceconfig "github.com/Vineth-R/swiftdrop/apps/services/parcel/internal/config"
	"github.com/Vineth-R/swiftdrop/apps/services/parcel/internal/dispatch"
	servicehttp "github.com/Vineth-R/swiftdrop/apps/services/parcel/internal/http"
	"github.com/Vineth-R/swiftdrop/apps/services/parcel/internal/identityclient"
	"github.com/Vineth-R/swiftdrop/apps/services/parcel/internal/outbox"
	"github.com/Vineth-R/swiftdrop/apps/services/parcel/internal/parcels"
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
		InternalToken:  config.InternalToken,
	}
	if config.AccessSecret == "" {
		logger.Warn("JWT_ACCESS_SECRET is not set; authenticated APIs will return 503")
	}
	if config.InternalTokenProblem != "" {
		logger.Warn(config.InternalTokenProblem)
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
		options.Tracking = tracking.NewRepository(database)
		options.Reports = reports.NewRepository(database)
		pricingClient := parcels.NewPricingClient(config.PricingBaseURL, config.InternalToken, &http.Client{
			Timeout: config.PricingTimeout,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		})
		options.Parcels = parcels.NewService(parcels.NewStore(database), pricingClient)
		options.Parcels.WithCourierReader(parcels.NewCourierClient(config.IdentityBaseURL, config.InternalToken, &http.Client{
			Timeout: config.IdentityTimeout,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}))

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
		relayOptions := outbox.DefaultRelayOptions()
		relayOptions.Exchange = config.RabbitMQExchange
		relay := outbox.NewRelay(
			outboxStore,
			publisher,
			logger,
			relayOptions,
		)
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
