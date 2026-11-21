package seed

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// DB is the transaction surface the seed needs. *postgres.Pool satisfies it.
type DB interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

// Result reports what the seed created or skipped.
type Result struct {
	Created  int
	Skipped  int
	Statuses []string
}

type statuses struct {
	customerID    string
	courierOneID  string
	courierTwoID  string
	dispatcherID  string
	pickupZoneID  string
	dropoffZoneID string
}

type parcelSpec struct {
	status        string
	weightKG      float64
	sizeCategory  string
	description   string
	delivered     bool
	failed        bool
	useCourierTwo bool
}

var demoParcels = []parcelSpec{
	{status: "CREATED", weightKG: 1.0, sizeCategory: "SMALL", description: "Demo parcel in CREATED"},
	{status: "PICKED_UP", weightKG: 1.5, sizeCategory: "SMALL", description: "Demo parcel in PICKED_UP", useCourierTwo: true},
	{status: "IN_TRANSIT", weightKG: 2.0, sizeCategory: "MEDIUM", description: "Demo parcel in IN_TRANSIT"},
	{status: "OUT_FOR_DELIVERY", weightKG: 2.5, sizeCategory: "MEDIUM", description: "Demo parcel in OUT_FOR_DELIVERY", useCourierTwo: true},
	{status: "DELIVERED", weightKG: 3.0, sizeCategory: "LARGE", description: "Demo parcel in DELIVERED", delivered: true},
	{status: "FAILED", weightKG: 3.5, sizeCategory: "LARGE", description: "Demo parcel in FAILED", failed: true, useCourierTwo: true},
}

// Run inserts one deterministic parcel per canonical status. Reruns are
// idempotent: an existing tracking number is skipped and its rows are not
// modified. The seed writes only the parcel schema; identity users and
// pricing zones are read-only seed inputs.
func Run(ctx context.Context, db DB, config Config) (Result, error) {
	if db == nil {
		return Result{}, errors.New("parcel demo seed requires a database")
	}
	if err := config.Validate(); err != nil {
		return Result{}, err
	}

	tx, err := db.Begin(ctx)
	if err != nil {
		return Result{}, fmt.Errorf("begin parcel seed transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	ids, err := resolveDemoIDs(ctx, tx, config)
	if err != nil {
		return Result{}, err
	}

	result := Result{Statuses: make([]string, 0, len(demoParcels))}
	base := time.Now().UTC().Add(-24 * time.Hour)
	for index, spec := range demoParcels {
		tracking := fmt.Sprintf("%s%07d", config.TrackingPrefix, index+1)
		createdAt := base.Add(time.Duration(index) * time.Minute)
		courierID := ""
		if spec.status != "CREATED" {
			courierID = ids.courierOneID
			if spec.useCourierTwo {
				courierID = ids.courierTwoID
			}
		}

		var parcelID string
		err := tx.QueryRow(ctx, `
			INSERT INTO parcel.parcels (
				tracking_number, customer_user_id, sender_name, sender_phone,
				pickup_address, pickup_city, pickup_zone_id,
				recipient_name, recipient_phone, dropoff_address, dropoff_city, dropoff_zone_id,
				weight_kg, size_category, description, status, current_courier_id,
				price_amount, currency, price_breakdown, created_by_user_id, created_at,
				delivered_at, delivered_to_name, failed_at, failure_reason)
			VALUES (
				$1, $2, 'Demo Customer', '+94770000001',
				'1 Demo Street', 'Colombo', $3,
				'Demo Recipient', '+94770000002', '2 Demo Avenue', 'Kandy', $4,
				$5, $6, $7, $8, $9,
				450.00, 'LKR', '{"base_fee":300.00,"weight_charge":100.00,"size_surcharge":50.00}'::jsonb,
				$2, $10, $11, $12, $13, $14)
			ON CONFLICT (tracking_number) DO NOTHING
			RETURNING id::text`,
			tracking, ids.customerID, ids.pickupZoneID, ids.dropoffZoneID,
			spec.weightKG, spec.sizeCategory, spec.description, spec.status, nullable(courierID),
			createdAt,
			nullableTime(createdAt.Add(4*time.Hour), spec.delivered),
			nullableString("Demo Recipient", spec.delivered),
			nullableTime(createdAt.Add(3*time.Hour), spec.failed),
			nullableString("Demo failure reason", spec.failed),
		).Scan(&parcelID)
		if errors.Is(err, pgx.ErrNoRows) {
			result.Skipped++
			continue
		}
		if err != nil {
			return Result{}, fmt.Errorf("insert demo parcel %s: %w", tracking, err)
		}
		result.Created++
		result.Statuses = append(result.Statuses, spec.status)

		if err := insertHistory(ctx, tx, parcelID, spec, ids, createdAt, courierID); err != nil {
			return Result{}, err
		}
		if spec.status != "CREATED" {
			if _, err := tx.Exec(ctx, `
				INSERT INTO parcel.parcel_assignments
					(id, parcel_id, courier_id, assigned_by, assigned_at, is_current)
				VALUES (gen_random_uuid(), $1, $2, $3, $4, true)`,
				parcelID, courierID, ids.dispatcherID, createdAt.Add(30*time.Minute),
			); err != nil {
				return Result{}, fmt.Errorf("assign demo parcel %s: %w", tracking, err)
			}
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return Result{}, fmt.Errorf("commit parcel seed transaction: %w", err)
	}
	return result, nil
}

func resolveDemoIDs(ctx context.Context, tx pgx.Tx, config Config) (statuses, error) {
	var ids statuses
	if err := tx.QueryRow(ctx,
		`SELECT id::text FROM identity.users WHERE lower(email) = lower($1)`, config.CustomerEmail,
	).Scan(&ids.customerID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return statuses{}, errors.New("SEED_CUSTOMER_EMAIL does not match an identity user; run make seed first")
		}
		return statuses{}, fmt.Errorf("read demo customer: %w", err)
	}
	for email, target := range map[string]*string{
		config.CourierOneEmail: &ids.courierOneID,
		config.CourierTwoEmail: &ids.courierTwoID,
	} {
		if err := tx.QueryRow(ctx, `
			SELECT u.id::text FROM identity.users u
			JOIN identity.courier_profiles p ON p.user_id = u.id
			WHERE lower(u.email) = lower($1)`, email,
		).Scan(target); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return statuses{}, fmt.Errorf("demo courier %s has no courier profile; run make seed first", email)
			}
			return statuses{}, fmt.Errorf("read demo courier: %w", err)
		}
	}
	if err := tx.QueryRow(ctx,
		`SELECT id::text FROM identity.users WHERE lower(email) = lower($1)`, config.DispatcherEmail,
	).Scan(&ids.dispatcherID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return statuses{}, errors.New("SEED_DISPATCHER_EMAIL does not match an identity user; run make seed first")
		}
		return statuses{}, fmt.Errorf("read demo dispatcher: %w", err)
	}
	if err := tx.QueryRow(ctx,
		`SELECT id::text FROM pricing.zones WHERE code = 'CMB' AND is_active`,
	).Scan(&ids.pickupZoneID); err != nil {
		return statuses{}, fmt.Errorf("read pickup zone CMB: %w", err)
	}
	if err := tx.QueryRow(ctx,
		`SELECT id::text FROM pricing.zones WHERE code = 'KDY' AND is_active`,
	).Scan(&ids.dropoffZoneID); err != nil {
		return statuses{}, fmt.Errorf("read dropoff zone KDY: %w", err)
	}
	return ids, nil
}

