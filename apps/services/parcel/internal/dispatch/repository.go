package dispatch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/Vineth-R/swiftdrop/apps/services/parcel/internal/statemachine"
)

// CourierStatusCounts stores the counts of active deliveries broken down by status.
type CourierStatusCounts struct {
	Total    int
	ByStatus map[string]int
}

// DB defines database operations required by the dispatch repository.
type DB interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	Begin(ctx context.Context) (pgx.Tx, error)
}

// Repository handles database interactions for the dispatch domain.
type Repository struct {
	db DB
}

// NewRepository constructs a dispatch Repository.
func NewRepository(db DB) *Repository {
	return &Repository{db: db}
}

// ListUnassigned queries parcels that have no current courier and are not in a closed state.
func (r *Repository) ListUnassigned(ctx context.Context, filter UnassignedFilter) (UnassignedPage, error) {
	if r.db == nil {
		return UnassignedPage{}, errors.New("database pool is not configured")
	}

	page := filter.Page
	if page < 1 {
		page = 1
	}
	pageSize := filter.PageSize
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	offset := (page - 1) * pageSize

	var whereClauses []string
	var args []any
	argIdx := 1

	whereClauses = append(whereClauses, "current_courier_id IS NULL")
	whereClauses = append(whereClauses, "status NOT IN ('DELIVERED', 'FAILED')")

	if filter.ZoneID != nil && strings.TrimSpace(*filter.ZoneID) != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("pickup_zone_id = $%d", argIdx))
		args = append(args, strings.TrimSpace(*filter.ZoneID))
		argIdx++
	}

	if filter.From != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("created_at >= $%d", argIdx))
		args = append(args, *filter.From)
		argIdx++
	}

	if filter.To != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("created_at <= $%d", argIdx))
		args = append(args, *filter.To)
		argIdx++
	}

	whereSQL := strings.Join(whereClauses, " AND ")

	// Count total items
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM parcel.parcels WHERE %s", whereSQL)
	var totalItems int
	if err := r.db.QueryRow(ctx, countQuery, args...).Scan(&totalItems); err != nil {
		return UnassignedPage{}, fmt.Errorf("count unassigned parcels: %w", err)
	}

	orderDir := "ASC"
	if strings.EqualFold(filter.Sort, "createdAt:desc") {
		orderDir = "DESC"
	}

	selectQuery := fmt.Sprintf(`
		SELECT id, tracking_number, status, pickup_address, pickup_city, pickup_zone_id,
		       dropoff_address, dropoff_city, dropoff_zone_id, weight_kg, size_category, created_at
		FROM parcel.parcels
		WHERE %s
		ORDER BY created_at %s
		LIMIT $%d OFFSET $%d`,
		whereSQL, orderDir, argIdx, argIdx+1,
	)
	args = append(args, pageSize, offset)

	rows, err := r.db.Query(ctx, selectQuery, args...)
	if err != nil {
		return UnassignedPage{}, fmt.Errorf("query unassigned parcels: %w", err)
	}
	defer rows.Close()

	items := make([]UnassignedParcel, 0)
	for rows.Next() {
		var item UnassignedParcel
		var weightNum pgtype.Numeric
		var pZoneID, dZoneID *string

		if err := rows.Scan(
			&item.ID,
			&item.TrackingNumber,
			&item.Status,
			&item.PickupAddress,
			&item.PickupCity,
			&pZoneID,
			&item.DropoffAddress,
			&item.DropoffCity,
			&dZoneID,
			&weightNum,
			&item.SizeCategory,
			&item.CreatedAt,
		); err != nil {
			return UnassignedPage{}, fmt.Errorf("scan unassigned parcel: %w", err)
		}

		if pZoneID != nil {
			item.PickupZoneID = *pZoneID
		}
		if dZoneID != nil {
			item.DropoffZoneID = *dZoneID
		}

		weightVal, err := weightNum.Float64Value()
		if err == nil && weightVal.Valid {
			item.WeightKG = weightVal.Float64
		}

		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return UnassignedPage{}, fmt.Errorf("iterate unassigned parcels: %w", err)
	}

	return UnassignedPage{
		Items:      items,
		Page:       page,
		PageSize:   pageSize,
		TotalItems: totalItems,
	}, nil
}

