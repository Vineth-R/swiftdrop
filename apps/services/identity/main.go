package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	serviceconfig "github.com/Vineth-R/swiftdrop/apps/services/identity/internal/config"
	servicehttp "github.com/Vineth-R/swiftdrop/apps/services/identity/internal/http"
	"github.com/Vineth-R/swiftdrop/apps/services/identity/internal/repository"
	"github.com/Vineth-R/swiftdrop/packages/go-common/httpx"
	"github.com/Vineth-R/swiftdrop/packages/go-common/logging"
	"github.com/Vineth-R/swiftdrop/packages/go-common/metrics"
	"github.com/Vineth-R/swiftdrop/packages/go-common/postgres"
)

const serviceName = "identity"

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

	var routerOptions []servicehttp.Option
	var database *postgres.Pool
	var pinger postgres.Pinger
	var cleanupDone chan struct{}
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

		authOptions, drainEmail, err := authRouterOptions(logger, config, database)
		if err != nil {
			return err
		}
		defer drainEmail()
		routerOptions = append(routerOptions, authOptions...)

		identityRepo, err := repository.New(database)
		if err != nil {
			return fmt.Errorf("create identity repository for token cleanup: %w", err)
		}
		cleanupDone = make(chan struct{})
		go func() {
			defer close(cleanupDone)
			runTokenCleanup(ctx, identityRepo, logger, config.TokenRetention, config.TokenCleanupInterval)
		}()
	} else {
		logger.Warn("DATABASE_URL is not set; readiness will report database down and authentication is disabled")
	}

	router := servicehttp.New(logger, routerOptions...)
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
	serveErr := httpx.Serve(ctx, server, 10*time.Second)
	if cleanupDone != nil {
		<-cleanupDone
	}
	return serveErr
}

// runTokenCleanup deletes expired identity tokens at startup and then on the
// configured interval. The cutoff keeps expired rows for TokenRetention before
// deleting them, which preserves a short audit window without unbounded growth.
func runTokenCleanup(ctx context.Context, repo *repository.Repository, logger *slog.Logger, retention, interval time.Duration) {
	cleanup := func() {
		cutoff := time.Now().UTC().Add(-retention)
		deleted, err := repo.CleanupExpiredTokens(ctx, cutoff, repository.TokenCleanupBatch)
		if err != nil {
			logger.Warn("identity token cleanup failed", "error", err)
			return
		}
		if deleted > 0 {
			logger.Info("identity token cleanup completed", "deleted", deleted)
		}
	}

	cleanup()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cleanup()
		}
	}
}
