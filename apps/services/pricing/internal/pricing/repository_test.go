package pricing

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const originID = "11111111-1111-4111-8111-111111111111"
const destinationID = "22222222-2222-4222-8222-222222222222"

type testRow struct {
	values []any
	err    error
}

func (r testRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	if len(dest) != len(r.values) {
		return errors.New("unexpected scan width")
	}
	for index, value := range r.values {
		target := reflect.ValueOf(dest[index]).Elem()
		if value == nil {
			target.SetZero()
		} else {
			target.Set(reflect.ValueOf(value))
		}
	}
	return nil
}

type testRows struct {
	pgx.Rows
	data    [][]any
	at      int
	err     error
	scanErr error
	closed  bool
}

func (r *testRows) Next() bool { r.at++; return r.at <= len(r.data) }
func (r *testRows) Scan(dest ...any) error {
	return (testRow{values: r.data[r.at-1], err: r.scanErr}).Scan(dest...)
}
func (r *testRows) Close()     { r.closed = true }
func (r *testRows) Err() error { return r.err }

type testDB struct {
	row      func(context.Context, string, ...any) pgx.Row
	rows     *testRows
	query    string
	queryErr error
	tx       pgx.Tx
	beginErr error
}

func (d *testDB) QueryRow(ctx context.Context, query string, args ...any) pgx.Row {
	return d.row(ctx, query, args...)
}
func (d *testDB) Query(_ context.Context, query string, _ ...any) (pgx.Rows, error) {
	d.query = query
	return d.rows, d.queryErr
}
func (d *testDB) Begin(context.Context) (pgx.Tx, error) { return d.tx, d.beginErr }

type testTx struct {
	pgx.Tx
	row                              func(context.Context, string, ...any) pgx.Row
	auditErr, commitErr, rollbackErr error
	audited, committed, rolledBack   bool
}

func (tx *testTx) QueryRow(ctx context.Context, query string, args ...any) pgx.Row {
	return tx.row(ctx, query, args...)
}
func (tx *testTx) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	tx.audited = true
	return pgconn.NewCommandTag("INSERT 0 1"), tx.auditErr
}
func (tx *testTx) Commit(context.Context) error {
	if tx.commitErr == nil {
		tx.committed = true
	}
	return tx.commitErr
}
func (tx *testTx) Rollback(context.Context) error {
	tx.rolledBack = true
	if tx.committed {
		return pgx.ErrTxClosed
	}
	return tx.rollbackErr
}

func TestRepositoryQuote(t *testing.T) {
	active := true
	inactive := false
	base := "450.00"
	maximum := "25.00"
	weight := "100.00"
	size := "0.00"
	negative := "-1.00"
	for _, test := range []struct {
		name    string
		values  []any
		dbErr   error
		want    error
		invalid bool
	}{
		{name: "seed quote", values: []any{&active, &active, &base, &maximum, &weight, &size}},
		{name: "missing zone", values: []any{nil, &active, &base, &maximum, &weight, &size}, want: ErrZoneNotFound},
		{name: "inactive zone", values: []any{&inactive, &active, &base, &maximum, &weight, &size}, want: ErrZoneNotFound},
		{name: "unpriced route", values: []any{&active, &active, nil, &maximum, &weight, &size}, want: ErrRouteNotServiceable},
		{name: "gap", values: []any{&active, &active, &base, &maximum, nil, &size}, want: ErrWeightNotPriced},
		{name: "no bands", values: []any{&active, &active, &base, nil, nil, &size}, want: ErrWeightNotPriced},
		{name: "missing surcharge", values: []any{&active, &active, &base, &maximum, &weight, nil}, invalid: true},
		{name: "corrupt amount", values: []any{&active, &active, &negative, &maximum, &weight, &size}, invalid: true},
		{name: "database failure", dbErr: errors.New("fixture failure"), invalid: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			currency := "LKR"
			test.values = append(test.values, &currency)
			db := &testDB{row: func(context.Context, string, ...any) pgx.Row { return testRow{values: test.values, err: test.dbErr} }}
			quote, err := NewRepository(db).Quote(t.Context(), Input{OriginZoneID: originID, DestinationZoneID: destinationID, WeightKg: 200, SizeCategory: "SMALL"})
			if test.want != nil {
				if !errors.Is(err, test.want) {
					t.Fatalf("error %v want %v", err, test.want)
				}
				return
			}
			if test.invalid {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil || quote.Total != 55000 {
				t.Fatalf("quote %+v, error %v", quote, err)
			}
		})
	}
	t.Run("above maximum", func(t *testing.T) {
		db := &testDB{row: func(context.Context, string, ...any) pgx.Row {
			return testRow{values: []any{&active, &active, &base, &maximum, nil, &size, nil}}
		}}
		_, err := NewRepository(db).Quote(t.Context(), Input{OriginZoneID: originID, DestinationZoneID: destinationID, WeightKg: 2600, SizeCategory: "SMALL"})
		if !errors.Is(err, ErrWeightNotPriced) {
			t.Fatalf("error %v, want %v", err, ErrWeightNotPriced)
		}
	})
	t.Run("unpriced route takes precedence over weight", func(t *testing.T) {
		db := &testDB{row: func(context.Context, string, ...any) pgx.Row {
			return testRow{values: []any{&active, &active, nil, &maximum, nil, &size, nil}}
		}}
		_, err := NewRepository(db).Quote(t.Context(), Input{OriginZoneID: originID, DestinationZoneID: destinationID, WeightKg: 2600, SizeCategory: "SMALL"})
		if !errors.Is(err, ErrRouteNotServiceable) {
			t.Fatalf("error %v, want %v", err, ErrRouteNotServiceable)
		}
	})
}