// AssignCourier atomically updates the parcel and current assignment and persists an outbox event.
func (r *Repository) AssignCourier(ctx context.Context, input AssignInput) (AssignResult, error) {
	if r.db == nil {
		return AssignResult{}, errors.New("database pool is not configured")
	}

	tx, err := r.db.Begin(ctx)
	if err != nil {
		return AssignResult{}, fmt.Errorf("begin transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	result, err := r.assignInTx(ctx, tx, input)
	if err != nil {
		return AssignResult{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return AssignResult{}, fmt.Errorf("commit assignment transaction: %w", err)
	}
	return result, nil
}

func (r *Repository) assignInTx(ctx context.Context, tx pgx.Tx, input AssignInput) (AssignResult, error) {
	// 1. Lock and inspect parcel
	var trackingNumber, status string
	err := tx.QueryRow(ctx, `
		SELECT tracking_number, status
		FROM parcel.parcels
		WHERE id = $1
		FOR UPDATE`,
		input.ParcelID,
	).Scan(&trackingNumber, &status)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return AssignResult{}, ErrParcelNotFound
		}
		return AssignResult{}, fmt.Errorf("lock parcel: %w", err)
	}

	if statemachine.IsTerminal(statemachine.ParcelStatus(status)) {
		return AssignResult{}, ErrParcelClosed
	}

	// 2. Lock and close previous active assignment (if any)
	var prevAssignmentID string
	var prevCourierID *string
	err = tx.QueryRow(ctx, `
		SELECT id, courier_id
		FROM parcel.parcel_assignments
		WHERE parcel_id = $1 AND is_current = true
		FOR UPDATE`,
		input.ParcelID,
	).Scan(&prevAssignmentID, &prevCourierID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return AssignResult{}, fmt.Errorf("lock current assignment: %w", err)
	}

	if prevAssignmentID != "" {
		_, err := tx.Exec(ctx, `
			UPDATE parcel.parcel_assignments
			SET is_current = false, unassigned_at = now(), updated_at = now()
			WHERE id = $1`,
			prevAssignmentID,
		)
		if err != nil {
			return AssignResult{}, fmt.Errorf("close previous assignment: %w", err)
		}
	}

	// 3. Insert new active assignment
	var reason *string
	trimmedReason := strings.TrimSpace(input.Reason)
	if trimmedReason != "" {
		reason = &trimmedReason
	}

	var assignedAt time.Time
	err = tx.QueryRow(ctx, `
		INSERT INTO parcel.parcel_assignments (
			id, parcel_id, courier_id, previous_courier_id, assigned_by,
			assigned_at, is_current, reason
		) VALUES (
			gen_random_uuid(), $1, $2, $3, $4, now(), true, $5
		) RETURNING assigned_at`,
		input.ParcelID, input.CourierID, prevCourierID, input.ActorUserID, reason,
	).Scan(&assignedAt)
	if err != nil {
		return AssignResult{}, fmt.Errorf("insert new assignment: %w", err)
	}

	// 4. Update parcel current_courier_id
	_, err = tx.Exec(ctx, `
		UPDATE parcel.parcels
		SET current_courier_id = $2, updated_at = now()
		WHERE id = $1`,
		input.ParcelID, input.CourierID,
	)
	if err != nil {
		return AssignResult{}, fmt.Errorf("update parcel courier: %w", err)
	}

	// 5. Insert outbox event for parcel.assigned
	payloadData := map[string]any{
		"parcel_id":       input.ParcelID,
		"tracking_number": trackingNumber,
		"courier_id":      input.CourierID,
		"assigned_by":     input.ActorUserID,
	}
	if prevCourierID != nil && *prevCourierID != "" {
		payloadData["previous_courier_id"] = *prevCourierID
	}
	payloadBytes, err := json.Marshal(payloadData)
	if err != nil {
		return AssignResult{}, fmt.Errorf("marshal outbox payload: %w", err)
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO parcel.outbox_events (
			id, event_type, routing_key, payload, status, attempts,
			aggregate_type, aggregate_id, created_at
		) VALUES (
			gen_random_uuid(), 'parcel.assigned', 'parcel.assigned', $1, 'PENDING', 0,
			'parcel', $2, now()
		)`,
		payloadBytes, input.ParcelID,
	)
	if err != nil {
		return AssignResult{}, fmt.Errorf("insert outbox event: %w", err)
	}

	return AssignResult{
		ParcelID:          input.ParcelID,
		TrackingNumber:    trackingNumber,
		CourierID:         input.CourierID,
		PreviousCourierID: prevCourierID,
		AssignedBy:        input.ActorUserID,
		AssignedAt:        assignedAt,
	}, nil
}

// GetActiveWorkloadCounts returns the delivery counts for each currently assigned courier.
func (r *Repository) GetActiveWorkloadCounts(ctx context.Context) (map[string]CourierStatusCounts, error) {
	if r.db == nil {
		return nil, errors.New("database pool is not configured")
	}

	query := `
		SELECT pa.courier_id, p.status, COUNT(*)
		FROM parcel.parcel_assignments pa
		JOIN parcel.parcels p ON p.id = pa.parcel_id
		WHERE pa.is_current = true
		  AND p.status NOT IN ('DELIVERED', 'FAILED')
		GROUP BY pa.courier_id, p.status`

	rows, err := r.db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query courier workload counts: %w", err)
	}
	defer rows.Close()

	countsByCourier := make(map[string]CourierStatusCounts)
	for rows.Next() {
		var courierID, status string
		var count int
		if err := rows.Scan(&courierID, &status, &count); err != nil {
			return nil, fmt.Errorf("scan workload row: %w", err)
		}

		entry, ok := countsByCourier[courierID]
		if !ok {
			entry = CourierStatusCounts{
				Total:    0,
				ByStatus: make(map[string]int),
			}
		}
		entry.Total += count
		entry.ByStatus[status] = count
		countsByCourier[courierID] = entry
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate workload rows: %w", err)
	}

	return countsByCourier, nil
}

// ListAdminParcels performs administrative search across all parcels with pagination and filters.
func (r *Repository) ListAdminParcels(ctx context.Context, filter AdminParcelFilter) (AdminParcelPage, error) {
	if r.db == nil {
		return AdminParcelPage{}, errors.New("database pool is not configured")
	}

	page := filter.Page
	if page < 1 {
		page = 1
	}
	pageSize := filter.PageSize
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	offset := (page - 1) * pageSize

	var whereClauses []string
	var args []any
	argIdx := 1

	if filter.Status != nil && strings.TrimSpace(*filter.Status) != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("status = $%d", argIdx))
		args = append(args, strings.TrimSpace(*filter.Status))
		argIdx++
	}

	if filter.CourierID != nil && strings.TrimSpace(*filter.CourierID) != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("current_courier_id = $%d", argIdx))
		args = append(args, strings.TrimSpace(*filter.CourierID))
		argIdx++
	}

	if filter.MerchantID != nil && strings.TrimSpace(*filter.MerchantID) != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("merchant_id = $%d", argIdx))
		args = append(args, strings.TrimSpace(*filter.MerchantID))
		argIdx++
	}

	if filter.CustomerID != nil && strings.TrimSpace(*filter.CustomerID) != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("customer_user_id = $%d", argIdx))
		args = append(args, strings.TrimSpace(*filter.CustomerID))
		argIdx++
	}

	if filter.TrackingNumber != nil && strings.TrimSpace(*filter.TrackingNumber) != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("tracking_number ILIKE $%d || '%%' ESCAPE '\\'", argIdx))
		args = append(args, escapeLike(strings.TrimSpace(*filter.TrackingNumber)))
		argIdx++
	}

	if filter.From != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("created_at >= $%d", argIdx))
		args = append(args, *filter.From)
		argIdx++
	}

	if filter.To != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("created_at <= $%d", argIdx))
		args = append(args, *filter.To)
		argIdx++
	}

	whereSQL := "1=1"
	if len(whereClauses) > 0 {
		whereSQL = strings.Join(whereClauses, " AND ")
	}

	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM parcel.parcels WHERE %s", whereSQL)
	var totalItems int
	if err := r.db.QueryRow(ctx, countQuery, args...).Scan(&totalItems); err != nil {
		return AdminParcelPage{}, fmt.Errorf("count admin parcels: %w", err)
	}

	orderDir := "DESC"
	if strings.EqualFold(filter.Sort, "createdAt:asc") {
		orderDir = "ASC"
	}

	selectQuery := fmt.Sprintf(`
		SELECT id, tracking_number, status, pickup_city, dropoff_city,
		       current_courier_id, merchant_id, customer_user_id, price_amount, created_at
		FROM parcel.parcels
		WHERE %s
		ORDER BY created_at %s
		LIMIT $%d OFFSET $%d`,
		whereSQL, orderDir, argIdx, argIdx+1,
	)
	args = append(args, pageSize, offset)

	rows, err := r.db.Query(ctx, selectQuery, args...)
	if err != nil {
		return AdminParcelPage{}, fmt.Errorf("query admin parcels: %w", err)
	}
	defer rows.Close()

	items := make([]AdminParcelItem, 0)
	for rows.Next() {
		var item AdminParcelItem
		var priceNum pgtype.Numeric

		if err := rows.Scan(
			&item.ID,
			&item.TrackingNumber,
			&item.Status,
			&item.PickupCity,
			&item.DropoffCity,
			&item.CourierID,
			&item.MerchantID,
			&item.CustomerID,
			&priceNum,
			&item.CreatedAt,
		); err != nil {
			return AdminParcelPage{}, fmt.Errorf("scan admin parcel: %w", err)
		}

		priceVal, err := priceNum.Float64Value()
		if err == nil && priceVal.Valid {
			// Round to 2 decimal places
			item.Total, _ = strconv.ParseFloat(fmt.Sprintf("%.2f", priceVal.Float64), 64)
		}

		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return AdminParcelPage{}, fmt.Errorf("iterate admin parcels: %w", err)
	}

	return AdminParcelPage{
		Items:      items,
		Page:       page,
		PageSize:   pageSize,
		TotalItems: totalItems,
	}, nil
}

// escapeLike makes %, _ and \ match themselves in a LIKE pattern.
func escapeLike(value string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(value)
}
