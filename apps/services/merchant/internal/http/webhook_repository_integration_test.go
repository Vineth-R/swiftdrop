//go:build integration

package servicehttp

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Vineth-R/swiftdrop/apps/services/merchant/internal/repository"
)

// These tests exercise the real webhook repository SQL against a disposable
// PostgreSQL database. They follow the merchant API-key integration fixture:
// create a uniquely named database, apply the schemas and merchant
// migrations, then drop it.

const webhookDBTimeout = 120 * time.Second

func newWebhookTestPool(t *testing.T) (context.Context, *pgxpool.Pool, *repository.Repository) {
	t.Helper()

	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is not set; skipping merchant webhook repository tests")
	}

	ctx, cancel := context.WithTimeout(context.Background(), webhookDBTimeout)
	t.Cleanup(cancel)

	adminConfig, err := pgx.ParseConfig(url)
	if err != nil {
		t.Fatalf("parse TEST_DATABASE_URL: %v", err)
	}
	admin, err := pgx.ConnectConfig(ctx, adminConfig)
	if err != nil {
		t.Fatalf("connect to test database server: %v", err)
	}

	databaseName := "swiftdrop_merchant_webhook_test_" + randomHex(8)
	quoted := pgx.Identifier{databaseName}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+quoted); err != nil {
		_ = admin.Close(ctx)
		t.Fatalf("create isolated database: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		if _, err := admin.Exec(cleanupCtx, "DROP DATABASE IF EXISTS "+quoted+" WITH (FORCE)"); err != nil {
			t.Logf("drop isolated database %s: %v", databaseName, err)
		}
		_ = admin.Close(cleanupCtx)
	})

	migrationConfig, err := pgx.ParseConfig(url)
	if err != nil {
		t.Fatalf("parse TEST_DATABASE_URL for isolated database: %v", err)
	}
	migrationConfig.Database = databaseName
	migrationConn, err := pgx.ConnectConfig(ctx, migrationConfig)
	if err != nil {
		t.Fatalf("connect to isolated database: %v", err)
	}
	t.Cleanup(func() { _ = migrationConn.Close(context.Background()) })

	root := filepath.Join("..", "..", "..", "..", "..")
	applySQL(t, ctx, migrationConn, filepath.Join(root, "infra/docker/postgres/init/01-schemas.sql"))
	migrations, err := filepath.Glob(filepath.Join(root, "migrations/merchant/*.up.sql"))
	if err != nil || len(migrations) == 0 {
		t.Fatalf("find merchant migrations: %v", err)
	}
	sort.Strings(migrations)
	for _, migration := range migrations {
		applySQL(t, ctx, migrationConn, migration)
	}

	poolConfig, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatalf("parse pool config: %v", err)
	}
	poolConfig.ConnConfig.Database = databaseName
	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		t.Fatalf("create pool: %v", err)
	}
	t.Cleanup(pool.Close)

	repo, err := repository.New(pool)
	if err != nil {
		t.Fatalf("repository.New returned error: %v", err)
	}
	return ctx, pool, repo
}

func createOwnedMerchant(t *testing.T, ctx context.Context, repo *repository.Repository) (merchantID, ownerUserID string) {
	t.Helper()
	merchantID, ownerUserID = userID(t), userID(t)
	if err := repo.CreateMerchant(ctx, repository.CreateMerchantParams{
		ID:            merchantID,
		OwnerUserID:   ownerUserID,
		BusinessName:  "Webhook Test " + merchantID[:8],
		BusinessRegNo: "WEBHOOK-" + merchantID[:8],
		ContactEmail:  "owner-" + merchantID[:8] + "@example.test",
		ContactPhone:  "+94771234567",
		PickupAddress: "12 Galle Road, Colombo",
		PickupZoneID:  userID(t),
	}); err != nil {
		t.Fatalf("CreateMerchant returned error: %v", err)
	}
	return merchantID, ownerUserID
}

func saveWebhook(t *testing.T, ctx context.Context, repo *repository.Repository, params repository.SaveWebhookParams) repository.WebhookSaveResult {
	t.Helper()
	if params.ID == "" {
		params.ID = userID(t)
	}
	if len(params.EventTypes) == 0 {
		params.EventTypes = []string{"parcel.status_changed"}
	}
	result, err := repo.SaveWebhook(ctx, params)
	if err != nil {
		t.Fatalf("SaveWebhook returned error: %v", err)
	}
	return result
}

