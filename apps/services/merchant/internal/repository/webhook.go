package repository

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// ErrWebhookNotConfigured means the merchant has not saved a webhook yet.
var ErrWebhookNotConfigured = errors.New("webhook not configured")

// Table names live in one place. If your other queries write the schema
// ("merchant.merchant_api_clients"), change these two values to match.
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
	// SecretEnc is the encrypted secret. It is written on insert, and on update
	// only when ReplaceSecret is true (first save or rotation).
	SecretEnc     []byte
	ReplaceSecret bool
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

// SaveWebhook inserts the webhook or, if the merchant already has one, updates
// it. The unique (merchant_id) constraint makes this safe when two requests
// arrive together.
func (r *Repository) SaveWebhook(ctx context.Context, p SaveWebhookParams) (Webhook, error) {
	const query = `
INSERT INTO ` + webhooksTable + ` AS w
    (id, merchant_id, url, signing_secret_enc, event_types, is_active, created_at, updated_at)
VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6, now(), now())
ON CONFLICT (merchant_id) DO UPDATE SET
    url = EXCLUDED.url,
    event_types = EXCLUDED.event_types,
    is_active = EXCLUDED.is_active,
    signing_secret_enc = CASE WHEN $7::boolean THEN EXCLUDED.signing_secret_enc ELSE w.signing_secret_enc END,
    updated_at = now()
RETURNING ` + webhookColumns
	return scanWebhook(r.db.QueryRow(ctx, query,
		p.ID, p.MerchantID, p.URL, p.SecretEnc, p.EventTypes, p.IsActive, p.ReplaceSecret))
}

// DeliveryFilter narrows the delivery log (API 8.15).
type DeliveryFilter struct {
	Success        *bool  // nil means both
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

// ListDeliveryAttempts returns one page of the merchant's attempts (newest
// first) and the total number of matching rows.
func (r *Repository) ListDeliveryAttempts(ctx context.Context, merchantID string, f DeliveryFilter) ([]DeliveryAttempt, int, error) {
	const where = ` WHERE merchant_id = $1::uuid
  AND ($2::boolean IS NULL OR success = $2::boolean)
  AND ($3::text = '' OR tracking_number = $3::text)`

	var total int
	if err := r.db.QueryRow(ctx, `SELECT count(*) FROM `+attemptsTable+where,
		merchantID, f.Success, f.TrackingNumber).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := r.db.Query(ctx, `SELECT event_id::text, tracking_number, attempt_number, http_status,
       success, next_retry_at, dead_lettered, created_at
FROM `+attemptsTable+where+`
ORDER BY created_at DESC, attempt_number DESC
LIMIT $4 OFFSET $5`, merchantID, f.Success, f.TrackingNumber, f.Limit, f.Offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	attempts := make([]DeliveryAttempt, 0)
	for rows.Next() {
		var a DeliveryAttempt
		if err := rows.Scan(&a.EventID, &a.TrackingNumber, &a.AttemptNumber, &a.HTTPStatus,
			&a.Success, &a.NextRetryAt, &a.DeadLettered, &a.CreatedAt); err != nil {
			return nil, 0, err
		}
		attempts = append(attempts, a)
	}
	return attempts, total, rows.Err()
}
