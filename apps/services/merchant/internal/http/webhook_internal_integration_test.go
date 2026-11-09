//go:build integration

package servicehttp

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Vineth-R/swiftdrop/apps/services/merchant/internal/repository"
)

// These tests run RecordDeliveryAttempt against a real PostgreSQL database, so
// the SQL (casts, the named unique constraint, NULL handling) is proven and not
// only the handler that calls it.

func attemptParams(t *testing.T, merchantID, eventID string, attempt int) repository.RecordAttemptParams {
	t.Helper()
	return repository.RecordAttemptParams{
		ID:             userID(t),
		MerchantID:     merchantID,
		EventID:        eventID,
		ParcelID:       userID(t),
		TrackingNumber: "SDN4T8W2LC6",
		AttemptNumber:  attempt,
	}
}

func countAttempts(t *testing.T, ctx context.Context, pool *pgxpool.Pool, merchantID string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM merchant.webhook_delivery_attempts WHERE merchant_id = $1::uuid`, merchantID).Scan(&n); err != nil {
		t.Fatalf("count attempts: %v", err)
	}
	return n
}

func TestRecordDeliveryAttemptStoresAndIsIdempotent(t *testing.T) {
	ctx, pool, repo := newWebhookTestPool(t)
	merchantID, _ := createOwnedMerchant(t, ctx, repo)
	saveWebhook(t, ctx, repo, repository.SaveWebhookParams{
		MerchantID: merchantID, URL: "https://shopzone.lk/hooks", IsActive: true, CandidateSecretEnc: []byte("v1:secret"),
	})
	eventID := userID(t)

	status, ms := 503, 182
	message := "upstream unavailable"
	retryAt := time.Date(2026, 11, 4, 12, 35, 0, 0, time.UTC)
	first := attemptParams(t, merchantID, eventID, 1)
	first.HTTPStatus, first.DurationMS, first.ErrorMessage, first.NextRetryAt = &status, &ms, &message, &retryAt

	created, err := repo.RecordDeliveryAttempt(ctx, first)
	if err != nil || !created {
		t.Fatalf("first record: created = %v, err = %v", created, err)
	}

	// The same report again (a retry or a redelivered message) writes nothing.
	again := first
	again.ID = userID(t)
	created, err = repo.RecordDeliveryAttempt(ctx, again)
	if err != nil || created {
		t.Fatalf("duplicate record: created = %v, err = %v, want false and no error", created, err)
	}
	if n := countAttempts(t, ctx, pool, merchantID); n != 1 {
		t.Fatalf("rows = %d, want 1", n)
	}

	// A different attempt number for the same event is a new row.
	second := attemptParams(t, merchantID, eventID, 2)
	second.Success = true
	ok := 200
	second.HTTPStatus = &ok
	if created, err = repo.RecordDeliveryAttempt(ctx, second); err != nil || !created {
		t.Fatalf("second attempt: created = %v, err = %v", created, err)
	}
	if n := countAttempts(t, ctx, pool, merchantID); n != 2 {
		t.Fatalf("rows = %d, want 2", n)
	}

	// The delivery log (API 8.15) shows what was recorded, newest first.
	attempts, total, err := repo.ListDeliveryAttempts(ctx, merchantID, repository.DeliveryFilter{Limit: 10})
	if err != nil || total != 2 || len(attempts) != 2 {
		t.Fatalf("list: total = %d, len = %d, err = %v", total, len(attempts), err)
	}
	var failed repository.DeliveryAttempt
	for _, a := range attempts {
		if a.AttemptNumber == 1 {
			failed = a
		}
	}
	if failed.Success || failed.HTTPStatus == nil || *failed.HTTPStatus != 503 ||
		failed.NextRetryAt == nil || !failed.NextRetryAt.Equal(retryAt) || failed.DeadLettered {
		t.Fatalf("stored attempt 1 = %+v", failed)
	}
}

func TestRecordDeliveryAttemptNullableFieldsAndDeadLetter(t *testing.T) {
	ctx, pool, repo := newWebhookTestPool(t)
	merchantID, _ := createOwnedMerchant(t, ctx, repo)
	saveWebhook(t, ctx, repo, repository.SaveWebhookParams{
		MerchantID: merchantID, URL: "https://shopzone.lk/hooks", IsActive: true, CandidateSecretEnc: []byte("v1:secret"),
	})

	last := attemptParams(t, merchantID, userID(t), 6)
	last.DeadLettered = true // no status (timeout), no error text, no duration
	if created, err := repo.RecordDeliveryAttempt(ctx, last); err != nil || !created {
		t.Fatalf("created = %v, err = %v", created, err)
	}

	var httpStatus, duration *int
	var message *string
	var retry *time.Time
	var dead, success bool
	if err := pool.QueryRow(ctx, `SELECT http_status, duration_ms, error_message, next_retry_at, dead_lettered, success
FROM merchant.webhook_delivery_attempts WHERE merchant_id = $1::uuid`, merchantID).
		Scan(&httpStatus, &duration, &message, &retry, &dead, &success); err != nil {
		t.Fatalf("read back: %v", err)
	}
	if httpStatus != nil || duration != nil || message != nil || retry != nil || !dead || success {
		t.Fatalf("stored = status %v, duration %v, message %v, retry %v, dead %v, success %v", httpStatus, duration, message, retry, dead, success)
	}
}

func TestRecordDeliveryAttemptWithoutWebhook(t *testing.T) {
	ctx, pool, repo := newWebhookTestPool(t)
	merchantID, _ := createOwnedMerchant(t, ctx, repo)

	_, err := repo.RecordDeliveryAttempt(ctx, attemptParams(t, merchantID, userID(t), 1))
	if !errors.Is(err, repository.ErrWebhookNotConfigured) {
		t.Fatalf("err = %v, want ErrWebhookNotConfigured", err)
	}
	if n := countAttempts(t, ctx, pool, merchantID); n != 0 {
		t.Fatalf("rows = %d, want 0", n)
	}
}

func TestRecordDeliveryAttemptRejectsOutOfRangeAttemptNumber(t *testing.T) {
	ctx, _, repo := newWebhookTestPool(t)
	merchantID, _ := createOwnedMerchant(t, ctx, repo)
	saveWebhook(t, ctx, repo, repository.SaveWebhookParams{
		MerchantID: merchantID, URL: "https://shopzone.lk/hooks", IsActive: true, CandidateSecretEnc: []byte("v1:secret"),
	})
	// The handler stops this earlier; the CHECK constraint is the last guard.
	if _, err := repo.RecordDeliveryAttempt(ctx, attemptParams(t, merchantID, userID(t), 7)); err == nil {
		t.Fatal("attempt number 7 was accepted")
	}
}

func TestRecordDeliveryAttemptsAreScopedToTheMerchant(t *testing.T) {
	ctx, pool, repo := newWebhookTestPool(t)
	merchantA, _ := createOwnedMerchant(t, ctx, repo)
	merchantB, _ := createOwnedMerchant(t, ctx, repo)
	for _, id := range []string{merchantA, merchantB} {
		saveWebhook(t, ctx, repo, repository.SaveWebhookParams{
			MerchantID: id, URL: "https://shopzone.lk/hooks", IsActive: true, CandidateSecretEnc: []byte("v1:secret"),
		})
	}
	eventID := userID(t)
	// The same event and attempt number for two merchants are two separate rows.
	for _, id := range []string{merchantA, merchantB} {
		if created, err := repo.RecordDeliveryAttempt(ctx, attemptParams(t, id, eventID, 1)); err != nil || !created {
			t.Fatalf("merchant %s: created = %v, err = %v", id, created, err)
		}
	}
	for _, id := range []string{merchantA, merchantB} {
		if n := countAttempts(t, ctx, pool, id); n != 1 {
			t.Fatalf("merchant %s has %d rows, want 1", id, n)
		}
	}
}