func TestWebhookRepositorySaveAndGet(t *testing.T) {
	ctx, _, repo := newWebhookTestPool(t)
	merchantID, ownerUserID := createOwnedMerchant(t, ctx, repo)

	found, err := repo.MerchantByOwnerUserID(ctx, ownerUserID)
	if err != nil {
		t.Fatalf("MerchantByOwnerUserID returned error: %v", err)
	}
	if found.ID != merchantID || found.OwnerUserID != ownerUserID {
		t.Fatalf("MerchantByOwnerUserID = %+v", found)
	}
	if _, err := repo.MerchantByOwnerUserID(ctx, userID(t)); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("unknown owner error = %v, want ErrNotFound", err)
	}

	if _, err := repo.WebhookByMerchant(ctx, merchantID); !errors.Is(err, repository.ErrWebhookNotConfigured) {
		t.Fatalf("WebhookByMerchant before save = %v, want ErrWebhookNotConfigured", err)
	}

	// A binary value proves BYTEA round-trips without text conversion.
	secret := []byte{0x00, 0x01, 0x7f, 0xfe, 0xff, 'v', '1', ':'}
	result := saveWebhook(t, ctx, repo, repository.SaveWebhookParams{
		MerchantID:         merchantID,
		URL:                "https://shopzone.lk/hooks/swiftdrop",
		EventTypes:         []string{"parcel.status_changed", "parcel.assigned"},
		IsActive:           true,
		CandidateSecretEnc: secret,
	})
	if !result.SecretStored {
		t.Fatal("the first save did not report the secret as stored")
	}

	stored, err := repo.WebhookByMerchant(ctx, merchantID)
	if err != nil {
		t.Fatalf("WebhookByMerchant returned error: %v", err)
	}
	if stored.ID == "" || stored.MerchantID != merchantID {
		t.Fatalf("stored webhook identity = %+v", stored)
	}
	if stored.URL != "https://shopzone.lk/hooks/swiftdrop" || !stored.IsActive {
		t.Fatalf("stored webhook fields = %+v", stored)
	}
	if string(stored.SigningSecretEnc) != string(secret) {
		t.Fatalf("BYTEA secret = %x, want %x", stored.SigningSecretEnc, secret)
	}
	if len(stored.EventTypes) != 2 || stored.EventTypes[0] != "parcel.status_changed" || stored.EventTypes[1] != "parcel.assigned" {
		t.Fatalf("TEXT[] event types = %v", stored.EventTypes)
	}
	if stored.CreatedAt.IsZero() || stored.UpdatedAt.IsZero() {
		t.Fatalf("timestamps not set: %+v", stored)
	}
}

func TestWebhookRepositoryUpdatePreservesSecretUnlessRotated(t *testing.T) {
	ctx, _, repo := newWebhookTestPool(t)
	merchantID, _ := createOwnedMerchant(t, ctx, repo)

	firstSecret := []byte("v1:first-secret")
	first := saveWebhook(t, ctx, repo, repository.SaveWebhookParams{
		MerchantID:         merchantID,
		URL:                "https://shopzone.lk/a",
		EventTypes:         []string{"parcel.status_changed"},
		IsActive:           true,
		CandidateSecretEnc: firstSecret,
	})
	if !first.SecretStored {
		t.Fatal("first save did not store the secret")
	}

	second := saveWebhook(t, ctx, repo, repository.SaveWebhookParams{
		MerchantID:         merchantID,
		URL:                "https://shopzone.lk/b",
		EventTypes:         []string{"parcel.status_changed"},
		IsActive:           false,
		CandidateSecretEnc: []byte("v1:second-secret"),
	})
	if second.SecretStored {
		t.Fatal("an update without rotation reported the candidate as stored")
	}
	if string(second.Webhook.SigningSecretEnc) != string(firstSecret) {
		t.Fatalf("secret changed without rotation: %x", second.Webhook.SigningSecretEnc)
	}
	if second.Webhook.URL != "https://shopzone.lk/b" || second.Webhook.IsActive {
		t.Fatalf("fields were not updated: %+v", second.Webhook)
	}

	rotatedSecret := []byte("v1:rotated-secret")
	rotated := saveWebhook(t, ctx, repo, repository.SaveWebhookParams{
		MerchantID:         merchantID,
		URL:                "https://shopzone.lk/c",
		EventTypes:         []string{"parcel.status_changed"},
		IsActive:           true,
		RotateSecret:       true,
		CandidateSecretEnc: rotatedSecret,
	})
	if !rotated.SecretStored {
		t.Fatal("rotation did not report the secret as stored")
	}
	if string(rotated.Webhook.SigningSecretEnc) != string(rotatedSecret) {
		t.Fatalf("rotation stored %x, want %x", rotated.Webhook.SigningSecretEnc, rotatedSecret)
	}
}

