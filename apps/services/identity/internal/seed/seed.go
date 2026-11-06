package seed

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/Vineth-R/swiftdrop/apps/services/identity/internal/repository"
	"github.com/Vineth-R/swiftdrop/apps/services/identity/internal/service"
	"github.com/Vineth-R/swiftdrop/packages/go-common/auth"
)

// DB is the subset of the shared PostgreSQL pool the seed orchestration needs.
// *postgres.Pool satisfies it.
type DB interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

// Hasher hashes a demo password. *password.Hasher satisfies it.
type Hasher interface {
	Hash(plain string) (string, error)
}

// Run validates the configuration, hashes all passwords, resolves courier
// zones, and inserts the five demo users in one transaction. The caller owns
// the database connection; no transaction is committed unless every insert
// succeeds.
func Run(ctx context.Context, db DB, hasher Hasher, config Config) (repository.SeedResult, error) {
	if db == nil {
		return repository.SeedResult{}, errors.New("demo seed requires a database")
	}
	if hasher == nil {
		return repository.SeedResult{}, errors.New("demo seed requires a password hasher")
	}
	if err := config.Validate(); err != nil {
		return repository.SeedResult{}, err
	}

	hashes := make([]string, len(config.Accounts))
	for index, account := range config.Accounts {
		hash, err := hasher.Hash(account.Password)
		if err != nil {
			return repository.SeedResult{}, fmt.Errorf("hash demo account %d password: %w", index+1, err)
		}
		hashes[index] = hash
	}

	tx, err := db.Begin(ctx)
	if err != nil {
		return repository.SeedResult{}, fmt.Errorf("begin demo seed transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	zoneIDs := map[string]string{}
	for _, account := range config.Accounts {
		code := strings.TrimSpace(account.HomeZoneCode)
		if code == "" {
			continue
		}
		if _, exists := zoneIDs[code]; exists {
			continue
		}
		var zoneID string
		var active bool
		err := tx.QueryRow(ctx,
			`SELECT id::text, is_active FROM pricing.zones WHERE code = $1`,
			code,
		).Scan(&zoneID, &active)
		if errors.Is(err, pgx.ErrNoRows) {
			return repository.SeedResult{}, fmt.Errorf("courier zone %s is not configured", code)
		}
		if err != nil {
			return repository.SeedResult{}, fmt.Errorf("read courier zone %s: %w", code, err)
		}
		if !active {
			return repository.SeedResult{}, fmt.Errorf("courier zone %s is not active", code)
		}
		zoneIDs[code] = zoneID
	}

	seedUsers := make([]repository.SeedUser, 0, len(config.Accounts))
	for index, account := range config.Accounts {
		user := repository.SeedUser{
			Email:        service.NormalizeEmail(account.Email),
			PasswordHash: hashes[index],
			FullName:     strings.TrimSpace(account.FullName),
			Role:         account.Role,
		}
		if account.Role == auth.RoleCourier {
			zoneID, ok := zoneIDs[strings.TrimSpace(account.HomeZoneCode)]
			if !ok {
				return repository.SeedResult{}, fmt.Errorf("courier zone %s is not resolved", account.HomeZoneCode)
			}
			user.Courier = &repository.CourierProfileInput{
				VehicleType:   strings.TrimSpace(account.VehicleType),
				VehicleNumber: strings.TrimSpace(account.VehicleNumber),
				HomeZoneID:    zoneID,
			}
		}
		seedUsers = append(seedUsers, user)
	}

	repo, err := repository.New(tx)
	if err != nil {
		return repository.SeedResult{}, fmt.Errorf("create seed repository: %w", err)
	}
	result, err := repo.SeedDemoUsers(ctx, seedUsers)
	if err != nil {
		return repository.SeedResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return repository.SeedResult{}, fmt.Errorf("commit demo seed transaction: %w", err)
	}
	return result, nil
}