func insertHistory(ctx context.Context, tx pgx.Tx, parcelID string, spec parcelSpec, ids statuses, createdAt time.Time, courierID string) error {
	chain := historyFor(spec.status)
	previous := any(nil)
	for index, status := range chain {
		actorID := ids.customerID
		actorRole := "CUSTOMER"
		if index > 0 {
			actorID = courierID
			actorRole = "COURIER"
		}
		failureReason := nullableString("Demo failure reason", status == "FAILED")
		if _, err := tx.Exec(ctx, `
			INSERT INTO parcel.parcel_status_history
				(parcel_id, previous_status, new_status, actor_user_id, actor_role, note, failure_reason, created_at)
			VALUES ($1, $2, $3, $4, $5, 'Demo seed', $6, $7)`,
			parcelID, previous, status, actorID, actorRole, failureReason,
			createdAt.Add(time.Duration(index)*30*time.Minute),
		); err != nil {
			return fmt.Errorf("insert demo history for %s: %w", parcelID, err)
		}
		previous = status
	}
	return nil
}

func historyFor(status string) []string {
	switch status {
	case "CREATED":
		return []string{"CREATED"}
	case "PICKED_UP":
		return []string{"CREATED", "PICKED_UP"}
	case "IN_TRANSIT":
		return []string{"CREATED", "PICKED_UP", "IN_TRANSIT"}
	case "OUT_FOR_DELIVERY":
		return []string{"CREATED", "PICKED_UP", "IN_TRANSIT", "OUT_FOR_DELIVERY"}
	case "DELIVERED":
		return []string{"CREATED", "PICKED_UP", "IN_TRANSIT", "OUT_FOR_DELIVERY", "DELIVERED"}
	case "FAILED":
		return []string{"CREATED", "PICKED_UP", "FAILED"}
	default:
		return []string{"CREATED"}
	}
}

func nullable(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func nullableString(value string, ok bool) any {
	if !ok {
		return nil
	}
	return value
}

func nullableTime(value time.Time, ok bool) any {
	if !ok {
		return nil
	}
	return value
}