func TestWebhookRepositoryListDeliveryAttempts(t *testing.T) {
	ctx, pool, repo := newWebhookTestPool(t)
	merchantID, _ := createOwnedMerchant(t, ctx, repo)
	otherMerchantID, _ := createOwnedMerchant(t, ctx, repo)

	webhook := saveWebhook(t, ctx, repo, repository.SaveWebhookParams{
		MerchantID:         merchantID,
		URL:                "https://shopzone.lk/hooks",
		EventTypes:         []string{"parcel.status_changed"},
		IsActive:           true,
		CandidateSecretEnc: []byte("v1:secret"),
	})
	otherWebhook := saveWebhook(t, ctx, repo, repository.SaveWebhookParams{
		MerchantID:         otherMerchantID,
		URL:                "https://other.example.test/hooks",
		EventTypes:         []string{"parcel.status_changed"},
		IsActive:           true,
		CandidateSecretEnc: []byte("v1:other-secret"),
	})

	base := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	statusOK, statusFail, statusOther := 200, 503, 500
	insertAttempt(t, ctx, pool, webhook.Webhook.ID, merchantID, "SDK7M2Q9XP1", 1, &statusOK, true, false, base)
	insertAttempt(t, ctx, pool, webhook.Webhook.ID, merchantID, "SDK7M2Q9XP2", 2, &statusFail, false, false, base.Add(time.Minute))
	insertAttempt(t, ctx, pool, webhook.Webhook.ID, merchantID, "SDK7M2Q9XP2", 3, &statusOther, false, true, base.Add(2*time.Minute))
	insertAttempt(t, ctx, pool, otherWebhook.Webhook.ID, otherMerchantID, "SDK7M2Q9XP3", 1, &statusOK, true, false, base.Add(3*time.Minute))

	all, total, err := repo.ListDeliveryAttempts(ctx, merchantID, repository.DeliveryFilter{Limit: 20, Offset: 0})
	if err != nil {
		t.Fatalf("ListDeliveryAttempts returned error: %v", err)
	}
	if total != 3 || len(all) != 3 {
		t.Fatalf("all attempts = %d total %d, want 3", len(all), total)
	}
	// Newest first, then attempt number descending.
	if all[0].AttemptNumber != 3 || all[1].AttemptNumber != 2 || all[2].AttemptNumber != 1 {
		t.Fatalf("ordering = %+v", all)
	}
	if all[0].HTTPStatus == nil || *all[0].HTTPStatus != 500 || !all[0].DeadLettered {
		t.Fatalf("first row = %+v", all[0])
	}
	if all[1].HTTPStatus == nil || *all[1].HTTPStatus != 503 || all[1].Success {
		t.Fatalf("middle row = %+v", all[1])
	}
	if all[2].HTTPStatus == nil || *all[2].HTTPStatus != 200 || !all[2].Success {
		t.Fatalf("last row = %+v", all[2])
	}

	success := true
	successful, successTotal, err := repo.ListDeliveryAttempts(ctx, merchantID, repository.DeliveryFilter{Success: &success, Limit: 20})
	if err != nil {
		t.Fatalf("success filter error: %v", err)
	}
	if successTotal != 1 || len(successful) != 1 || successful[0].TrackingNumber != "SDK7M2Q9XP1" {
		t.Fatalf("success filter = %+v total %d", successful, successTotal)
	}

	failed := false
	failedAttempts, failedTotal, err := repo.ListDeliveryAttempts(ctx, merchantID, repository.DeliveryFilter{Success: &failed, Limit: 20})
	if err != nil {
		t.Fatalf("failure filter error: %v", err)
	}
	if failedTotal != 2 || len(failedAttempts) != 2 {
		t.Fatalf("failure filter = %d total %d, want 2", len(failedAttempts), failedTotal)
	}

	byTracking, trackingTotal, err := repo.ListDeliveryAttempts(ctx, merchantID, repository.DeliveryFilter{TrackingNumber: "SDK7M2Q9XP2", Limit: 20})
	if err != nil {
		t.Fatalf("tracking filter error: %v", err)
	}
	if trackingTotal != 2 || len(byTracking) != 2 {
		t.Fatalf("tracking filter = %d total %d, want 2", len(byTracking), trackingTotal)
	}

	page, pageTotal, err := repo.ListDeliveryAttempts(ctx, merchantID, repository.DeliveryFilter{Limit: 1, Offset: 1})
	if err != nil {
		t.Fatalf("pagination error: %v", err)
	}
	if pageTotal != 3 || len(page) != 1 || page[0].AttemptNumber != 2 {
		t.Fatalf("page = %+v total %d", page, pageTotal)
	}

	empty, emptyTotal, err := repo.ListDeliveryAttempts(ctx, merchantID, repository.DeliveryFilter{TrackingNumber: "SDNOPE0000", Limit: 20})
	if err != nil {
		t.Fatalf("empty filter error: %v", err)
	}
	if emptyTotal != 0 || empty == nil || len(empty) != 0 {
		t.Fatalf("empty result = %v total %d, want a non-nil empty slice", empty, emptyTotal)
	}

	other, otherTotal, err := repo.ListDeliveryAttempts(ctx, otherMerchantID, repository.DeliveryFilter{Limit: 20})
	if err != nil {
		t.Fatalf("other merchant error: %v", err)
	}
	if otherTotal != 1 || len(other) != 1 || other[0].TrackingNumber != "SDK7M2Q9XP3" {
		t.Fatalf("merchant isolation failed: %+v total %d", other, otherTotal)
	}
}

