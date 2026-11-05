//go:build integration && postgres

package reports

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Vineth-R/swiftdrop/tests/integration/parceltest"
)

const reportFixtureInsert = `INSERT INTO parcel.parcels (
	id, tracking_number, customer_user_id, sender_name, sender_phone,
	pickup_address, pickup_city, pickup_zone_id,
	recipient_name, recipient_phone, dropoff_address, dropoff_city, dropoff_zone_id,
	weight_kg, size_category, status, failure_reason,
	price_amount, price_breakdown, created_by_user_id,
	created_at, updated_at, delivered_at, failed_at, delivered_to_name
) VALUES (
	$1, $2, $3, 'Report Test', '+94770000000',
	'Pickup', 'Colombo', '00000000-0000-4000-8000-000000000001',
	'Recipient', '+94771111111', 'Dropoff', 'Kandy', '00000000-0000-4000-8000-000000000002',
	2.00, 'SMALL', $4, $5,
	550.00, '{"base_fee":450,"weight_charge":100,"size_surcharge":0}', $3,
	$6, $6, $7, $8, $9
)`

type reportParcel struct {
	status   string
	created  time.Time
	finished time.Time
	reason   string
}

func reportTime(day, hour int) time.Time {
	return time.Date(2026, time.March, day, hour, 0, 0, 0, time.UTC)
}

func insertReportParcel(t *testing.T, ctx context.Context, conn *pgx.Conn, sequence int, parcel reportParcel) {
	t.Helper()
	var deliveredAt, failedAt, deliveredTo, reason any
	switch parcel.status {
	case "DELIVERED":
		if parcel.finished.IsZero() {
			t.Fatal("delivered fixture needs a delivery time")
		}
		deliveredAt = parcel.finished
		deliveredTo = "Recipient"
	case "FAILED":
		if parcel.finished.IsZero() || parcel.reason == "" {
			t.Fatal("failed fixture needs a failure time and reason")
		}
		failedAt = parcel.finished
		reason = parcel.reason
	}
	_, err := conn.Exec(ctx, reportFixtureInsert,
		parceltest.UUID(t),
		fmt.Sprintf("SDK%08d", sequence),
		parceltest.UUID(t),
		parcel.status,
		reason,
		parcel.created,
		deliveredAt,
		failedAt,
		deliveredTo,
	)
	if err != nil {
		t.Fatalf("insert report parcel %d: %v", sequence, err)
	}
}

func newReportConnection(t *testing.T, ctx context.Context, database *parceltest.Database, timezone string) *pgx.Conn {
	t.Helper()
	conn, err := pgx.Connect(ctx, database.URL())
	if err != nil {
		t.Fatalf("connect to isolated report database: %v", err)
	}
	t.Cleanup(func() {
		_ = conn.Close(context.Background())
	})
	if _, err := conn.Exec(ctx, "SET TIME ZONE "+quoteLiteral(timezone)); err != nil {
		t.Fatalf("set report session timezone: %v", err)
	}
	return conn
}

