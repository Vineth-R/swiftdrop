package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/Vineth-R/swiftdrop/packages/go-common/auth"
)

// SeedUser is one demo account inserted by the local seed command. Courier is
// set only for COURIER accounts.
type SeedUser struct {
	Email        string
	PasswordHash string
	FullName     string
	Role         auth.Role
	Courier      *CourierProfileInput
}

// SeedResult reports what a seed transaction created or skipped.
type SeedResult struct {
	Created int
	Skipped int
}

// SeedDemoUsers inserts demo accounts using the transaction-bound database
// connection. The caller owns the transaction: this method never begins or
// commits one. Existing emails are skipped and never modified.
func (r *Repository) SeedDemoUsers(ctx context.Context, users []SeedUser) (SeedResult, error) {
	if r == nil || r.db == nil {
		return SeedResult{}, errors.New("seed demo users requires a database")
	}

	roles := make([]auth.Role, 0, len(users))
	seenRoles := map[auth.Role]bool{}
	for _, user := range users {
		if !seenRoles[user.Role] {
			seenRoles[user.Role] = true
			roles = append(roles, user.Role)
		}
	}
	for _, role := range roles {
		var name string
		err := r.db.QueryRow(ctx, `SELECT name FROM identity.roles WHERE name = $1`, string(role)).Scan(&name)
		if errors.Is(err, pgx.ErrNoRows) {
			return SeedResult{}, fmt.Errorf("seed demo users: role %s is not seeded", role)
		}
		if err != nil {
			return SeedResult{}, fmt.Errorf("seed demo users: read role %s: %w", role, err)
		}
	}

	result := SeedResult{}
	for _, user := range users {
		var userID string
		err := r.db.QueryRow(ctx, `
			INSERT INTO identity.users
				(email, password_hash, full_name, phone, role_id, auth_provider, email_verified)
			SELECT $1, $2, $3, NULL, ro.id, 'LOCAL', true
			FROM identity.roles ro
			WHERE ro.name = $4
			ON CONFLICT DO NOTHING
			RETURNING id::text`,
			user.Email, user.PasswordHash, user.FullName, string(user.Role),
		).Scan(&userID)
		if errors.Is(err, pgx.ErrNoRows) {
			result.Skipped++
			continue
		}
		if err != nil {
			return SeedResult{}, fmt.Errorf("seed demo users: insert %s: %w", user.Role, err)
		}
		result.Created++

		if user.Courier == nil {
			continue
		}
		if _, err := r.db.Exec(ctx, `
			INSERT INTO identity.courier_profiles
				(user_id, vehicle_type, vehicle_number, home_zone_id)
			VALUES ($1, $2, $3, $4::uuid)
			ON CONFLICT (user_id) DO NOTHING`,
			userID, user.Courier.VehicleType, user.Courier.VehicleNumber, user.Courier.HomeZoneID,
		); err != nil {
			return SeedResult{}, fmt.Errorf("seed demo users: insert %s profile: %w", user.Role, err)
		}
	}

	return result, nil
}
