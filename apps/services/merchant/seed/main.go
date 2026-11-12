package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/Vineth-R/swiftdrop/apps/services/merchant/internal/identityclient"
	merchantrepository "github.com/Vineth-R/swiftdrop/apps/services/merchant/internal/repository"
	"github.com/Vineth-R/swiftdrop/apps/services/merchant/internal/seed"
	"github.com/Vineth-R/swiftdrop/packages/go-common/config"
	"github.com/Vineth-R/swiftdrop/packages/go-common/logging"
	"github.com/Vineth-R/swiftdrop/packages/go-common/postgres"
)

const serviceName = "merchant-seed"

func main() {
	if err := run(); err != nil {
		// The bootstrap logger intentionally redacts arbitrary error values.
		// Pass a plain, safe reason string instead so operators still see an
		// actionable failure; the seed never puts secrets in its errors.
		logging.NewBootstrap(serviceName).Error("merchant demo seed failed", "reason", err.Error())
		os.Exit(1)
	}
}

func run() error {
	loader := config.Load()

	seedConfig, err := seed.LoadConfig(loader)
	if err != nil {
		return err
	}

	logLevel, err := loader.EnumFold("LOG_LEVEL", "info", "debug", "info", "warn", "warning", "error")
	if err != nil {
		return err
	}

	databaseConfig, err := postgres.LoadConfigFromURL(seedConfig.DatabaseURL)
	if err != nil {
		return fmt.Errorf("database configuration: %w", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	database, err := postgres.Open(ctx, databaseConfig)
	if err != nil {
		return fmt.Errorf("connect database: %w", err)
	}
	defer database.Close()

	repository, err := merchantrepository.New(database)
	if err != nil {
		return fmt.Errorf("create merchant repository: %w", err)
	}

	identity, err := identityclient.New(seedConfig.IdentityBaseURL, seedConfig.InternalToken, seedConfig.IdentityTimeout)
	if err != nil {
		return fmt.Errorf("create identity client: %w", err)
	}

	result, err := seed.Run(ctx, repository, identity, seed.NewPostgresZoneSource(database), seedConfig)
	if err != nil {
		return err
	}

	logger, err := logging.New(serviceName, seedConfig.AppEnv, logLevel)
	if err != nil {
		return err
	}
	if result.AlreadySeeded {
		logger.Info("merchant demo seed already applied", "merchant_id", result.Merchant.ID, "status", result.Merchant.Status, "demo_key_created", result.APIKeyCreated, "demo_key_prefix", result.APIKeyPrefix)
		return nil
	}
	logger.Info("merchant demo seed complete", "merchant_id", result.Merchant.ID, "status", result.Merchant.Status, "demo_key_created", result.APIKeyCreated, "demo_key_prefix", result.APIKeyPrefix)
	return nil
}
