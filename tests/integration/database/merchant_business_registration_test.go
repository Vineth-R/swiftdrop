//go:build integration

package database_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

var merchantUpMigrations = []string{
	"migrations/merchant/000001_initial_schema.up.sql",
	"migrations/merchant/000002_tables.up.sql",
	"migrations/merchant/000006_merchant_constraints.up.sql",
	"migrations/merchant/000007_registration_onboarding.up.sql",
	"migrations/merchant/000008_business_reg_no_optional.up.sql",
	"migrations/merchant/000009_restore_business_reg_no_required.up.sql",
	"migrations/merchant/000010_merchant_order_snapshots.up.sql",
}

func applyMerchantThrough(t *testing.T, ctx context.Context, conn *pgx.Conn, through int) {
	t.Helper()
	applySQLFile(t, ctx, conn, "infra/docker/postgres/init/01-schemas.sql")
	for version := 1; version <= through; version++ {
		applySQLFile(t, ctx, conn, merchantUpMigrations[version-1])
	}
}

func insertMerchantWithRegistration(ctx context.Context, conn *pgx.Conn, registration any) (string, error) {
	var id string
	err := conn.QueryRow(ctx, `
		INSERT INTO merchant.merchants (
			id, owner_user_id, business_name, business_reg_no, business_address,
			contact_email, contact_phone, pickup_address, pickup_zone_id, status
		) VALUES (
			gen_random_uuid(), gen_random_uuid(), 'Test Shop', $1, '1 Test Road',
			'shop@example.test', '+94112223344', '1 Test Road', gen_random_uuid(), 'PENDING'
		) RETURNING id::text`, registration).Scan(&id)
	return id, err
}

func insertMerchantBaseline(ctx context.Context, conn *pgx.Conn, registration any) error {
	_, err := conn.Exec(ctx, `
		INSERT INTO merchant.merchants (
			id, owner_user_id, business_name, business_reg_no,
			contact_email, contact_phone, pickup_address, pickup_zone_id, status
		) VALUES (
			gen_random_uuid(), gen_random_uuid(), 'Test Shop', $1,
			'shop@example.test', '+94112223344', '1 Test Road', gen_random_uuid(), 'PENDING'
		)`, registration)
	return err
}

func expectConstraintViolation(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("invalid business registration number was accepted")
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23514" || pgErr.ConstraintName != "merchants_business_reg_no_present" {
		t.Fatalf("error = %v, want merchants_business_reg_no_present check violation", err)
	}
}

func TestMerchantBusinessRegistrationConstraint(t *testing.T) {
	ctx, conn := newIsolatedDatabase(t)
	applyMerchantThrough(t, ctx, conn, 6)

	validID, err := insertMerchantWithRegistration(ctx, conn, "M9-TEST-1")
	if err != nil {
		t.Fatalf("valid registration number rejected: %v", err)
	}

	for _, invalid := range []any{nil, "", "   ", "\t\n"} {
		_, err := insertMerchantWithRegistration(ctx, conn, invalid)
		expectConstraintViolation(t, err)
	}

	_, err = conn.Exec(ctx, `UPDATE merchant.merchants SET business_reg_no = NULL WHERE id = $1`, validID)
	expectConstraintViolation(t, err)
	_, err = conn.Exec(ctx, `UPDATE merchant.merchants SET business_reg_no = '   ' WHERE id = $1`, validID)
	expectConstraintViolation(t, err)
	if _, err := conn.Exec(ctx, `UPDATE merchant.merchants SET business_reg_no = 'M9-TEST-2' WHERE id = $1`, validID); err != nil {
		t.Fatalf("valid update rejected: %v", err)
	}
}

func TestMerchantBusinessRegistrationUpgradeFromVersionEight(t *testing.T) {
	ctx, conn := newIsolatedDatabase(t)
	applyMerchantThrough(t, ctx, conn, 5)
	applySQLFile(t, ctx, conn, "migrations/merchant/000009_restore_business_reg_no_required.up.sql")

	if _, err := insertMerchantWithRegistration(ctx, conn, "M9-UPGRADE-1"); err != nil {
		t.Fatalf("valid registration number rejected after upgrade: %v", err)
	}
	_, err := insertMerchantWithRegistration(ctx, conn, "")
	expectConstraintViolation(t, err)
}

func TestMerchantBusinessRegistrationRejectsInvalidExistingRows(t *testing.T) {
	ctx, conn := newIsolatedDatabase(t)
	applySQLFile(t, ctx, conn, "infra/docker/postgres/init/01-schemas.sql")
	for _, path := range merchantUpMigrations[:3] {
		applySQLFile(t, ctx, conn, path)
	}
	for _, invalid := range []any{nil, "", "   "} {
		if err := insertMerchantBaseline(ctx, conn, invalid); err != nil {
			t.Fatalf("pre-000007 invalid fixture could not be inserted: %v", err)
		}
	}
	applySQLFile(t, ctx, conn, merchantUpMigrations[3])
	applySQLFile(t, ctx, conn, merchantUpMigrations[4])

	sql, err := os.ReadFile(repositoryPath(merchantUpMigrations[5]))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.PgConn().Exec(ctx, string(sql)).ReadAll(); err == nil {
		t.Fatal("000009 accepted an existing NULL registration number")
	} else if !strings.Contains(err.Error(), "merchants_business_reg_no_present") {
		t.Fatalf("unexpected 000009 failure: %v", err)
	}
}

func TestMerchantBusinessRegistrationRollbackAndReapply(t *testing.T) {
	ctx, conn := newIsolatedDatabase(t)
	applyMerchantThrough(t, ctx, conn, 6)
	applySQLFile(t, ctx, conn, "migrations/merchant/000009_restore_business_reg_no_required.down.sql")

	var count int
	if err := conn.QueryRow(ctx, `SELECT count(*) FROM pg_constraint WHERE conname = 'merchants_business_reg_no_present'`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("constraint after rollback = %d, error = %v", count, err)
	}
	applySQLFile(t, ctx, conn, "migrations/merchant/000009_restore_business_reg_no_required.up.sql")
	if _, err := insertMerchantWithRegistration(ctx, conn, "M9-ROLLBACK-1"); err != nil {
		t.Fatalf("valid registration number rejected after reapply: %v", err)
	}
}
