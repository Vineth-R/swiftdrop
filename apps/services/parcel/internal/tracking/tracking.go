package tracking

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
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

const trackingSnapshot = `
	 SELECT p.tracking_number,p.status,p.pickup_city,p.dropoff_city,p.updated_at,
	 coalesce((SELECT json_agg(json_build_object('status',h.new_status,'at',h.created_at)
	   ORDER BY h.created_at,h.id) FROM parcel.parcel_status_history h WHERE h.parcel_id=p.id),'[]'::json)
	 FROM parcel.parcels p WHERE `

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
	err := r.db.QueryRow(ctx, trackingSnapshot+`p.tracking_number=$1`, number).Scan(
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

// LookupAuthorized returns the same allowlisted snapshot, but ownership is
// checked in the lookup itself. A customer only matches a parcel whose
// customer_user_id is theirs; staff (DISPATCHER/ADMIN) match any parcel for
// operational access. A missing parcel and another customer's parcel both
// return ErrNotFound so the caller cannot enumerate tracking numbers.
func (r *Repository) LookupAuthorized(ctx context.Context, number, customerUserID string, staff bool) (Result, error) {
	if !numberPattern.MatchString(number) {
		return Result{}, ErrInvalid
	}
	if !staff && strings.TrimSpace(customerUserID) == "" {
		return Result{}, ErrNotFound
	}
	if r.db == nil {
		return Result{}, ErrUnavailable
	}
	result := Result{Timeline: []Event{}}
	var history []byte
	err := r.db.QueryRow(ctx, trackingSnapshot+`p.tracking_number=$1 AND ($3 OR p.customer_user_id::text=$2)`,
		number, customerUserID, staff).Scan(
		&result.TrackingNumber, &result.Status, &result.PickupCity, &result.DropoffCity, &result.LastUpdatedAt, &history,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Result{}, ErrNotFound
	}
	if err != nil {
		return Result{}, fmt.Errorf("read authorized tracking snapshot: %w", err)
	}
	if err := decodeHistory(history, &result.Timeline); err != nil {
		return Result{}, err
	}
	return result, nil
}
