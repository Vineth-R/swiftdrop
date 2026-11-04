// Package reports reads the ADMIN delivery reports (API v1.1 section 7).
//
// All dates are UTC calendar dates. A range is inclusive on both ends: from
// 2026-11-01 to 2026-11-03 covers three days.
package reports

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Vineth-R/swiftdrop/packages/go-common/validation"
)

const (
	// DefaultDailyDays is the window GET /reports/daily uses when from and to are omitted.
	DefaultDailyDays = 7
	// DefaultOutcomeDays is the window GET /reports/outcomes uses when from and to are omitted.
	DefaultOutcomeDays = 30
	// MaxRangeDays is the longest range a caller may ask for.
	MaxRangeDays = 90
	// TopFailureReasonLimit caps topFailureReasons.
	TopFailureReasonLimit = 5

	dateLayout = "2006-01-02"
)

// ErrUnavailable is returned when no database is configured.
var ErrUnavailable = errors.New("reports database unavailable")

// Range is an inclusive pair of UTC dates, each at midnight.
type Range struct {
	From time.Time
	To   time.Time
}

// ParseRange reads the from and to query values. Empty values fall back to the
// last defaultDays days ending today (UTC). Problems are added to issues and
// ok is false. The range may not run backwards or be longer than MaxRangeDays.
func ParseRange(issues *validation.Collector, fromRaw, toRaw string, defaultDays int, now time.Time) (Range, bool) {
	before := issues.Len()
	today := midnightUTC(now)

	to := today
	if toRaw != "" {
		parsed, err := time.Parse(dateLayout, toRaw)
		if err != nil {
			issues.Add("to", "must be a date in YYYY-MM-DD format")
		} else {
			to = parsed
		}
	}

	from := to.AddDate(0, 0, -(defaultDays - 1))
	if fromRaw != "" {
		parsed, err := time.Parse(dateLayout, fromRaw)
		if err != nil {
			issues.Add("from", "must be a date in YYYY-MM-DD format")
		} else {
			from = parsed
		}
	}

	if issues.Len() != before {
		return Range{}, false
	}
	if from.After(to) {
		issues.Add("to", "must not be before from")
		return Range{}, false
	}
	days := int(to.Sub(from).Hours()/24) + 1
	if days > MaxRangeDays {
		issues.Add("to", fmt.Sprintf("range must not be longer than %d days", MaxRangeDays))
		return Range{}, false
	}
	return Range{From: from, To: to}, true
}

func midnightUTC(t time.Time) time.Time {
	u := t.UTC()
	return time.Date(u.Year(), u.Month(), u.Day(), 0, 0, 0, 0, time.UTC)
}

// DailyRow is one day of GET /reports/daily.
type DailyRow struct {
	Date      string `json:"date"`
	Created   int    `json:"created"`
	Delivered int    `json:"delivered"`
	Failed    int    `json:"failed"`
}

// Totals counts the parcels created in the range by where they are now.
type Totals struct {
	Created    int `json:"created"`
	Delivered  int `json:"delivered"`
	Failed     int `json:"failed"`
	InProgress int `json:"inProgress"`
}

// FailureReason is one entry of topFailureReasons.
type FailureReason struct {
	Reason string `json:"reason"`
	Count  int    `json:"count"`
}

// Outcomes is the body of GET /reports/outcomes.
type Outcomes struct {
	From              string          `json:"from"`
	To                string          `json:"to"`
	Totals            Totals          `json:"totals"`
	SuccessRate       float64         `json:"successRate"`
	FailureRate       float64         `json:"failureRate"`
	TopFailureReasons []FailureReason `json:"topFailureReasons"`
}

// Summarise turns raw counts into totals and the two rates. Rates are
// percentages of finished parcels (delivered plus failed), rounded to two
// decimals. With no finished parcels both rates are 0.
func Summarise(created, delivered, failed int) (Totals, float64, float64) {
	totals := Totals{
		Created:    created,
		Delivered:  delivered,
		Failed:     failed,
		InProgress: created - delivered - failed,
	}
	finished := delivered + failed
	return totals, rate(delivered, finished), rate(failed, finished)
}

func rate(part, whole int) float64 {
	if whole == 0 {
		return 0
	}
	return math.Round(float64(part)/float64(whole)*10000) / 100
}

// DB is the part of the connection pool the repository needs.
type DB interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

// Repository runs the report queries.
type Repository struct{ db DB }

// NewRepository returns a repository. A nil db makes every call fail with ErrUnavailable.
func NewRepository(db DB) *Repository { return &Repository{db: db} }

