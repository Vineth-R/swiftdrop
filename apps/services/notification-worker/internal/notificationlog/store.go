// Package notificationlog provides the read side of the notification delivery
// log. It is intentionally separate from the idempotency store: this package
// only reads notification.notification_logs for the administrator API.
package notificationlog

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Vineth-R/swiftdrop/apps/services/notification-worker/internal/idempotency"
)

// MaxPageSize is the largest page the administrator list accepts.
const MaxPageSize = 100

// Stable, non-sensitive errors. Database driver messages are deliberately
// discarded so SQL text and connection details cannot reach logs or clients.
var (
	ErrUnavailable   = errors.New("notification log database unavailable")
	ErrInvalidFilter = errors.New("invalid notification log filter")
	ErrListFailed    = errors.New("notification log query failed")
)

// DB is the subset of the shared PostgreSQL pool this repository needs.
type DB interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

// Log is one notification delivery-log row.
type Log struct {
	ID           string
	EventID      string
	ParcelID     string
	Channel      string
	Recipient    string
	Subject      *string
	Status       string
	ErrorMessage *string
	CreatedAt    time.Time
	SentAt       *time.Time
}

// Filter selects one page of notification delivery logs.
type Filter struct {
	Status string
	Limit  int
	Offset int
}

// Repository reads notification delivery logs.
type Repository struct {
	db DB
}

// NewRepository returns a repository backed by db.
func NewRepository(db DB) *Repository {
	return &Repository{db: db}
}

// ValidStatus reports whether status is one of the delivery statuses accepted
// by notification.notification_logs.
func ValidStatus(status string) bool {
	switch status {
	case idempotency.StatusPending, idempotency.StatusSent, idempotency.StatusFailed:
		return true
	default:
		return false
	}
}

const logColumns = `id::text, event_id::text, parcel_id::text, channel, recipient,
	subject, status, error_message, created_at, sent_at`

// List returns one page of notification logs matching filter and the total
// number of matches. Results are newest first, with id as a deterministic
// tie-breaker.
func (r *Repository) List(ctx context.Context, filter Filter) ([]Log, int, error) {
	if r == nil || r.db == nil {
		return nil, 0, ErrUnavailable
	}
	if filter.Limit < 1 || filter.Limit > MaxPageSize || filter.Offset < 0 {
		return nil, 0, ErrInvalidFilter
	}

	where := ""
	args := []any{}
	if filter.Status != "" {
		if !ValidStatus(filter.Status) {
			return nil, 0, ErrInvalidFilter
		}
		args = append(args, filter.Status)
		where = " WHERE status = $1"
	}

	var total int
	if err := r.db.QueryRow(ctx,
		"SELECT count(*) FROM notification.notification_logs"+where,
		args...,
	).Scan(&total); err != nil {
		return nil, 0, ErrListFailed
	}

	pageArgs := make([]any, 0, len(args)+2)
	pageArgs = append(pageArgs, args...)
	pageArgs = append(pageArgs, filter.Limit, filter.Offset)
	query := "SELECT " + logColumns +
		" FROM notification.notification_logs" + where +
		" ORDER BY created_at DESC, id DESC LIMIT $" +
		strconv.Itoa(len(pageArgs)-1) + " OFFSET $" + strconv.Itoa(len(pageArgs))

	rows, err := r.db.Query(ctx, query, pageArgs...)
	if err != nil {
		return nil, 0, ErrListFailed
	}
	defer rows.Close()

	logs := []Log{}
	for rows.Next() {
		var entry Log
		if err := rows.Scan(
			&entry.ID, &entry.EventID, &entry.ParcelID, &entry.Channel, &entry.Recipient,
			&entry.Subject, &entry.Status, &entry.ErrorMessage, &entry.CreatedAt, &entry.SentAt,
		); err != nil {
			return nil, 0, ErrListFailed
		}
		logs = append(logs, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, ErrListFailed
	}
	return logs, total, nil
}
