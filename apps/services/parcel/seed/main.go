package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/Vineth-R/swiftdrop/apps/services/parcel/internal/seed"
	"github.com/Vineth-R/swiftdrop/packages/go-common/config"
	"github.com/Vineth-R/swiftdrop/packages/go-common/logging"
	"github.com/Vineth-R/swiftdrop/packages/go-common/postgres"
)

const serviceName = "parcel-seed"

func main() {
	if err := run(); err != nil {
		logging.NewBootstrap(serviceName).Error("parcel demo seed failed", "reason", err.Error())
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

	result, err := seed.Run(ctx, database, seedConfig)
	if err != nil {
		return err
	}

	logger, err := logging.New(serviceName, seedConfig.AppEnv, logLevel)
	if err != nil {
		return err
	}
	logger.Info("demo parcels seeded", "created", result.Created, "skipped", result.Skipped, "statuses", result.Statuses)
	return nil
}