// Daily counts parcels created, delivered and failed on each day of the range.
// Created uses created_at, delivered uses delivered_at and failed uses failed_at,
// so a parcel created on Monday and delivered on Tuesday counts on both days.
const dailySQL = `
WITH days AS (
    SELECT generate_series($1::date, $2::date, interval '1 day')::date AS day
),
created AS (
    SELECT (created_at AT TIME ZONE 'UTC')::date AS day, count(*)::int AS n
    FROM parcel.parcels
    WHERE created_at >= ($1::date)::timestamp AT TIME ZONE 'UTC'
      AND created_at <  (($2::date) + 1)::timestamp AT TIME ZONE 'UTC'
    GROUP BY 1
),
delivered AS (
    SELECT (delivered_at AT TIME ZONE 'UTC')::date AS day, count(*)::int AS n
    FROM parcel.parcels
    WHERE delivered_at >= ($1::date)::timestamp AT TIME ZONE 'UTC'
      AND delivered_at <  (($2::date) + 1)::timestamp AT TIME ZONE 'UTC'
    GROUP BY 1
),
failed AS (
    SELECT (failed_at AT TIME ZONE 'UTC')::date AS day, count(*)::int AS n
    FROM parcel.parcels
    WHERE failed_at >= ($1::date)::timestamp AT TIME ZONE 'UTC'
      AND failed_at <  (($2::date) + 1)::timestamp AT TIME ZONE 'UTC'
    GROUP BY 1
)
SELECT to_char(d.day, 'YYYY-MM-DD'),
       coalesce(c.n, 0),
       coalesce(v.n, 0),
       coalesce(f.n, 0)
FROM days d
LEFT JOIN created   c ON c.day = d.day
LEFT JOIN delivered v ON v.day = d.day
LEFT JOIN failed    f ON f.day = d.day
ORDER BY d.day`

// Daily returns one row per day in the range, including days with no parcels.
func (r *Repository) Daily(ctx context.Context, rng Range) ([]DailyRow, error) {
	if r.db == nil {
		return nil, ErrUnavailable
	}
	rows, err := r.db.Query(ctx, dailySQL, rng.From, rng.To)
	if err != nil {
		return nil, fmt.Errorf("query daily report: %w", err)
	}
	defer rows.Close()

	result := make([]DailyRow, 0, MaxRangeDays)
	for rows.Next() {
		var row DailyRow
		if err := rows.Scan(&row.Date, &row.Created, &row.Delivered, &row.Failed); err != nil {
			return nil, fmt.Errorf("scan daily report: %w", err)
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read daily report: %w", err)
	}
	return result, nil
}

// Outcomes follows the parcels created in the range and counts where each one
// is now, so created = delivered + failed + inProgress always holds.
const outcomeTotalsSQL = `
SELECT count(*)::int,
       (count(*) FILTER (WHERE status = 'DELIVERED'))::int,
       (count(*) FILTER (WHERE status = 'FAILED'))::int
FROM parcel.parcels
WHERE created_at >= ($1::date)::timestamp AT TIME ZONE 'UTC'
  AND created_at <  (($2::date) + 1)::timestamp AT TIME ZONE 'UTC'`

const outcomeReasonsSQL = `
SELECT failure_reason, count(*)::int
FROM parcel.parcels
WHERE status = 'FAILED'
  AND failure_reason IS NOT NULL
  AND created_at >= ($1::date)::timestamp AT TIME ZONE 'UTC'
  AND created_at <  (($2::date) + 1)::timestamp AT TIME ZONE 'UTC'
GROUP BY failure_reason
ORDER BY count(*) DESC, failure_reason
LIMIT $3`

// Outcomes returns the success and failure rates and the most common failure reasons.
func (r *Repository) Outcomes(ctx context.Context, rng Range) (Outcomes, error) {
	if r.db == nil {
		return Outcomes{}, ErrUnavailable
	}
	var created, delivered, failed int
	if err := r.db.QueryRow(ctx, outcomeTotalsSQL, rng.From, rng.To).Scan(&created, &delivered, &failed); err != nil {
		return Outcomes{}, fmt.Errorf("query outcome totals: %w", err)
	}

	rows, err := r.db.Query(ctx, outcomeReasonsSQL, rng.From, rng.To, TopFailureReasonLimit)
	if err != nil {
		return Outcomes{}, fmt.Errorf("query failure reasons: %w", err)
	}
	defer rows.Close()

	reasons := make([]FailureReason, 0, TopFailureReasonLimit)
	for rows.Next() {
		var item FailureReason
		if err := rows.Scan(&item.Reason, &item.Count); err != nil {
			return Outcomes{}, fmt.Errorf("scan failure reason: %w", err)
		}
		reasons = append(reasons, item)
	}
	if err := rows.Err(); err != nil {
		return Outcomes{}, fmt.Errorf("read failure reasons: %w", err)
	}

	totals, success, failure := Summarise(created, delivered, failed)
	return Outcomes{
		From:              rng.From.Format(dateLayout),
		To:                rng.To.Format(dateLayout),
		Totals:            totals,
		SuccessRate:       success,
		FailureRate:       failure,
		TopFailureReasons: reasons,
	}, nil
}
