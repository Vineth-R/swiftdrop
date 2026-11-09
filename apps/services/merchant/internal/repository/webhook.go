package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// ErrWebhookNotConfigured means the merchant has not saved a webhook yet.
var ErrWebhookNotConfigured = errors.New("webhook not configured")

const (
	webhooksTable = "merchant.merchant_webhooks"
	attemptsTable = "merchant.webhook_delivery_attempts"
)

// Webhook is one merchant's webhook configuration (one row per merchant).
type Webhook struct {
	ID         string
	MerchantID string
	URL        string
	// SigningSecretEnc holds the UTF-8 bytes of the secretbox text
	// ("v1:..."). Convert with string(...) before calling Decrypt.
	SigningSecretEnc []byte
	EventTypes       []string
	IsActive         bool
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// SaveWebhookParams creates or replaces the merchant's webhook.
type SaveWebhookParams struct {
	ID         string // used only when the row is created
	MerchantID string
	URL        string
	EventTypes []string
	IsActive   bool
	// RotateSecret asks for CandidateSecretEnc to replace the stored secret.
	RotateSecret bool
	// CandidateSecretEnc is the encrypted secret to store when the row is
	// created or rotated. It is ignored when an existing secret is kept.
	CandidateSecretEnc []byte
}

// WebhookSaveResult reports the persisted configuration and whether the
// caller's candidate secret is the one now stored. A caller must return a
// plaintext signing secret only when SecretStored is true.
type WebhookSaveResult struct {
	Webhook      Webhook
	SecretStored bool
}

const webhookColumns = `id::text, merchant_id::text, url, signing_secret_enc, event_types, is_active, created_at, updated_at`

func scanWebhook(row pgx.Row) (Webhook, error) {
	var w Webhook
	err := row.Scan(&w.ID, &w.MerchantID, &w.URL, &w.SigningSecretEnc, &w.EventTypes, &w.IsActive, &w.CreatedAt, &w.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Webhook{}, ErrWebhookNotConfigured
	}
	return w, err
}

// WebhookByMerchant returns the merchant's webhook or ErrWebhookNotConfigured.
func (r *Repository) WebhookByMerchant(ctx context.Context, merchantID string) (Webhook, error) {
	const query = `SELECT ` + webhookColumns + ` FROM ` + webhooksTable + ` WHERE merchant_id = $1::uuid`
	return scanWebhook(r.db.QueryRow(ctx, query, merchantID))
}

// SaveWebhook creates or replaces the merchant's webhook. Concurrent saves for
// the same merchant are serialised with a row lock on merchant.merchants, so
// first-time creation is atomic and the stored secret is always the one the
// caller is told about:
//
//   - the first save inserts the candidate secret and reports SecretStored;
//   - a concurrent first save sees the row created by the winner, keeps the
//     winner's secret, and reports SecretStored=false, so it never returns a
//     secret that was not persisted;
//   - a later save keeps the existing secret unless RotateSecret is true;
//   - rotation writes the candidate secret before returning it.
//
// The caller must pass CandidateSecretEnc for insert and rotation. The
// transaction commits before the result is returned, so a database failure
// never reports a secret as saved.
func (r *Repository) SaveWebhook(ctx context.Context, p SaveWebhookParams) (WebhookSaveResult, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return WebhookSaveResult{}, fmt.Errorf("begin webhook save: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// The merchant row serialises every webhook save for this merchant, not
	// just this process. It also proves the merchant still exists.
	var lockedMerchantID string
	if err := tx.QueryRow(ctx,
		`SELECT id FROM merchant.merchants WHERE id = $1::uuid FOR UPDATE`,
		p.MerchantID,
	).Scan(&lockedMerchantID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return WebhookSaveResult{}, ErrNotFound
		}
		return WebhookSaveResult{}, fmt.Errorf("lock merchant for webhook save: %w", err)
	}

	_, err = scanWebhook(tx.QueryRow(ctx,
		`SELECT `+webhookColumns+` FROM `+webhooksTable+` WHERE merchant_id = $1::uuid FOR UPDATE`,
		p.MerchantID,
	))
	switch {
	case errors.Is(err, ErrWebhookNotConfigured):
		if len(p.CandidateSecretEnc) == 0 {
			return WebhookSaveResult{}, errors.New("webhook creation requires an encrypted secret")
		}
		saved, err := scanWebhook(tx.QueryRow(ctx, `
INSERT INTO `+webhooksTable+`
    (id, merchant_id, url, signing_secret_enc, event_types, is_active, created_at, updated_at)
VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6, now(), now())
RETURNING `+webhookColumns,
			p.ID, p.MerchantID, p.URL, p.CandidateSecretEnc, p.EventTypes, p.IsActive))
		if err != nil {
			return WebhookSaveResult{}, fmt.Errorf("insert webhook: %w", err)
		}
		if err := tx.Commit(ctx); err != nil {
			return WebhookSaveResult{}, fmt.Errorf("commit webhook insert: %w", err)
		}
		return WebhookSaveResult{Webhook: saved, SecretStored: true}, nil
	case err != nil:
		return WebhookSaveResult{}, fmt.Errorf("load webhook for save: %w", err)
	}

	// The row already exists. Keep its secret unless rotation was requested,
	// even if the caller believed this was a first save.
	if p.RotateSecret {
		if len(p.CandidateSecretEnc) == 0 {
			return WebhookSaveResult{}, errors.New("webhook rotation requires an encrypted secret")
		}
		saved, err := scanWebhook(tx.QueryRow(ctx, `
UPDATE `+webhooksTable+` SET
    url = $2,
    event_types = $3,
    is_active = $4,
    signing_secret_enc = $5,
    updated_at = now()
WHERE merchant_id = $1::uuid
RETURNING `+webhookColumns, p.MerchantID, p.URL, p.EventTypes, p.IsActive, p.CandidateSecretEnc))
		if err != nil {
			return WebhookSaveResult{}, fmt.Errorf("rotate webhook: %w", err)
		}
		if err := tx.Commit(ctx); err != nil {
			return WebhookSaveResult{}, fmt.Errorf("commit webhook rotation: %w", err)
		}
		return WebhookSaveResult{Webhook: saved, SecretStored: true}, nil
	}

	saved, err := scanWebhook(tx.QueryRow(ctx, `
UPDATE `+webhooksTable+` SET
    url = $2,
    event_types = $3,
    is_active = $4,
    updated_at = now()
WHERE merchant_id = $1::uuid
RETURNING `+webhookColumns, p.MerchantID, p.URL, p.EventTypes, p.IsActive))
	if err != nil {
		return WebhookSaveResult{}, fmt.Errorf("update webhook: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return WebhookSaveResult{}, fmt.Errorf("commit webhook update: %w", err)
	}
	return WebhookSaveResult{Webhook: saved, SecretStored: false}, nil
}

// DeliveryFilter narrows the delivery log (API 8.15).
type DeliveryFilter struct {
	Success        *bool  // nil means both
	HTTPStatus     *int   // nil means every status
	TrackingNumber string // empty means all
	Limit          int
	Offset         int
}

// DeliveryAttempt is one row of the delivery log.
type DeliveryAttempt struct {
	EventID        string
	TrackingNumber string
	AttemptNumber  int
	HTTPStatus     *int
	Success        bool
	NextRetryAt    *time.Time
	DeadLettered   bool
	CreatedAt      time.Time
}

const deliveryWhere = ` WHERE merchant_id = $1::uuid
  AND ($2::boolean IS NULL OR success = $2::boolean)
  AND ($3::text = '' OR tracking_number = $3::text)
  AND ($4::integer IS NULL OR http_status = $4::integer)`

// ListDeliveryAttempts returns one page of the merchant's attempts (newest
// first) and the total number of matching rows.
func (r *Repository) ListDeliveryAttempts(ctx context.Context, merchantID string, f DeliveryFilter) ([]DeliveryAttempt, int, error) {
	var total int
	if err := r.db.QueryRow(ctx, `SELECT count(*) FROM `+attemptsTable+deliveryWhere,
		merchantID, f.Success, f.TrackingNumber, f.HTTPStatus).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count delivery attempts: %w", err)
	}

	rows, err := r.db.Query(ctx, `SELECT event_id::text, tracking_number, attempt_number, http_status,
       success, next_retry_at, dead_lettered, created_at
FROM `+attemptsTable+deliveryWhere+`
ORDER BY created_at DESC, attempt_number DESC
LIMIT $5 OFFSET $6`, merchantID, f.Success, f.TrackingNumber, f.HTTPStatus, f.Limit, f.Offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list delivery attempts: %w", err)
	}
	defer rows.Close()

	attempts := make([]DeliveryAttempt, 0, f.Limit)
	for rows.Next() {
		var a DeliveryAttempt
		if err := rows.Scan(&a.EventID, &a.TrackingNumber, &a.AttemptNumber, &a.HTTPStatus,
			&a.Success, &a.NextRetryAt, &a.DeadLettered, &a.CreatedAt); err != nil {
			return nil, 0, fmt.Errorf("scan delivery attempt: %w", err)
		}
		attempts = append(attempts, a)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate delivery attempts: %w", err)
	}
	return attempts, total, nil
}
