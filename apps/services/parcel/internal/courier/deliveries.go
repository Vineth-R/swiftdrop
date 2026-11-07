// Package courier provides assignment-scoped delivery reads for API v1.1 section 6.
package courier

import (
	"context"
	"errors"
	"fmt"
	"time"
	_ "time/tzdata" // Keep Asia/Colombo available on Windows and minimal images.

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/Vineth-R/swiftdrop/apps/services/parcel/internal/repository"
)

var (
	ErrNotFound    = errors.New("parcel not found")
	ErrForbidden   = errors.New("parcel is not assigned to this courier")
	ErrUnavailable = errors.New("courier deliveries are unavailable")
	// ErrInconsistentAssignment reports a parcel whose current courier has no
	// matching current assignment row. It is an internal data-consistency
	// failure, never a client-facing validation or authorization result.
	ErrInconsistentAssignment = errors.New("parcel has no current assignment for its current courier")
)

// Filter is validated at the HTTP boundary. End is exclusive.
type Filter struct {
	CourierID      string
	Start, End     time.Time
	Status         string
	Page, PageSize int
}

type Delivery struct {
	ID             string                  `json:"id"`
	TrackingNumber string                  `json:"trackingNumber"`
	Status         repository.ParcelStatus `json:"status"`
	PickupAddress  string                  `json:"pickupAddress"`
	DropoffAddress string                  `json:"dropoffAddress"`
	DropoffCity    string                  `json:"dropoffCity"`
	RecipientName  string                  `json:"recipientName"`
	RecipientPhone string                  `json:"recipientPhone"`
	WeightKG       float64                 `json:"weightKg"`
	AssignedAt     time.Time               `json:"assignedAt"`
}

type History struct {
	PreviousStatus *repository.ParcelStatus `json:"previousStatus"`
	Status         repository.ParcelStatus  `json:"status"`
	Note           *string                  `json:"note"`
	FailureReason  *string                  `json:"failureReason"`
	ActorRole      *string                  `json:"actorRole"`
	ChangedAt      time.Time                `json:"changedAt"`
}

type Proof struct {
	RecipientName *string   `json:"recipientName"`
	Note          *string   `json:"note"`
	HasPhoto      bool      `json:"hasPhoto"`
	CreatedAt     time.Time `json:"createdAt"`
}

type Detail struct {
	Delivery
	Description  *string                 `json:"description"`
	SizeCategory repository.SizeCategory `json:"sizeCategory"`
	SenderName   string                  `json:"senderName"`
	SenderPhone  string                  `json:"senderPhone"`
	History      []History               `json:"history"`
	Proof        *Proof                  `json:"proof"`
}

type Page struct {
	Items      []Delivery
	TotalItems int
}

type DB interface {
	Begin(context.Context) (pgx.Tx, error)
}

type Repository struct{ db DB }

func NewRepository(db DB) *Repository { return &Repository{db: db} }

// DayBounds returns a Colombo day as a half-open UTC range. An omitted date
// defaults to today in Colombo, independently of the host/session timezone.
func DayBounds(date string, now time.Time) (time.Time, time.Time, error) {
	zone, err := time.LoadLocation("Asia/Colombo")
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("load courier timezone: %w", err)
	}
	if date == "" {
		date = now.In(zone).Format(time.DateOnly)
	}
	start, err := time.ParseInLocation(time.DateOnly, date, zone)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("parse courier date: %w", err)
	}
	if start.Year() < 1 {
		return time.Time{}, time.Time{}, errors.New("courier date year must be positive")
	}
	return start.UTC(), start.AddDate(0, 0, 1).UTC(), nil
}

// beginRead gives ownership checks, count/items, history and proof one snapshot.
// A concurrent reassignment cannot mix data from different assignment states.
func (r *Repository) beginRead(ctx context.Context) (pgx.Tx, error) {
	if r.db == nil {
		return nil, ErrUnavailable
	}
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin courier read: %w", err)
	}
	if _, err := tx.Exec(ctx, "SET TRANSACTION ISOLATION LEVEL REPEATABLE READ, READ ONLY"); err != nil {
		return nil, errors.Join(fmt.Errorf("configure courier read: %w", err), closeRead(tx))
	}
	return tx, nil
}

func closeRead(tx pgx.Tx) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// Rollback only releases a read-only snapshot; pgx closes a broken connection.
	if err := tx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
		return fmt.Errorf("close courier read: %w", err)
	}
	return nil
}

const deliveryScope = `p.current_courier_id = $1
	AND (p.status NOT IN ('DELIVERED','FAILED')
		OR (p.status = 'DELIVERED' AND p.delivered_at >= $2 AND p.delivered_at < $3)
		OR (p.status = 'FAILED' AND p.failed_at >= $2 AND p.failed_at < $3))
	AND ($4::text = '' OR p.status = $4)`