func TestQuoteRejectsUnsupportedCurrency(t *testing.T) {
	active, base, maximum, weight, size, currency := true, "450.00", "25.00", "100.00", "0.00", "USD"
	db := &testDB{row: func(context.Context, string, ...any) pgx.Row {
		return testRow{values: []any{&active, &active, &base, &maximum, &weight, &size, &currency}}
	}}
	_, err := NewRepository(db).Quote(t.Context(), Input{OriginZoneID: originID, DestinationZoneID: destinationID, WeightKg: 200, SizeCategory: "SMALL"})
	if err == nil {
		t.Fatal("unsupported currency was mislabeled LKR")
	}
}
func TestRepositoryZones(t *testing.T) {
	for _, test := range []struct {
		name                       string
		queryErr, scanErr, rowsErr error
	}{
		{name: "active zones"}, {name: "query failure", queryErr: errors.New("query")},
		{name: "scan failure", scanErr: errors.New("scan")}, {name: "iteration failure", rowsErr: errors.New("iteration")},
	} {
		t.Run(test.name, func(t *testing.T) {
			rows := &testRows{data: [][]any{{originID, "CMB", "Colombo", []string{"Colombo"}, true}}, scanErr: test.scanErr, err: test.rowsErr}
			zones, err := NewRepository(&testDB{rows: rows, queryErr: test.queryErr}).Zones(t.Context())
			if test.queryErr != nil || test.scanErr != nil || test.rowsErr != nil {
				if err == nil {
					t.Fatal("expected database error")
				}
			} else if err != nil || len(zones) != 1 || zones[0].Code != "CMB" {
				t.Fatalf("zones %+v, err %v", zones, err)
			}
			if test.queryErr == nil && !rows.closed {
				t.Fatal("rows not closed")
			}
		})
	}
}
func TestRepositoryZone(t *testing.T) {
	for _, test := range []struct {
		name, id string
		rowErr   error
		want     error
	}{
		{name: "zone", id: originID}, {name: "missing", id: originID, rowErr: pgx.ErrNoRows, want: ErrZoneNotFound},
		{name: "failure", id: originID, rowErr: errors.New("database")}, {name: "bad id", id: "bad"},
	} {
		t.Run(test.name, func(t *testing.T) {
			db := &testDB{row: func(context.Context, string, ...any) pgx.Row {
				return testRow{values: []any{originID, "CMB", "Colombo", []string{}, true}, err: test.rowErr}
			}}
			zone, err := NewRepository(db).Zone(t.Context(), test.id)
			if test.want != nil {
				if !errors.Is(err, test.want) {
					t.Fatal(err)
				}
				return
			}
			if test.rowErr != nil || test.id == "bad" {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil || zone.ID != originID {
				t.Fatalf("zone %+v err %v", zone, err)
			}
		})
	}
}
func TestRepositoryMutateAtomicity(t *testing.T) {
	name := "Test zone"
	code := "TEST"
	for _, test := range []struct {
		name                          string
		auditErr, commitErr, writeErr error
		updating                      bool
	}{
		{name: "create and audit"}, {name: "update and audit", updating: true},
		{name: "audit failure", auditErr: errors.New("audit")}, {name: "commit failure", commitErr: errors.New("commit")},
		{name: "write failure", writeErr: &pgconn.PgError{Code: "23505", ConstraintName: "zones_code_key"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			tx := &testTx{auditErr: test.auditErr, commitErr: test.commitErr, row: func(context.Context, string, ...any) pgx.Row {
				calls++
				if test.updating && calls == 1 {
					return testRow{values: []any{originID}}
				}
				return testRow{values: []any{`{"id":"` + originID + `"}`}, err: test.writeErr}
			}}
			mutation := Mutation{Name: &name}
			id := originID
			if !test.updating {
				mutation.Code = &code
				id = ""
			}
			result, err := NewRepository(&testDB{tx: tx}).Mutate(t.Context(), "zones", id, originID, mutation)
			failed := test.auditErr != nil || test.commitErr != nil || test.writeErr != nil
			if failed {
				if err == nil || tx.committed {
					t.Fatalf("non atomic failed mutation: %v", err)
				}
			} else if err != nil || !json.Valid(result) || !tx.committed || !tx.audited {
				t.Fatalf("mutation failed: %v", err)
			}
			if !tx.rolledBack {
				t.Fatal("transaction cleanup was not attempted")
			}
		})
	}
}
func TestMapWriteError(t *testing.T) {
	for _, test := range []struct{ name, code, constraint, want string }{
		{name: "zone duplicate", code: "23505", constraint: "zones_code_key", want: "ZONE_CODE_EXISTS"},
		{name: "rule duplicate", code: "23505", constraint: "pricing_rules_active_pair_uq", want: "RULE_EXISTS"},
		{name: "band overlap", code: "23P01", constraint: "weight_bands_no_overlap", want: "BAND_OVERLAP"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var conflict *Conflict
			err := mapWriteError(&pgconn.PgError{Code: test.code, ConstraintName: test.constraint})
			if !errors.As(err, &conflict) || conflict.Code != test.want {
				t.Fatal(err)
			}
		})
	}
}
func TestRepositoryList(t *testing.T) {
	for _, test := range []struct {
		name, sort        string
		queryErr, scanErr error
	}{
		{name: "default"}, {name: "descending", sort: "id:desc"}, {name: "created", sort: "createdAt:desc"},
		{name: "updated", sort: "updatedAt:asc"}, {name: "invalid sort", sort: "unsafe"},
		{name: "query failure", queryErr: errors.New("query")}, {name: "scan failure", scanErr: errors.New("scan")},
	} {
		t.Run(test.name, func(t *testing.T) {
			rows := &testRows{data: [][]any{{`{"id":"` + originID + `"}`}}, scanErr: test.scanErr}
			db := &testDB{row: func(context.Context, string, ...any) pgx.Row { return testRow{values: []any{1}} }, rows: rows, queryErr: test.queryErr}
			active := true
			result, total, err := NewRepository(db).List(t.Context(), "rules", Filter{Page: 1, PageSize: 20, Origin: originID, Destination: destinationID, Active: &active, Sort: test.sort})
			if test.sort == "unsafe" || test.queryErr != nil || test.scanErr != nil {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil || total != 1 || len(result) != 1 || !rows.closed {
				t.Fatalf("list result %v %d %v", result, total, err)
			}
		})
	}
}
func TestRepositoryListResourceFilters(t *testing.T) {
	active := true
	for _, test := range []struct {
		name     string
		kind     string
		filter   Filter
		contains []string
		excludes []string
	}{
		{
			name:     "zones filter by active state only",
			kind:     "zones",
			filter:   Filter{Page: 1, PageSize: 20, Active: &active},
			contains: []string{"FROM pricing.zones", "is_active=$1"},
			excludes: []string{"origin_zone_id", "destination_zone_id"},
		},
		{
			name:     "rules keep zone and active filters",
			kind:     "rules",
			filter:   Filter{Page: 1, PageSize: 20, Origin: originID, Destination: destinationID, Active: &active},
			contains: []string{"FROM pricing.pricing_rules", "origin_zone_id=$1", "destination_zone_id=$2", "is_active=$3"},
		},
		{
			name:     "bands ignore active state",
			kind:     "bands",
			filter:   Filter{Page: 1, PageSize: 20, Active: &active},
			contains: []string{"FROM pricing.weight_bands"},
			excludes: []string{"is_active=$", "origin_zone_id", "destination_zone_id"},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			db := &testDB{
				row:  func(context.Context, string, ...any) pgx.Row { return testRow{values: []any{1}} },
				rows: &testRows{data: [][]any{{`{"id":"` + originID + `"}`}}},
			}
			result, total, err := NewRepository(db).List(t.Context(), test.kind, test.filter)
			if err != nil || total != 1 || len(result) != 1 {
				t.Fatalf("list result %v %d %v", result, total, err)
			}
			for _, want := range test.contains {
				if !strings.Contains(db.query, want) {
					t.Fatalf("query %q does not contain %q", db.query, want)
				}
			}
			for _, unwanted := range test.excludes {
				if strings.Contains(db.query, unwanted) {
					t.Fatalf("query %q contains %q", db.query, unwanted)
				}
			}
		})
	}
}

func TestUnavailableRepository(t *testing.T) {
	repo := NewRepository(nil)
	if _, err := repo.Zones(t.Context()); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	if _, err := repo.Zone(t.Context(), originID); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	if _, _, err := repo.List(t.Context(), "rules", Filter{Page: 1, PageSize: 20}); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
}
