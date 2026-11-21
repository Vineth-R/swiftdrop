//go:build integration

package database_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestPricingMigrations(t *testing.T) {
	ctx, conn := newIsolatedDatabase(t)

	applySQLFile(t, ctx, conn, "infra/docker/postgres/init/01-schemas.sql")
	applySQLFile(t, ctx, conn, "migrations/pricing/000001_initial_schema.up.sql")
	applySQLFile(t, ctx, conn, "migrations/pricing/000002_tables.up.sql")

	t.Run("seed data", func(t *testing.T) {
		for _, test := range []struct {
			name  string
			query string
			want  int
		}{
			{name: "zones", query: "SELECT count(*) FROM pricing.zones", want: 5},
			{name: "rules", query: "SELECT count(*) FROM pricing.pricing_rules", want: 25},
			{name: "weight bands", query: "SELECT count(*) FROM pricing.weight_bands", want: 4},
			{name: "size surcharges", query: "SELECT count(*) FROM pricing.size_surcharges", want: 4},
			{name: "audit logs", query: "SELECT count(*) FROM pricing.audit_logs", want: 0},
		} {
			var got int
			if err := conn.QueryRow(ctx, test.query).Scan(&got); err != nil {
				t.Fatalf("%s: %v", test.name, err)
			}
			if got != test.want {
				t.Fatalf("%s = %d, want %d", test.name, got, test.want)
			}
		}
		for _, code := range []string{"CMB", "KDY"} {
			var exists bool
			if err := conn.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM pricing.zones WHERE code=$1 AND is_active)", code).Scan(&exists); err != nil || !exists {
				t.Fatalf("%s seed zone missing or inactive: %v", code, err)
			}
		}
	})

	t.Run("weight band boundaries", func(t *testing.T) {
		for _, test := range []struct {
			weight string
			want   string
		}{
			{weight: "1", want: "0.00"},
			{weight: "5", want: "100.00"},
			{weight: "5.01", want: "250.00"},
			{weight: "10", want: "250.00"},
			{weight: "10.01", want: "500.00"},
			{weight: "25", want: "500.00"},
		} {
			var charge string
			err := conn.QueryRow(ctx, `SELECT charge::text FROM pricing.weight_bands
				WHERE is_active AND min_weight_kg < $1::numeric AND max_weight_kg >= $1::numeric`, test.weight).Scan(&charge)
			if err != nil {
				t.Fatalf("weight %s: %v", test.weight, err)
			}
			if charge != test.want {
				t.Fatalf("weight %s charge = %s, want %s", test.weight, charge, test.want)
			}
		}
		var count int
		if err := conn.QueryRow(ctx, `SELECT count(*) FROM pricing.weight_bands
			WHERE is_active AND min_weight_kg < 25.01 AND max_weight_kg >= 25.01`).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("25.01 kg matched %d bands, want 0", count)
		}
	})

	t.Run("repeated execution is idempotent", func(t *testing.T) {
		applySQLFile(t, ctx, conn, "migrations/pricing/000002_tables.up.sql")
		var zones, rules, bands, surcharges int
		if err := conn.QueryRow(ctx, `SELECT
			(SELECT count(*) FROM pricing.zones),
			(SELECT count(*) FROM pricing.pricing_rules),
			(SELECT count(*) FROM pricing.weight_bands),
			(SELECT count(*) FROM pricing.size_surcharges)`).Scan(&zones, &rules, &bands, &surcharges); err != nil {
			t.Fatal(err)
		}
		if zones != 5 || rules != 25 || bands != 4 || surcharges != 4 {
			t.Fatalf("counts after reapply = %d/%d/%d/%d", zones, rules, bands, surcharges)
		}
	})

	origin := pricingMigrationZoneID(t, ctx, conn, "CMB")
	destination := pricingMigrationZoneID(t, ctx, conn, "KDY")

	t.Run("foreign key", func(t *testing.T) {
		_, err := conn.Exec(ctx, `INSERT INTO pricing.pricing_rules(origin_zone_id,destination_zone_id,base_fee)
			VALUES (gen_random_uuid(),gen_random_uuid(),1)`)
		expectPricingPgError(t, err, "23503", "")
	})

	t.Run("unique zone code", func(t *testing.T) {
		_, err := conn.Exec(ctx, `INSERT INTO pricing.zones(code,name) VALUES ('CMB','duplicate')`)
		expectPricingPgError(t, err, "23505", "zones_code_key")
	})

	t.Run("unique active route", func(t *testing.T) {
		_, err := conn.Exec(ctx, `INSERT INTO pricing.pricing_rules(origin_zone_id,destination_zone_id,base_fee)
			VALUES ($1,$2,1)`, origin, destination)
		expectPricingPgError(t, err, "23505", "pricing_rules_active_pair_uq")
	})

	t.Run("check constraints", func(t *testing.T) {
		_, err := conn.Exec(ctx, `INSERT INTO pricing.pricing_rules(origin_zone_id,destination_zone_id,base_fee)
			VALUES ($1,$2,-1)`, origin, destination)
		expectPricingPgError(t, err, "23514", "")
		_, err = conn.Exec(ctx, `INSERT INTO pricing.size_surcharges(size_category,surcharge) VALUES ('XL',1)`)
		expectPricingPgError(t, err, "23514", "")
		_, err = conn.Exec(ctx, `INSERT INTO pricing.weight_bands(min_weight_kg,max_weight_kg,charge) VALUES (5,5,1)`)
		expectPricingPgError(t, err, "23514", "weight_bands_range")
	})

	t.Run("overlap exclusion", func(t *testing.T) {
		_, err := conn.Exec(ctx, `INSERT INTO pricing.weight_bands(min_weight_kg,max_weight_kg,charge) VALUES (2,3,1)`)
		expectPricingPgError(t, err, "23P01", "weight_bands_no_overlap")

		if _, err := conn.Exec(ctx, `INSERT INTO pricing.weight_bands(min_weight_kg,max_weight_kg,charge) VALUES (25,30,1)`); err != nil {
			t.Fatalf("adjacent band rejected: %v", err)
		}
	})

	t.Run("rollback and reapply", func(t *testing.T) {
		applySQLFile(t, ctx, conn, "migrations/pricing/000002_tables.down.sql")
		for _, table := range []string{"zones", "pricing_rules", "weight_bands", "size_surcharges", "audit_logs"} {
			var exists bool
			if err := conn.QueryRow(ctx, "SELECT to_regclass($1) IS NOT NULL", "pricing."+table).Scan(&exists); err != nil {
				t.Fatal(err)
			}
			if exists {
				t.Fatalf("pricing.%s still exists after down migration", table)
			}
		}

		applySQLFile(t, ctx, conn, "migrations/pricing/000001_initial_schema.down.sql")
		var schemaExists bool
		if err := conn.QueryRow(ctx, "SELECT to_regnamespace('pricing') IS NOT NULL").Scan(&schemaExists); err != nil {
			t.Fatal(err)
		}
		if schemaExists {
			t.Fatal("pricing schema still exists after 000001 down migration")
		}

		applySQLFile(t, ctx, conn, "migrations/pricing/000001_initial_schema.up.sql")
		applySQLFile(t, ctx, conn, "migrations/pricing/000002_tables.up.sql")
		var zones int
		if err := conn.QueryRow(ctx, "SELECT count(*) FROM pricing.zones").Scan(&zones); err != nil || zones != 5 {
			t.Fatalf("zones after reapply = %d, err %v", zones, err)
		}
	})
}

func pricingMigrationZoneID(t *testing.T, ctx context.Context, conn *pgx.Conn, code string) string {
	t.Helper()
	var id string
	if err := conn.QueryRow(ctx, "SELECT id::text FROM pricing.zones WHERE code=$1", code).Scan(&id); err != nil {
		t.Fatalf("read zone %s: %v", code, err)
	}
	return id
}

func expectPricingPgError(t *testing.T, err error, code, constraint string) {
	t.Helper()
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		t.Fatalf("expected PostgreSQL error %s, got %v", code, err)
	}
	if pgErr.Code != code {
		t.Fatalf("error code = %s, want %s (%v)", pgErr.Code, code, err)
	}
	if constraint != "" && pgErr.ConstraintName != constraint {
		t.Fatalf("constraint = %q, want %q", pgErr.ConstraintName, constraint)
	}
}
