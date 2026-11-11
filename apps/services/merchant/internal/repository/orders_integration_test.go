//go:build integration

package repository

import (
	"context"
	"crypto/rand"
	"encoding/hex"
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

	"github.com/Vineth-R/swiftdrop/apps/services/merchant/internal/orders"
)

func orderUUID() string {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		panic(err)
	}
	raw[6] = raw[6]&0x0f | 0x40
	raw[8] = raw[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", raw[0:4], raw[4:6], raw[6:8], raw[8:10], raw[10:16])
}

func orderHex(size int) string {
	buffer := make([]byte, size)
	if _, err := rand.Read(buffer); err != nil {
		panic(err)
	}
	return hex.EncodeToString(buffer)
}

func applyOrderSQL(t *testing.T, ctx context.Context, conn *pgx.Conn, path string) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if _, err := conn.Exec(ctx, string(raw)); err != nil {
		t.Fatalf("apply %s: %v", path, err)
	}
}

func newOrderTestPool(t *testing.T) (context.Context, *pgxpool.Pool, *Repository) {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is not set; skipping merchant order repository tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	t.Cleanup(cancel)

	adminConfig, err := pgx.ParseConfig(url)
	if err != nil {
		t.Fatalf("parse TEST_DATABASE_URL: %v", err)
	}
	admin, err := pgx.ConnectConfig(ctx, adminConfig)
	if err != nil {
		t.Fatalf("connect to test database server: %v", err)
	}
	databaseName := "swiftdrop_merchant_order_test_" + orderHex(8)
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
		t.Fatalf("parse migration URL: %v", err)
	}
	migrationConfig.Database = databaseName
	migrationConn, err := pgx.ConnectConfig(ctx, migrationConfig)
	if err != nil {
		t.Fatalf("connect to isolated database: %v", err)
	}
	t.Cleanup(func() { _ = migrationConn.Close(context.Background()) })

	root := filepath.Join("..", "..", "..", "..", "..")
	applyOrderSQL(t, ctx, migrationConn, filepath.Join(root, "infra/docker/postgres/init/01-schemas.sql"))
	migrations, err := filepath.Glob(filepath.Join(root, "migrations/merchant/*.up.sql"))
	if err != nil || len(migrations) == 0 {
		t.Fatalf("find merchant migrations: %v", err)
	}
	sort.Strings(migrations)
	for _, migration := range migrations {
		applyOrderSQL(t, ctx, migrationConn, migration)
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
	repo, err := New(pool)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return ctx, pool, repo
}

func createOrderMerchant(t *testing.T, ctx context.Context, repo *Repository) (string, string) {
	t.Helper()
	merchantID, ownerID := orderUUID(), orderUUID()
	if err := repo.CreateMerchant(ctx, CreateMerchantParams{
		ID: merchantID, OwnerUserID: ownerID, BusinessName: "Order Test " + merchantID[:8],
		BusinessRegNo: "ORDER-" + merchantID[:8], ContactEmail: "order-" + merchantID[:8] + "@example.test",
		ContactPhone: "+94771234567", PickupAddress: "12 Galle Road", PickupZoneID: orderUUID(),
	}); err != nil {
		t.Fatalf("CreateMerchant() error = %v", err)
	}
	return merchantID, ownerID
}

func orderParams(merchantID, external string) orders.CreateParams {
	return orders.CreateParams{
		MerchantID: merchantID, ExternalOrderID: external,
		RequestHash: orderHex(32), ParcelID: orderUUID(), TrackingNumber: "SDN" + orderHex(4),
		Pickup:    orders.Contact{Name: "Shop", Phone: "+94112223344", Address: "1 Road", City: "Colombo", ZoneID: orderUUID()},
		Recipient: orders.Contact{Name: "Amali", Phone: "+94712345678", Address: "2 Road", City: "Kandy", ZoneID: orderUUID()},
		WeightKG:  150, SizeCategory: "SMALL",
		Fee:    orders.Fee{Currency: "LKR", BaseFee: 45000, WeightCharge: 10000, Total: 55000},
		Status: "CREATED",
	}
}

