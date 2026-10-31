package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

var ErrKeyLimit = errors.New("merchant API key limit reached")

const MaxActiveAPIKeys = 5

type APIClient struct {
	ID         string
	MerchantID string
	Name       string
	KeyPrefix  string
	LookupID   string
	Status     string
	LastUsedAt *time.Time
	CreatedAt  time.Time
	RevokedAt  *time.Time
}

type CreateAPIClientParams struct {
	ID         string
	MerchantID string
	Name       string
	KeyPrefix  string
	LookupID   string
	KeyHash    string
}

func (r *Repository) CreateAPIClient(ctx context.Context, params CreateAPIClientParams) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin API key creation: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var merchantID string
	if err := tx.QueryRow(ctx, `SELECT id FROM merchant.merchants WHERE id = $1 FOR UPDATE`, params.MerchantID).Scan(&merchantID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("lock merchant for API key: %w", err)
	}
	var active int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM merchant.merchant_api_clients WHERE merchant_id = $1 AND status = 'ACTIVE'`, params.MerchantID).Scan(&active); err != nil {
		return fmt.Errorf("count active API keys: %w", err)
	}
	if active >= MaxActiveAPIKeys {
		return ErrKeyLimit
	}
	if _, err := tx.Exec(ctx, `INSERT INTO merchant.merchant_api_clients (id, merchant_id, name, key_prefix, lookup_id, key_hash) VALUES ($1, $2, $3, $4, $5, $6)`, params.ID, params.MerchantID, params.Name, params.KeyPrefix, params.LookupID, params.KeyHash); err != nil {
		return fmt.Errorf("create API key: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit API key creation: %w", err)
	}
	return nil
}

func (r *Repository) APIClientsByMerchant(ctx context.Context, merchantID string) ([]APIClient, error) {
	rows, err := r.db.Query(ctx, `SELECT id, merchant_id, name, key_prefix, lookup_id, status, last_used_at, created_at, revoked_at FROM merchant.merchant_api_clients WHERE merchant_id = $1 ORDER BY created_at DESC`, merchantID)
	if err != nil {
		return nil, fmt.Errorf("list API keys: %w", err)
	}
	defer rows.Close()
	clients := make([]APIClient, 0)
	for rows.Next() {
		var client APIClient
		if err := rows.Scan(&client.ID, &client.MerchantID, &client.Name, &client.KeyPrefix, &client.LookupID, &client.Status, &client.LastUsedAt, &client.CreatedAt, &client.RevokedAt); err != nil {
			return nil, fmt.Errorf("scan API key: %w", err)
		}
		clients = append(clients, client)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate API keys: %w", err)
	}
	return clients, nil
}

func (r *Repository) RevokeAPIClient(ctx context.Context, merchantID, clientID string) error {
	result, err := r.db.Exec(ctx, `UPDATE merchant.merchant_api_clients SET status = 'REVOKED', revoked_at = now() WHERE id = $1 AND merchant_id = $2 AND status = 'ACTIVE'`, clientID, merchantID)
	if err != nil {
		return fmt.Errorf("revoke API key: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) APIClientByLookup(ctx context.Context, lookupID string) (APIClient, string, error) {
	var client APIClient
	var hash string
	err := r.db.QueryRow(ctx, `SELECT id, merchant_id, name, key_prefix, lookup_id, key_hash, status, last_used_at, created_at, revoked_at FROM merchant.merchant_api_clients WHERE lookup_id = $1`, lookupID).Scan(&client.ID, &client.MerchantID, &client.Name, &client.KeyPrefix, &client.LookupID, &hash, &client.Status, &client.LastUsedAt, &client.CreatedAt, &client.RevokedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return APIClient{}, "", ErrNotFound
	}
	if err != nil {
		return APIClient{}, "", fmt.Errorf("lookup API key: %w", err)
	}
	return client, hash, nil
}

func (r *Repository) TouchAPIClient(ctx context.Context, clientID string) error {
	_, err := r.db.Exec(ctx, `UPDATE merchant.merchant_api_clients SET last_used_at = now() WHERE id = $1 AND status = 'ACTIVE'`, clientID)
	return err
}