func quoteLiteral(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func reportContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func explainPlan(t *testing.T, ctx context.Context, conn *pgx.Conn, query string, args ...any) string {
	t.Helper()
	var raw []byte
	if err := conn.QueryRow(ctx, "EXPLAIN (FORMAT JSON) "+query, args...).Scan(&raw); err != nil {
		t.Fatalf("explain report query: %v", err)
	}
	return string(raw)
}

func TestReportDailyUTCBoundariesWithNonUTCSession(t *testing.T) {
	database := parceltest.New(t)
	ctx := reportContext(t)
	conn := newReportConnection(t, ctx, database, "America/New_York")
	repository := NewRepository(conn)

	insertReportParcel(t, ctx, conn, 1, reportParcel{status: "CREATED", created: reportTime(1, 0)})
	insertReportParcel(t, ctx, conn, 2, reportParcel{status: "DELIVERED", created: reportTime(1, 23), finished: reportTime(2, 0)})
	insertReportParcel(t, ctx, conn, 3, reportParcel{status: "FAILED", created: reportTime(2, 12), finished: reportTime(3, 23), reason: "Boundary failure"})
	insertReportParcel(t, ctx, conn, 4, reportParcel{status: "FAILED", created: reportTime(1, 10), finished: reportTime(2, 10), reason: "Inside failure"})
	// Outside the inclusive range on both sides. A session-timezone shift would
	// accidentally include the earlier parcel or move the first returned date.
	insertReportParcel(t, ctx, conn, 5, reportParcel{status: "CREATED", created: time.Date(2026, time.February, 28, 23, 59, 0, 0, time.UTC)})
	insertReportParcel(t, ctx, conn, 6, reportParcel{status: "DELIVERED", created: time.Date(2026, time.March, 6, 0, 0, 0, 0, time.UTC), finished: time.Date(2026, time.March, 6, 1, 0, 0, 0, time.UTC)})

	rows, err := repository.Daily(ctx, Range{From: reportTime(1, 0), To: reportTime(4, 0)})
	if err != nil {
		t.Fatalf("daily report: %v", err)
	}
	want := []DailyRow{
		{Date: "2026-03-01", Created: 3, Delivered: 0, Failed: 0},
		{Date: "2026-03-02", Created: 1, Delivered: 1, Failed: 1},
		{Date: "2026-03-03", Created: 0, Delivered: 0, Failed: 1},
		{Date: "2026-03-04", Created: 0, Delivered: 0, Failed: 0},
	}
	if !reflect.DeepEqual(rows, want) {
		t.Fatalf("daily rows = %+v, want %+v", rows, want)
	}

	empty, err := repository.Daily(ctx, Range{From: reportTime(10, 0), To: reportTime(11, 0)})
	if err != nil {
		t.Fatalf("empty daily report: %v", err)
	}
	wantEmpty := []DailyRow{
		{Date: "2026-03-10"},
		{Date: "2026-03-11"},
	}
	if !reflect.DeepEqual(empty, wantEmpty) {
		t.Fatalf("empty daily rows = %+v, want %+v", empty, wantEmpty)
	}
}

func TestReportOutcomesUTCBoundariesAndFailureReasons(t *testing.T) {
	database := parceltest.New(t)
	ctx := reportContext(t)
	conn := newReportConnection(t, ctx, database, "America/New_York")
	repository := NewRepository(conn)

	sequence := 1
	insert := func(parcel reportParcel) {
		insertReportParcel(t, ctx, conn, sequence, parcel)
		sequence++
	}
	at := time.Date(2026, time.April, 5, 12, 0, 0, 0, time.UTC)

	for range 10 {
		insert(reportParcel{status: "DELIVERED", created: at, finished: time.Date(2026, time.June, 1, 0, 0, 0, 0, time.UTC)})
	}
	reasons := []string{
		"Recipient not available", "Recipient not available", "Recipient not available",
		"Incorrect address", "Incorrect address",
		"Refused by recipient", "Refused by recipient",
		"Address incomplete",
		"Damaged",
		"Other",
	}
	for _, reason := range reasons {
		insert(reportParcel{status: "FAILED", created: at, finished: time.Date(2026, time.April, 15, 0, 0, 0, 0, time.UTC), reason: reason})
	}
	insert(reportParcel{status: "IN_TRANSIT", created: at})
	insert(reportParcel{status: "CREATED", created: at})

	// Inclusive lower boundary (inside), inclusive upper boundary (inside), and
	// three failures just before the range. The asymmetry catches a one-day
	// shift in either direction.
	insert(reportParcel{status: "DELIVERED", created: time.Date(2026, time.April, 1, 0, 0, 0, 0, time.UTC), finished: at})
	insert(reportParcel{status: "FAILED", created: time.Date(2026, time.April, 10, 23, 59, 59, 0, time.UTC), finished: at, reason: "Upper boundary"})
	for range 3 {
		insert(reportParcel{status: "FAILED", created: time.Date(2026, time.March, 31, 23, 59, 59, 0, time.UTC), finished: at, reason: "Outside before"})
	}
	insert(reportParcel{status: "DELIVERED", created: time.Date(2026, time.April, 11, 0, 0, 0, 0, time.UTC), finished: at})

	result, err := repository.Outcomes(ctx, Range{From: time.Date(2026, time.April, 1, 0, 0, 0, 0, time.UTC), To: time.Date(2026, time.April, 10, 0, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatalf("outcomes report: %v", err)
	}
	if result.From != "2026-04-01" || result.To != "2026-04-10" {
		t.Fatalf("outcomes range = %s..%s, want 2026-04-01..2026-04-10", result.From, result.To)
	}
	wantTotals := Totals{Created: 24, Delivered: 11, Failed: 11, InProgress: 2}
	if result.Totals != wantTotals {
		t.Fatalf("outcomes totals = %+v, want %+v", result.Totals, wantTotals)
	}
	if result.SuccessRate != 50 || result.FailureRate != 50 {
		t.Fatalf("outcomes rates = %v/%v, want 50/50", result.SuccessRate, result.FailureRate)
	}
	wantReasons := []FailureReason{
		{Reason: "Recipient not available", Count: 3},
		{Reason: "Incorrect address", Count: 2},
		{Reason: "Refused by recipient", Count: 2},
		{Reason: "Address incomplete", Count: 1},
		{Reason: "Damaged", Count: 1},
	}
	if !reflect.DeepEqual(result.TopFailureReasons, wantReasons) {
		t.Fatalf("top failure reasons = %+v, want %+v", result.TopFailureReasons, wantReasons)
	}

	emptyRange, err := repository.Outcomes(ctx, Range{From: time.Date(2026, time.May, 1, 0, 0, 0, 0, time.UTC), To: time.Date(2026, time.May, 3, 0, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatalf("empty-range outcomes report: %v", err)
	}
	if emptyRange.Totals != (Totals{}) || emptyRange.SuccessRate != 0 || emptyRange.FailureRate != 0 || len(emptyRange.TopFailureReasons) != 0 {
		t.Fatalf("empty outcomes = %+v, want zero totals and no reasons", emptyRange)
	}
}

func TestReportOutcomesWithoutFailures(t *testing.T) {
	database := parceltest.New(t)
	ctx := reportContext(t)
	conn := newReportConnection(t, ctx, database, "UTC")
	repository := NewRepository(conn)

	inRange := time.Date(2026, time.July, 5, 12, 0, 0, 0, time.UTC)
	insertReportParcel(t, ctx, conn, 1, reportParcel{status: "DELIVERED", created: inRange, finished: time.Date(2026, time.July, 6, 0, 0, 0, 0, time.UTC)})
	insertReportParcel(t, ctx, conn, 2, reportParcel{status: "CREATED", created: inRange})

	result, err := repository.Outcomes(ctx, Range{From: time.Date(2026, time.July, 1, 0, 0, 0, 0, time.UTC), To: time.Date(2026, time.July, 10, 0, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatalf("outcomes report: %v", err)
	}
	if result.Totals != (Totals{Created: 2, Delivered: 1, InProgress: 1}) {
		t.Fatalf("totals = %+v, want 2 created, 1 delivered, 1 in progress", result.Totals)
	}
	if result.SuccessRate != 100 || result.FailureRate != 0 {
		t.Fatalf("rates = %v/%v, want 100/0", result.SuccessRate, result.FailureRate)
	}
	if len(result.TopFailureReasons) != 0 {
		t.Fatalf("top failure reasons = %+v, want none", result.TopFailureReasons)
	}
}

func TestReportNinetyDayRangeAndIndexUse(t *testing.T) {
	database := parceltest.New(t)
	ctx := reportContext(t)
	conn := newReportConnection(t, ctx, database, "UTC")
	repository := NewRepository(conn)

	_, err := conn.Exec(ctx, `INSERT INTO parcel.parcels (
		id, tracking_number, customer_user_id, sender_name, sender_phone,
		pickup_address, pickup_city, pickup_zone_id,
		recipient_name, recipient_phone, dropoff_address, dropoff_city, dropoff_zone_id,
		weight_kg, size_category, status, failure_reason,
		price_amount, price_breakdown, created_by_user_id,
		created_at, updated_at, delivered_at, failed_at, delivered_to_name
	)
	SELECT
		gen_random_uuid(), 'SDK' || lpad(n::text, 8, '0'),
		'00000000-0000-4000-8000-0000000000c1'::uuid,
		'Report Load', '+94770000000',
		'Pickup', 'Colombo', '00000000-0000-4000-8000-000000000001'::uuid,
		'Recipient', '+94771111111', 'Dropoff', 'Kandy', '00000000-0000-4000-8000-000000000002'::uuid,
		2.00, 'SMALL',
		CASE WHEN n % 10 = 0 THEN 'DELIVERED' WHEN n % 10 = 1 THEN 'FAILED' ELSE 'CREATED' END,
		CASE WHEN n % 10 = 1 THEN 'Load failure' END,
		550.00, '{"base_fee":450,"weight_charge":100,"size_surcharge":0}'::jsonb,
		'00000000-0000-4000-8000-0000000000c1'::uuid,
		base, base,
		CASE WHEN n % 10 = 0 THEN base + interval '1 hour' END,
		CASE WHEN n % 10 = 1 THEN base + interval '2 hours' END,
		CASE WHEN n % 10 = 0 THEN 'Recipient' END
	FROM (
		SELECT n, '2026-01-01T00:00:00Z'::timestamptz + ((n % 180) * interval '1 day') AS base
		FROM generate_series(1, 1800) AS n
	) AS source`)
	if err != nil {
		t.Fatalf("insert report load fixtures: %v", err)
	}
	if _, err := conn.Exec(ctx, "ANALYZE parcel.parcels"); err != nil {
		t.Fatalf("analyze report fixtures: %v", err)
	}

	from := time.Date(2026, time.March, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, time.May, 29, 0, 0, 0, 0, time.UTC)
	rows, err := repository.Daily(ctx, Range{From: from, To: to})
	if err != nil {
		t.Fatalf("90-day daily report: %v", err)
	}
	if len(rows) != 90 || rows[0].Date != "2026-03-01" || rows[89].Date != "2026-05-29" {
		t.Fatalf("90-day daily rows = %d (%s..%s), want 90 rows from 2026-03-01 to 2026-05-29", len(rows), rows[0].Date, rows[len(rows)-1].Date)
	}

	result, err := repository.Outcomes(ctx, Range{From: from, To: to})
	if err != nil {
		t.Fatalf("90-day outcomes report: %v", err)
	}
	if result.From != "2026-03-01" || result.To != "2026-05-29" || result.Totals.Created == 0 {
		t.Fatalf("90-day outcomes = %+v, want a populated range", result)
	}

	if _, err := conn.Exec(ctx, "SET enable_seqscan = off"); err != nil {
		t.Fatalf("disable sequential scans: %v", err)
	}
	defer func() {
		if _, err := conn.Exec(context.Background(), "RESET enable_seqscan"); err != nil {
			t.Errorf("reset sequential scans: %v", err)
		}
	}()

	fromArg, toArg := from.Format(dateLayout), to.Format(dateLayout)
	dailyPlan := explainPlan(t, ctx, conn, dailySQL, fromArg, toArg)
	for _, index := range []string{"parcels_created_idx", "parcels_delivered_idx", "parcels_failed_idx"} {
		if !strings.Contains(dailyPlan, index) {
			t.Fatalf("daily plan does not use %s: %s", index, dailyPlan)
		}
	}
	if !strings.Contains(dailyPlan, "Index Cond") {
		t.Fatalf("daily plan has no index range condition: %s", dailyPlan)
	}
	outcomePlan := explainPlan(t, ctx, conn, outcomeTotalsSQL, fromArg, toArg)
	if !strings.Contains(outcomePlan, "parcels_created_idx") {
		t.Fatalf("outcomes totals plan does not use parcels_created_idx: %s", outcomePlan)
	}
	reasonsPlan := explainPlan(t, ctx, conn, outcomeReasonsSQL, fromArg, toArg, TopFailureReasonLimit)
	if !strings.Contains(reasonsPlan, "parcels_status_created") {
		t.Fatalf("failure reasons plan does not use parcels_status_created: %s", reasonsPlan)
	}
}