func TestOrderRepositoryCreateAndDetail(t *testing.T) {
	ctx, _, repo := newOrderTestPool(t)
	merchantID, _ := createOrderMerchant(t, ctx, repo)
	params := orderParams(merchantID, "SZ-100245")
	key := "key-1"
	params.IdempotencyKey = &key

	order, err := repo.CreateOrder(ctx, params)
	if err != nil {
		t.Fatalf("CreateOrder() error = %v", err)
	}
	if order.ID == "" || order.TrackingNumber != params.TrackingNumber || order.Fee.Total != 55000 || order.Status != "CREATED" {
		t.Fatalf("order = %+v", order)
	}
	detail, err := repo.OrderDetail(ctx, merchantID, order.ID)
	if err != nil {
		t.Fatalf("OrderDetail() error = %v", err)
	}
	if len(detail.Timeline) != 1 || detail.Timeline[0].Status != "CREATED" {
		t.Fatalf("timeline = %+v", detail.Timeline)
	}
	if detail.Pickup.Name != "Shop" || detail.Recipient.City != "Kandy" || detail.Fee.Total != 55000 {
		t.Fatalf("detail snapshots = %+v", detail)
	}
	if _, err := repo.OrderByExternalOrderID(ctx, merchantID, "SZ-100245"); err != nil {
		t.Fatalf("OrderByExternalOrderID() error = %v", err)
	}
	if _, err := repo.OrderByIdempotencyKey(ctx, merchantID, "key-1"); err != nil {
		t.Fatalf("OrderByIdempotencyKey() error = %v", err)
	}

	other, _ := createOrderMerchant(t, ctx, repo)
	if _, err := repo.OrderDetail(ctx, other, order.ID); !errors.Is(err, orders.ErrNotFound) {
		t.Fatalf("cross-merchant OrderDetail error = %v, want ErrNotFound", err)
	}
}

func TestOrderRepositoryRejectsDuplicateExternalAndKey(t *testing.T) {
	ctx, _, repo := newOrderTestPool(t)
	merchantID, _ := createOrderMerchant(t, ctx, repo)
	if _, err := repo.CreateOrder(ctx, orderParams(merchantID, "SZ-DUP")); err != nil {
		t.Fatalf("first CreateOrder() error = %v", err)
	}
	duplicate := orderParams(merchantID, "SZ-DUP")
	duplicate.ParcelID = orderUUID()
	duplicate.TrackingNumber = "SDN-DUP0002"
	if _, err := repo.CreateOrder(ctx, duplicate); !errors.Is(err, orders.ErrDuplicateOrder) {
		t.Fatalf("duplicate external error = %v, want ErrDuplicateOrder", err)
	}

	key := "same-key"
	first := orderParams(merchantID, "SZ-KEY1")
	first.IdempotencyKey = &key
	if _, err := repo.CreateOrder(ctx, first); err != nil {
		t.Fatalf("first keyed CreateOrder() error = %v", err)
	}
	second := orderParams(merchantID, "SZ-KEY2")
	second.IdempotencyKey = &key
	if _, err := repo.CreateOrder(ctx, second); !errors.Is(err, orders.ErrDuplicateOrder) {
		t.Fatalf("duplicate key error = %v, want ErrDuplicateOrder", err)
	}
}

