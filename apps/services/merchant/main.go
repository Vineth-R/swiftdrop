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
	"github.com/Vineth-R/swiftdrop/apps/services/merchant/internal/onboarding"
	"github.com/Vineth-R/swiftdrop/apps/services/merchant/internal/pricingclient"
	merchantrepository "github.com/Vineth-R/swiftdrop/apps/services/merchant/internal/repository"
	"github.com/Vineth-R/swiftdrop/packages/go-common/httpx"
	"github.com/Vineth-R/swiftdrop/packages/go-common/logging"
	"github.com/Vineth-R/swiftdrop/packages/go-common/postgres"
)

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
	var onboardingService *onboarding.Service
	if merchantRepo != nil {
		onboardingService, err = onboarding.New(merchantRepo, identityClient, pricingClient, mail.Notifier, logger)
		if err != nil {
			return fmt.Errorf("create onboarding service: %w", err)
		}
	}
	router := servicehttp.New(logger)
	servicehttp.Mount(router, servicehttp.Options{
		Repository: merchantRepo, Onboarding: onboardingService, Identity: identityClient,
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
