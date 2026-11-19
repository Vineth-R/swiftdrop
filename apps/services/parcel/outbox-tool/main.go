// Command outbox-tool reports, replays or expires failed parcel outbox events.
//
// Replaying returns FAILED rows to PENDING with a fresh attempt budget so the
// normal relay republishes them. Expiry deletes FAILED rows older than an
// explicit retention cutoff. Both operations are dry-run by default and require
// -apply, because they change delivery state or remove evidence.
//
// The tool is an operator utility, not a service. It reads DATABASE_URL_DIRECT
// (or DATABASE_URL) and never starts an HTTP listener or a broker consumer.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Vineth-R/swiftdrop/apps/services/parcel/internal/outbox"
	"github.com/Vineth-R/swiftdrop/packages/go-common/logging"
	"github.com/Vineth-R/swiftdrop/packages/go-common/postgres"
)

type report struct {
	Mode      string               `json:"mode"`
	Applied   bool                 `json:"applied"`
	OlderThan string               `json:"olderThan,omitempty"`
	Limit     int                  `json:"limit"`
	Summary   outbox.FailedSummary `json:"summary"`
	Affected  int64                `json:"affected"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "outbox-tool:", err)
		os.Exit(1)
	}
}

func run() error {
	replay := flag.Bool("replay", false, "return failed events to PENDING so the relay republishes them")
	expire := flag.Bool("expire", false, "delete failed events older than -older-than")
	apply := flag.Bool("apply", false, "perform the selected operation; without it the tool only reports")
	olderThan := flag.Duration("older-than", 30*24*time.Hour, "only events created before now minus this duration are eligible")
	limit := flag.Int("limit", 100, "maximum number of events changed in one run")
	reportPath := flag.String("report", "", "write the JSON report to this path")
	flag.Parse()

	if *replay && *expire {
		return errors.New("-replay and -expire cannot be combined")
	}
	if *olderThan < 0 {
		return errors.New("-older-than must not be negative")
	}
	if *limit <= 0 {
		return errors.New("-limit must be positive")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	databaseURL := os.Getenv("DATABASE_URL_DIRECT")
	if databaseURL == "" {
		databaseURL = os.Getenv("DATABASE_URL")
	}
	databaseConfig, err := postgres.LoadConfigFromURL(databaseURL)
	if err != nil {
		return fmt.Errorf("configure database: %w", err)
	}
	if !databaseConfig.Configured() {
		return errors.New("DATABASE_URL_DIRECT or DATABASE_URL is required")
	}
	pool, err := postgres.Open(ctx, databaseConfig)
	if err != nil {
		return fmt.Errorf("connect database: %w", err)
	}
	defer pool.Close()

	logger, err := logging.New("parcel-outbox-tool", os.Getenv("APP_ENV"), "info")
	if err != nil {
		return err
	}
	store := outbox.NewSQLStore(pool)
	summary, err := store.FailedSummary(ctx)
	if err != nil {
		return err
	}

	result := report{Mode: "summary", Limit: *limit, Summary: summary}
	cutoff := time.Now().UTC().Add(-*olderThan)
	if *replay || *expire {
		result.OlderThan = olderThan.String()
	}
	switch {
	case *replay:
		result.Mode = "replay"
		if *apply {
			affected, err := store.ReplayFailed(ctx, cutoff, *limit)
			if err != nil {
				return err
			}
			result.Affected = affected
			result.Applied = true
			logger.Info("failed outbox events replayed", "affected", affected, "older_than", olderThan.String(), "limit", *limit)
		}
	case *expire:
		result.Mode = "expire"
		if *apply {
			affected, err := store.DeleteFailed(ctx, cutoff, *limit)
			if err != nil {
				return err
			}
			result.Affected = affected
			result.Applied = true
			logger.Info("failed outbox events expired", "affected", affected, "older_than", olderThan.String(), "limit", *limit)
		}
	}

	if result.Applied {
		updated, err := store.FailedSummary(ctx)
		if err != nil {
			return err
		}
		result.Summary = updated
	}

	encoded, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(encoded))
	if *reportPath != "" {
		if err := os.WriteFile(*reportPath, append(encoded, '\n'), 0o600); err != nil {
			return fmt.Errorf("write report: %w", err)
		}
	}
	if (result.Mode == "replay" || result.Mode == "expire") && !result.Applied {
		logger.Warn("dry run: no events changed; pass -apply to perform the operation", "mode", result.Mode)
	}
	return nil
}