func TestOrderRepositoryListFiltersAndPagination(t *testing.T) {
	ctx, pool, repo := newOrderTestPool(t)
	merchantID, _ := createOrderMerchant(t, ctx, repo)
	for index, status := range []string{"CREATED", "IN_TRANSIT", "DELIVERED"} {
		params := orderParams(merchantID, fmt.Sprintf("SZ-LIST-%d", index))
		params.Status = status
		order, err := repo.CreateOrder(ctx, params)
		if err != nil {
			t.Fatalf("CreateOrder() error = %v", err)
		}
		if _, err := pool.Exec(ctx, `UPDATE merchant.merchant_orders SET created_at = now() - $2::interval WHERE id = $1`, order.ID, fmt.Sprintf("%d hours", 3-index)); err != nil {
			t.Fatalf("age order: %v", err)
		}
	}

	items, total, err := repo.ListOrders(ctx, orders.ListFilter{MerchantID: merchantID, Page: 1, PageSize: 2, Sort: "createdAt", Desc: false})
	if err != nil {
		t.Fatalf("ListOrders() error = %v", err)
	}
	if total != 3 || len(items) != 2 || items[0].ExternalOrderID != "SZ-LIST-0" {
		t.Fatalf("list = %+v total=%d", items, total)
	}

	items, total, err = repo.ListOrders(ctx, orders.ListFilter{MerchantID: merchantID, Status: "IN_TRANSIT", Page: 1, PageSize: 20, Sort: "createdAt", Desc: true})
	if err != nil || total != 1 || len(items) != 1 || items[0].Status != "IN_TRANSIT" {
		t.Fatalf("status filter = %+v total=%d err=%v", items, total, err)
	}

	items, total, err = repo.ListOrders(ctx, orders.ListFilter{MerchantID: merchantID, ExternalOrderID: "SZ-LIST-2", Page: 1, PageSize: 20, Sort: "createdAt", Desc: true})
	if err != nil || total != 1 || len(items) != 1 || items[0].ExternalOrderID != "SZ-LIST-2" {
		t.Fatalf("external filter = %+v total=%d err=%v", items, total, err)
	}

	items, total, err = repo.ListOrders(ctx, orders.ListFilter{MerchantID: merchantID, Page: 1, PageSize: 20, Sort: "updatedAt", Desc: true, From: ptrTime(time.Now().Add(-48 * time.Hour))})
	if err != nil || total != 3 || len(items) != 3 {
		t.Fatalf("from filter = %+v total=%d err=%v", items, total, err)
	}

	other, _ := createOrderMerchant(t, ctx, repo)
	items, total, err = repo.ListOrders(ctx, orders.ListFilter{MerchantID: other, Page: 1, PageSize: 20, Sort: "createdAt", Desc: true})
	if err != nil || total != 0 || len(items) != 0 {
		t.Fatalf("cross-merchant list = %+v total=%d err=%v", items, total, err)
	}
}

func ptrTime(value time.Time) *time.Time { return &value }

func TestApplyStatusEventIsIdempotentAndOrdered(t *testing.T) {
	ctx, _, repo := newOrderTestPool(t)
	merchantID, _ := createOrderMerchant(t, ctx, repo)
	order, err := repo.CreateOrder(ctx, orderParams(merchantID, "SZ-SYNC"))
	if err != nil {
		t.Fatalf("CreateOrder() error = %v", err)
	}
	newer := orders.StatusEvent{
		EventID: orderUUID(), MerchantID: merchantID, ParcelID: order.ParcelID,
		TrackingNumber: order.TrackingNumber, PreviousStatus: "CREATED", Status: "IN_TRANSIT",
		OccurredAt: time.Now().UTC().Add(time.Minute),
	}
	applied, err := repo.ApplyStatusEvent(ctx, newer)
	if err != nil || !applied {
		t.Fatalf("ApplyStatusEvent() = %v, %v", applied, err)
	}
	detail, _ := repo.OrderDetail(ctx, merchantID, order.ID)
	if detail.Status != "IN_TRANSIT" || len(detail.Timeline) != 2 {
		t.Fatalf("detail = %+v timeline=%+v", detail.Order, detail.Timeline)
	}

	applied, err = repo.ApplyStatusEvent(ctx, newer)
	if err != nil || applied {
		t.Fatalf("duplicate ApplyStatusEvent() = %v, %v", applied, err)
	}
	older := newer
	older.EventID = orderUUID()
	older.Status = "PICKED_UP"
	older.OccurredAt = newer.OccurredAt.Add(-time.Hour)
	applied, err = repo.ApplyStatusEvent(ctx, older)
	if err != nil || applied {
		t.Fatalf("older ApplyStatusEvent() = %v, %v", applied, err)
	}
	detail, _ = repo.OrderDetail(ctx, merchantID, order.ID)
	if detail.Status != "IN_TRANSIT" || len(detail.Timeline) != 2 {
		t.Fatalf("older event regressed state: %+v %+v", detail.Order, detail.Timeline)
	}

	mismatch := newer
	mismatch.EventID = orderUUID()
	mismatch.TrackingNumber = "SDN-WRONG00"
	mismatch.OccurredAt = time.Now().UTC().Add(2 * time.Minute)
	if _, err := repo.ApplyStatusEvent(ctx, mismatch); !errors.Is(err, orders.ErrTrackingMismatch) {
		t.Fatalf("mismatch error = %v, want ErrTrackingMismatch", err)
	}
	unknown := newer
	unknown.EventID = orderUUID()
	unknown.ParcelID = orderUUID()
	unknown.OccurredAt = time.Now().UTC().Add(3 * time.Minute)
	if _, err := repo.ApplyStatusEvent(ctx, unknown); !errors.Is(err, orders.ErrNotFound) {
		t.Fatalf("unknown error = %v, want ErrNotFound", err)
	}
}