// List uses the authoritative current courier copy maintained atomically by M6.
// The existing courier, completion-time and assignment indexes support this read.
func (r *Repository) List(ctx context.Context, filter Filter) (result Page, err error) {
	tx, err := r.beginRead(ctx)
	if err != nil {
		return Page{}, err
	}
	defer func() { err = errors.Join(err, closeRead(tx)) }()
	args := []any{filter.CourierID, filter.Start, filter.End, filter.Status}
	result = Page{Items: make([]Delivery, 0)}
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM parcel.parcels p WHERE `+deliveryScope, args...).Scan(&result.TotalItems); err != nil {
		return Page{}, fmt.Errorf("count courier deliveries: %w", err)
	}
	args = append(args, filter.PageSize, int64(filter.Page-1)*int64(filter.PageSize))
	rows, err := tx.Query(ctx, `SELECT p.id, p.tracking_number, p.status,
		p.pickup_address, p.dropoff_address, p.dropoff_city, p.recipient_name,
		p.recipient_phone, p.weight_kg::double precision, a.assigned_at
		FROM parcel.parcels p
		LEFT JOIN parcel.parcel_assignments a ON a.parcel_id = p.id
			AND a.is_current AND a.courier_id = p.current_courier_id
		WHERE `+deliveryScope+` ORDER BY a.assigned_at DESC NULLS LAST, p.id
		LIMIT $5 OFFSET $6`, args...)
	if err != nil {
		return Page{}, fmt.Errorf("query courier deliveries: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var item Delivery
		var assignedAt pgtype.Timestamptz
		if err := rows.Scan(&item.ID, &item.TrackingNumber, &item.Status,
			&item.PickupAddress, &item.DropoffAddress, &item.DropoffCity,
			&item.RecipientName, &item.RecipientPhone, &item.WeightKG, &assignedAt); err != nil {
			return Page{}, fmt.Errorf("scan courier delivery: %w", err)
		}
		if !assignedAt.Valid {
			return Page{}, fmt.Errorf("scan courier delivery %s: %w", item.ID, ErrInconsistentAssignment)
		}
		item.AssignedAt = assignedAt.Time.UTC()
		result.Items = append(result.Items, item)
	}
	if err := rows.Err(); err != nil {
		return Page{}, fmt.Errorf("iterate courier deliveries: %w", err)
	}
	return result, nil
}

func (r *Repository) Detail(ctx context.Context, courierID, parcelID string) (result Detail, err error) {
	tx, err := r.beginRead(ctx)
	if err != nil {
		return Detail{}, err
	}
	defer func() { err = errors.Join(err, closeRead(tx)) }()
	repo, err := repository.New(tx)
	if err != nil {
		return Detail{}, fmt.Errorf("create courier parcel reader: %w", err)
	}
	p, err := repo.ParcelByID(ctx, parcelID)
	if errors.Is(err, repository.ErrNotFound) {
		return Detail{}, ErrNotFound
	}
	if err != nil {
		return Detail{}, fmt.Errorf("read courier parcel: %w", err)
	}
	if p.CurrentCourierID == nil || *p.CurrentCourierID != courierID {
		return Detail{}, ErrForbidden
	}
	weight, err := p.WeightKG.Float64Value()
	if err != nil || !weight.Valid {
		return Detail{}, errors.New("invalid stored parcel weight")
	}
	result = Detail{
		Delivery: Delivery{ID: p.ID, TrackingNumber: p.TrackingNumber, Status: p.Status,
			PickupAddress: p.PickupAddress, DropoffAddress: p.DropoffAddress, DropoffCity: p.DropoffCity,
			RecipientName: p.RecipientName, RecipientPhone: p.RecipientPhone, WeightKG: weight.Float64},
		Description: p.Description, SizeCategory: p.SizeCategory, SenderName: p.SenderName,
		SenderPhone: p.SenderPhone, History: make([]History, 0),
	}
	var assignedAt pgtype.Timestamptz
	err = tx.QueryRow(ctx, `SELECT assigned_at FROM parcel.parcel_assignments
		WHERE parcel_id = $1 AND courier_id = $2 AND is_current`, parcelID, courierID).Scan(&assignedAt)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !assignedAt.Valid) {
		return Detail{}, fmt.Errorf("read courier assignment: %w", ErrInconsistentAssignment)
	}
	if err != nil {
		return Detail{}, fmt.Errorf("read courier assignment: %w", err)
	}
	result.AssignedAt = assignedAt.Time.UTC()
	history, err := repo.StatusHistoryByParcel(ctx, parcelID)
	if err != nil {
		return Detail{}, fmt.Errorf("read courier history: %w", err)
	}
	for _, entry := range history {
		result.History = append(result.History, History{PreviousStatus: entry.PreviousStatus,
			Status: entry.NewStatus, Note: entry.Note, FailureReason: entry.FailureReason,
			ActorRole: entry.ActorRole, ChangedAt: entry.CreatedAt.UTC()})
	}
	var proof Proof
	err = tx.QueryRow(ctx, `SELECT recipient_name, note, photo_path IS NOT NULL, created_at
		FROM parcel.delivery_proofs WHERE parcel_id = $1 ORDER BY created_at DESC, id DESC LIMIT 1`, parcelID).
		Scan(&proof.RecipientName, &proof.Note, &proof.HasPhoto, &proof.CreatedAt)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return Detail{}, fmt.Errorf("read courier proof: %w", err)
	}
	if err == nil {
		proof.CreatedAt = proof.CreatedAt.UTC()
		result.Proof = &proof
	}
	return result, nil
}
