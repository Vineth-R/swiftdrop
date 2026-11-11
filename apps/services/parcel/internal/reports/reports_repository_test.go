package reports

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

// fakeRows is a pgx.Rows that serves canned rows. The embedded interface is nil;
// only the methods the repository calls are implemented.
type fakeRows struct {
	pgx.Rows
	data    [][]any
	at      int
	scanErr error
	err     error
	closed  bool
}

func (r *fakeRows) Next() bool { r.at++; return r.at <= len(r.data) }
func (r *fakeRows) Close()     { r.closed = true }
func (r *fakeRows) Err() error { return r.err }
func (r *fakeRows) Scan(dest ...any) error {
	if r.scanErr != nil {
		return r.scanErr
	}
	return assign(dest, r.data[r.at-1])
}

// fakeRow is a pgx.Row that serves one canned row, or an error.
type fakeRow struct {
	values []any
	err    error
}

func (r fakeRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	return assign(dest, r.values)
}

// assign copies string and int values into the pointers Scan was given.
func assign(dest []any, values []any) error {
	if len(dest) != len(values) {
		return errors.New("column count mismatch")
	}
	for i, d := range dest {
		switch target := d.(type) {
		case *string:
			*target = values[i].(string)
		case *int:
			*target = values[i].(int)
		default:
			return errors.New("unsupported scan target")
		}
	}
	return nil
}

type fakeDB struct {
	rows     map[string]*fakeRows // by a fragment of the SQL
	row      fakeRow
	queryErr error
	queries  []string
	args     [][]any
}

func (d *fakeDB) Query(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
	d.queries = append(d.queries, sql)
	d.args = append(d.args, args)
	if d.queryErr != nil {
		return nil, d.queryErr
	}
	for fragment, rows := range d.rows {
		if strings.Contains(sql, fragment) {
			return rows, nil
		}
	}
	return &fakeRows{}, nil
}

func (d *fakeDB) QueryRow(_ context.Context, sql string, args ...any) pgx.Row {
	d.queries = append(d.queries, sql)
	d.args = append(d.args, args)
	return d.row
}

var testRange = Range{From: now.AddDate(0, 0, -2), To: now}

func TestDailyReturnsEveryRowAndPassesISODates(t *testing.T) {
	rows := &fakeRows{data: [][]any{
		{"2026-11-03", 4, 2, 0},
		{"2026-11-04", 0, 0, 0},
		{"2026-11-05", 7, 3, 1},
	}}
	db := &fakeDB{rows: map[string]*fakeRows{"generate_series": rows}}

	got, err := NewRepository(db).Daily(t.Context(), testRange)
	if err != nil {
		t.Fatalf("Daily: %v", err)
	}
	want := []DailyRow{
		{Date: "2026-11-03", Created: 4, Delivered: 2, Failed: 0},
		{Date: "2026-11-04"},
		{Date: "2026-11-05", Created: 7, Delivered: 3, Failed: 1},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("rows = %+v, want %+v", got, want)
	}
	if !rows.closed {
		t.Fatal("rows were not closed")
	}
	if !reflect.DeepEqual(db.args[0], []any{"2026-11-03", "2026-11-05"}) {
		t.Fatalf("args = %v, want the ISO dates", db.args[0])
	}
}

func TestDailyReturnsAnEmptyListNotNil(t *testing.T) {
	db := &fakeDB{rows: map[string]*fakeRows{"generate_series": {}}}
	got, err := NewRepository(db).Daily(t.Context(), testRange)
	if err != nil {
		t.Fatalf("Daily: %v", err)
	}
	if got == nil || len(got) != 0 {
		t.Fatalf("got %#v, want an empty non-nil slice", got)
	}
}

func TestDailyErrors(t *testing.T) {
	boom := errors.New("boom")
	cases := map[string]*fakeDB{
		"query fails": {queryErr: boom},
		"scan fails": {rows: map[string]*fakeRows{
			"generate_series": {data: [][]any{{"2026-11-03", 1, 1, 1}}, scanErr: boom},
		}},
		"reading rows fails": {rows: map[string]*fakeRows{
			"generate_series": {err: boom},
		}},
	}
	for name, db := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := NewRepository(db).Daily(t.Context(), testRange); !errors.Is(err, boom) {
				t.Fatalf("error = %v, want it to wrap %v", err, boom)
			}
		})
	}
}

func TestOutcomesCombinesTotalsRatesAndReasons(t *testing.T) {
	reasons := &fakeRows{data: [][]any{
		{"RECIPIENT_UNAVAILABLE", 12},
		{"ADDRESS_NOT_FOUND", 9},
	}}
	db := &fakeDB{
		row:  fakeRow{values: []any{410, 352, 21}},
		rows: map[string]*fakeRows{"failure_reason": reasons},
	}

	got, err := NewRepository(db).Outcomes(t.Context(), testRange)
	if err != nil {
		t.Fatalf("Outcomes: %v", err)
	}
	want := Outcomes{
		From:        "2026-11-03",
		To:          "2026-11-05",
		Totals:      Totals{Created: 410, Delivered: 352, Failed: 21, InProgress: 37},
		SuccessRate: 94.37,
		FailureRate: 5.63,
		TopFailureReasons: []FailureReason{
			{Reason: "RECIPIENT_UNAVAILABLE", Count: 12},
			{Reason: "ADDRESS_NOT_FOUND", Count: 9},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("outcomes = %+v, want %+v", got, want)
	}
	if !reasons.closed {
		t.Fatal("reason rows were not closed")
	}
	// The reasons query is limited to the top five.
	last := db.args[len(db.args)-1]
	if last[len(last)-1] != TopFailureReasonLimit {
		t.Fatalf("limit arg = %v, want %d", last[len(last)-1], TopFailureReasonLimit)
	}
}

func TestOutcomesWithNoParcelsHasEmptyReasonsAndZeroRates(t *testing.T) {
	db := &fakeDB{row: fakeRow{values: []any{0, 0, 0}}}
	got, err := NewRepository(db).Outcomes(t.Context(), testRange)
	if err != nil {
		t.Fatalf("Outcomes: %v", err)
	}
	if got.SuccessRate != 0 || got.FailureRate != 0 || got.Totals != (Totals{}) {
		t.Fatalf("got %+v, want all zero", got)
	}
	if got.TopFailureReasons == nil || len(got.TopFailureReasons) != 0 {
		t.Fatalf("reasons = %#v, want an empty non-nil slice", got.TopFailureReasons)
	}
}

func TestOutcomesErrors(t *testing.T) {
	boom := errors.New("boom")
	cases := map[string]*fakeDB{
		"totals query fails": {row: fakeRow{err: boom}},
		"reasons query fails": {
			row:      fakeRow{values: []any{1, 0, 1}},
			queryErr: boom,
		},
		"reason scan fails": {
			row: fakeRow{values: []any{1, 0, 1}},
			rows: map[string]*fakeRows{
				"failure_reason": {data: [][]any{{"X", 1}}, scanErr: boom},
			},
		},
		"reading reasons fails": {
			row:  fakeRow{values: []any{1, 0, 1}},
			rows: map[string]*fakeRows{"failure_reason": {err: boom}},
		},
	}
	for name, db := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := NewRepository(db).Outcomes(t.Context(), testRange); !errors.Is(err, boom) {
				t.Fatalf("error = %v, want it to wrap %v", err, boom)
			}
		})
	}
}

func TestRangeDateArgsAreISODates(t *testing.T) {
	from, to := testRange.dateArgs()
	if from != "2026-11-03" || to != "2026-11-05" {
		t.Fatalf("dateArgs = %s, %s", from, to)
	}
}
