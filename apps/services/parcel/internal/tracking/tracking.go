package tracking

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/jackc/pgx/v5"
)

var ErrNotFound = errors.New("tracking number not found")
var ErrInvalid = errors.New("invalid tracking number")
var ErrUnavailable = errors.New("tracking database unavailable")
var numberPattern = regexp.MustCompile(`^SD[KN][A-Z0-9]{8}$`)

type Event struct {
	Status string    `json:"status"`
	At     time.Time `json:"at"`
}
type Result struct {
	TrackingNumber string    `json:"trackingNumber"`
	Status         string    `json:"status"`
	PickupCity     string    `json:"pickupCity"`
	DropoffCity    string    `json:"dropoffCity"`
	LastUpdatedAt  time.Time `json:"lastUpdatedAt"`
	Timeline       []Event   `json:"timeline"`
}
type DB interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}
type Repository struct{ db DB }

func NewRepository(db DB) *Repository { return &Repository{db: db} }

// Lookup selects only the public allowlist in one statement/snapshot, including history.
func (r *Repository) Lookup(ctx context.Context, number string) (Result, error) {
	if !numberPattern.MatchString(number) {
		return Result{}, ErrInvalid
	}
	if r.db == nil {
		return Result{}, ErrUnavailable
	}
	result := Result{Timeline: []Event{}}
	var history []byte
	err := r.db.QueryRow(ctx, `
	 SELECT p.tracking_number,p.status,p.pickup_city,p.dropoff_city,p.updated_at,
	 coalesce((SELECT json_agg(json_build_object('status',h.new_status,'at',h.created_at)
	   ORDER BY h.created_at,h.id) FROM parcel.parcel_status_history h WHERE h.parcel_id=p.id),'[]'::json)
	 FROM parcel.parcels p WHERE p.tracking_number=$1`, number).Scan(
		&result.TrackingNumber, &result.Status, &result.PickupCity, &result.DropoffCity, &result.LastUpdatedAt, &history,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Result{}, ErrNotFound
	}
	if err != nil {
		return Result{}, fmt.Errorf("read public tracking snapshot: %w", err)
	}
	if err := decodeHistory(history, &result.Timeline); err != nil {
		return Result{}, err
	}
	return result, nil
}