func TestApplyStatusEventConcurrentCannotRegress(t *testing.T) {
	ctx, _, repo := newOrderTestPool(t)
	merchantID, _ := createOrderMerchant(t, ctx, repo)
	order, err := repo.CreateOrder(ctx, orderParams(merchantID, "SZ-RACE"))
	if err != nil {
		t.Fatalf("CreateOrder() error = %v", err)
	}
	base := time.Now().UTC()
	newer := orders.StatusEvent{
		EventID: orderUUID(), MerchantID: merchantID, ParcelID: order.ParcelID,
		TrackingNumber: order.TrackingNumber, PreviousStatus: "CREATED", Status: "IN_TRANSIT",
		OccurredAt: base.Add(2 * time.Minute),
	}
	older := orders.StatusEvent{
		EventID: orderUUID(), MerchantID: merchantID, ParcelID: order.ParcelID,
		TrackingNumber: order.TrackingNumber, PreviousStatus: "CREATED", Status: "PICKED_UP",
		OccurredAt: base.Add(time.Minute),
	}

	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, event := range []orders.StatusEvent{newer, older} {
		event := event
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := repo.ApplyStatusEvent(ctx, event)
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent ApplyStatusEvent() error = %v", err)
		}
	}
	detail, _ := repo.OrderDetail(ctx, merchantID, order.ID)
	if detail.Status != "IN_TRANSIT" {
		t.Fatalf("final status = %s, want IN_TRANSIT", detail.Status)
	}
	if len(detail.Timeline) < 2 || detail.Timeline[len(detail.Timeline)-1].Status != "IN_TRANSIT" {
		t.Fatalf("timeline = %+v, want a CREATED + IN_TRANSIT tail", detail.Timeline)
	}
}

func TestApplyStatusEventRollsBackInvalidStatus(t *testing.T) {
	ctx, _, repo := newOrderTestPool(t)
	merchantID, _ := createOrderMerchant(t, ctx, repo)
	order, err := repo.CreateOrder(ctx, orderParams(merchantID, "SZ-ROLLBACK"))
	if err != nil {
		t.Fatalf("CreateOrder() error = %v", err)
	}
	bad := orders.StatusEvent{
		EventID: orderUUID(), MerchantID: merchantID, ParcelID: order.ParcelID,
		TrackingNumber: order.TrackingNumber, PreviousStatus: "CREATED", Status: "NOT_A_STATUS",
		OccurredAt: time.Now().UTC().Add(time.Minute),
	}
	if _, err := repo.ApplyStatusEvent(ctx, bad); err == nil {
		t.Fatal("invalid status was accepted")
	}
	detail, _ := repo.OrderDetail(ctx, merchantID, order.ID)
	if detail.Status != "CREATED" || len(detail.Timeline) != 1 {
		t.Fatalf("partial update after rollback: %+v %+v", detail.Order, detail.Timeline)
	}
}
