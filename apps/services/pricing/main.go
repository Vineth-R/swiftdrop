package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	serviceconfig "github.com/Vineth-R/swiftdrop/apps/services/pricing/internal/config"
	servicehttp "github.com/Vineth-R/swiftdrop/apps/services/pricing/internal/http"
	"github.com/Vineth-R/swiftdrop/apps/services/pricing/internal/pricing"
	"github.com/Vineth-R/swiftdrop/packages/go-common/httpx"
	"github.com/Vineth-R/swiftdrop/packages/go-common/logging"
	"github.com/Vineth-R/swiftdrop/packages/go-common/metrics"
	"github.com/Vineth-R/swiftdrop/packages/go-common/postgres"
	"github.com/Vineth-R/swiftdrop/packages/go-common/sessioncheck"
)

const serviceName = "pricing"

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
		logger.Info("database connection established")
	} else {
		logger.Warn("DATABASE_URL is not set; readiness will report database down")
	}
	sessionClient, err := sessioncheck.NewClient(config.IdentityBaseURL, config.InternalToken, config.IdentityTimeout)
	if err != nil {
		return fmt.Errorf("configure identity session checker: %w", err)
	}
	options := servicehttp.Options{InternalToken: config.InternalToken, AccessSecret: config.AccessSecret,
		CookieName: config.CookieName, TrustedProxies: config.TrustedProxies,
		SessionCheck: sessioncheck.New(sessionClient)}
	if database != nil {
		options.Store = pricing.NewRepository(database)
	}
	router, err := servicehttp.New(logger, options)
	if err != nil {
		return fmt.Errorf("configure pricing router: %w", err)
	}
	registry := metrics.NewRegistry()
	router.GET("/metrics", metrics.Handler(registry))

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
	return httpx.Serve(ctx, server, 10*time.Second)
}
