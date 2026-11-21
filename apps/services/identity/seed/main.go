package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/Vineth-R/swiftdrop/apps/services/identity/internal/password"
	"github.com/Vineth-R/swiftdrop/apps/services/identity/internal/seed"
	"github.com/Vineth-R/swiftdrop/packages/go-common/config"
	"github.com/Vineth-R/swiftdrop/packages/go-common/logging"
	"github.com/Vineth-R/swiftdrop/packages/go-common/postgres"
)

const serviceName = "identity-seed"

func main() {
	if err := run(); err != nil {
		logging.NewBootstrap(serviceName).Error("demo seed failed", "error", err)
		os.Exit(1)
	}
}

func run() error {
	loader := config.Load()

	seedConfig, err := seed.LoadConfig(loader)
	if err != nil {
		return err
	}

	// This local operator CLI is outside the fail-closed service/worker set.
	// The development default only controls logging; it does not make the target
	// database safe. Verify DATABASE_URL before running this command.
	appEnv, err := loader.EnumFold("APP_ENV", "development", "development", "staging", "production")
	if err != nil {
		return err
	}
	logLevel, err := loader.EnumFold("LOG_LEVEL", "info", "debug", "info", "warn", "warning", "error")
	if err != nil {
		return err
	}

	databaseURL, err := loader.Required("DATABASE_URL")
	if err != nil {
		return err
	}
	allowRemote := strings.EqualFold(loader.String("ALLOW_REMOTE_SEED", ""), "1") ||
		strings.EqualFold(loader.String("ALLOW_REMOTE_SEED", ""), "true")
	if err := seed.ValidateTarget(appEnv, databaseURL, allowRemote); err != nil {
		return err
	}

	databaseConfig, err := postgres.LoadConfig(loader)
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

	hasher, err := password.NewHasher(password.DefaultCost)
	if err != nil {
		return fmt.Errorf("create password hasher: %w", err)
	}

	result, err := seed.Run(ctx, database, hasher, seedConfig)
	if err != nil {
		return err
	}

	logger, err := logging.New(serviceName, appEnv, logLevel)
	if err != nil {
		return err
	}
	logger.Info("demo users seeded", "created", result.Created, "skipped", result.Skipped)
	return nil
}
