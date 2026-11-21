package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// Token cleanup bounds. The cleanup loops until no eligible rows remain or the
// per-run iteration cap is reached, so a large backlog cannot create one long
// transaction.
const (
	TokenCleanupBatch         = 1000
	TokenCleanupMaxIterations = 100
)

// CleanupExpiredTokens deletes expired refresh tokens and expired or used
// password-reset and email-verification tokens. It never deletes an unexpired,
// unused token, so active sessions and pending links continue to work. It is
// safe to run repeatedly and from more than one replica: each statement deletes
// an independent bounded set.
func (r *Repository) CleanupExpiredTokens(ctx context.Context, now time.Time, batch int) (int64, error) {
	if r == nil || r.db == nil {
		return 0, errors.New("token cleanup requires a database")
	}
	if batch < 1 || batch > 10000 {
		batch = TokenCleanupBatch
	}

	var total int64
	for iteration := 0; iteration < TokenCleanupMaxIterations; iteration++ {
		tx, err := r.db.Begin(ctx)
		if err != nil {
			return total, fmt.Errorf("begin token cleanup: %w", err)
		}
		deleted, err := cleanupTokensTx(ctx, tx, now, batch)
		if err != nil {
			_ = tx.Rollback(ctx)
			return total, err
		}
		if err := tx.Commit(ctx); err != nil {
			return total, fmt.Errorf("commit token cleanup: %w", err)
		}
		total += deleted
		if deleted == 0 {
			return total, nil
		}
	}
	return total, nil
}

func cleanupTokensTx(ctx context.Context, tx pgx.Tx, now time.Time, batch int) (int64, error) {
	queries := []string{
		`DELETE FROM identity.refresh_tokens
		 WHERE id IN (
		   SELECT id FROM identity.refresh_tokens
		   WHERE expires_at < $1
		   ORDER BY expires_at
		   LIMIT $2
		 )`,
		`DELETE FROM identity.password_reset_tokens
		 WHERE id IN (
		   SELECT id FROM identity.password_reset_tokens
		   WHERE expires_at < $1 OR (used_at IS NOT NULL AND used_at < $1)
		   ORDER BY expires_at
		   LIMIT $2
		 )`,
		`DELETE FROM identity.email_verification_tokens
		 WHERE id IN (
		   SELECT id FROM identity.email_verification_tokens
		   WHERE expires_at < $1 OR (used_at IS NOT NULL AND used_at < $1)
		   ORDER BY expires_at
		   LIMIT $2
		 )`,
	}
	var total int64
	for _, query := range queries {
		tag, err := tx.Exec(ctx, query, now.UTC(), batch)
		if err != nil {
			return total, fmt.Errorf("delete expired identity tokens: %w", err)
		}
		total += tag.RowsAffected()
	}
	return total, nil
}