func TestWebhookRepositoryConcurrentFirstSave(t *testing.T) {
	ctx, _, repo := newWebhookTestPool(t)
	merchantID, _ := createOwnedMerchant(t, ctx, repo)

	type outcome struct {
		secret []byte
		stored bool
		err    error
	}
	secrets := [][]byte{[]byte("v1:concurrent-a"), []byte("v1:concurrent-b")}
	ids := []string{userID(t), userID(t)}
	results := make(chan outcome, len(secrets))
	var start sync.WaitGroup
	start.Add(1)
	var done sync.WaitGroup
	done.Add(len(secrets))
	for i := range secrets {
		go func(i int) {
			defer done.Done()
			start.Wait()
			result, err := repo.SaveWebhook(ctx, repository.SaveWebhookParams{
				ID:                 ids[i],
				MerchantID:         merchantID,
				URL:                fmt.Sprintf("https://shopzone.lk/%d", i),
				EventTypes:         []string{"parcel.status_changed"},
				IsActive:           true,
				CandidateSecretEnc: secrets[i],
			})
			results <- outcome{secret: secrets[i], stored: result.SecretStored, err: err}
		}(i)
	}
	start.Done()
	done.Wait()
	close(results)

	storedCount := 0
	winner := outcome{}
	for result := range results {
		if result.err != nil {
			t.Fatalf("concurrent SaveWebhook returned error: %v", result.err)
		}
		if result.stored {
			storedCount++
			winner = result
		}
	}
	if storedCount != 1 {
		t.Fatalf("%d concurrent first saves reported a stored secret, want exactly 1", storedCount)
	}

	stored, err := repo.WebhookByMerchant(ctx, merchantID)
	if err != nil {
		t.Fatalf("WebhookByMerchant returned error: %v", err)
	}
	if string(stored.SigningSecretEnc) != string(winner.secret) {
		t.Fatalf("persisted secret %x is not the secret returned to the winner %x", stored.SigningSecretEnc, winner.secret)
	}

	// A follow-up save without rotation still keeps the winner's secret.
	followUp := saveWebhook(t, ctx, repo, repository.SaveWebhookParams{
		MerchantID:         merchantID,
		URL:                "https://shopzone.lk/follow-up",
		EventTypes:         []string{"parcel.status_changed"},
		IsActive:           true,
		CandidateSecretEnc: []byte("v1:unused"),
	})
	if followUp.SecretStored {
		t.Fatal("a follow-up save replaced the stored secret")
	}
	if string(followUp.Webhook.SigningSecretEnc) != string(winner.secret) {
		t.Fatal("the follow-up save changed the stored secret")
	}
}

func insertAttempt(t *testing.T, ctx context.Context, pool *pgxpool.Pool,
	webhookID, merchantID, trackingNumber string, attempt int, httpStatus *int,
	success, deadLettered bool, createdAt time.Time,
) {
	t.Helper()
	if _, err := pool.Exec(ctx, `
INSERT INTO merchant.webhook_delivery_attempts
    (event_id, webhook_id, merchant_id, parcel_id, tracking_number, attempt_number,
     http_status, success, next_retry_at, dead_lettered, duration_ms, created_at)
VALUES ($1::uuid, $2::uuid, $3::uuid, $4::uuid, $5, $6, $7, $8, NULL, $9, 12, $10)`,
		userID(t), webhookID, merchantID, userID(t), trackingNumber, attempt,
		httpStatus, success, deadLettered, createdAt,
	); err != nil {
		t.Fatalf("insert delivery attempt: %v", err)
	}
}
