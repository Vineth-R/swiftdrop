package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/Vineth-R/swiftdrop/apps/services/merchant/internal/orders"
)

const orderColumns = `o.id, o.merchant_id, o.external_order_id, o.idempotency_key, o.request_hash,
	o.parcel_id, o.tracking_number,
	COALESCE(o.pickup_name, ''), COALESCE(o.pickup_phone, ''), COALESCE(o.pickup_address, ''),
	COALESCE(o.pickup_city, ''), COALESCE(o.pickup_zone_id::text, ''),
	o.recipient_name, COALESCE(o.recipient_phone, ''), COALESCE(o.recipient_address, ''),
	COALESCE(o.recipient_zone_id::text, ''), o.dropoff_city,
	(o.weight_kg * 100)::bigint, o.size_category, o.description,
	(o.fee_amount * 100)::bigint, o.fee_breakdown, o.currency,
	o.last_known_status, o.last_status_at, o.created_at, o.updated_at`

const orderReturning = `id, merchant_id, external_order_id, idempotency_key, request_hash,
	parcel_id, tracking_number,
	COALESCE(pickup_name, ''), COALESCE(pickup_phone, ''), COALESCE(pickup_address, ''),
	COALESCE(pickup_city, ''), COALESCE(pickup_zone_id::text, ''),
	recipient_name, COALESCE(recipient_phone, ''), COALESCE(recipient_address, ''),
	COALESCE(recipient_zone_id::text, ''), dropoff_city,
	(weight_kg * 100)::bigint, size_category, description,
	(fee_amount * 100)::bigint, fee_breakdown, currency,
	last_known_status, last_status_at, created_at, updated_at`

func scanOrder(row pgx.Row) (orders.Order, error) {
	var order orders.Order
	var weightCents, feeCents int64
	var breakdown []byte
	err := row.Scan(
		&order.ID, &order.MerchantID, &order.ExternalOrderID, &order.IdempotencyKey, &order.RequestHash,
		&order.ParcelID, &order.TrackingNumber,
		&order.Pickup.Name, &order.Pickup.Phone, &order.Pickup.Address, &order.Pickup.City, &order.Pickup.ZoneID,
		&order.Recipient.Name, &order.Recipient.Phone, &order.Recipient.Address, &order.Recipient.ZoneID,
		&order.Recipient.City,
		&weightCents, &order.SizeCategory, &order.Description,
		&feeCents, &breakdown, &order.Fee.Currency,
		&order.Status, &order.LastStatusAt, &order.CreatedAt, &order.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return orders.Order{}, orders.ErrNotFound
	}
	if err != nil {
		return orders.Order{}, fmt.Errorf("scan merchant order: %w", err)
	}
	order.WeightKG = orders.Decimal(weightCents)
	order.Fee.Total = orders.Decimal(feeCents)
	if len(breakdown) > 0 {
		if err := json.Unmarshal(breakdown, &order.Fee); err != nil {
			return orders.Order{}, fmt.Errorf("decode fee snapshot: %w", err)
		}
		order.Fee.Total = orders.Decimal(feeCents)
	}
	return order, nil
}

// OrderByExternalOrderID is the stable replay key for a merchant order.
func (r *Repository) OrderByExternalOrderID(ctx context.Context, merchantID, externalOrderID string) (orders.Order, error) {
	return scanOrder(r.db.QueryRow(ctx, `SELECT `+orderColumns+`
		FROM merchant.merchant_orders o
		WHERE o.merchant_id = $1 AND o.external_order_id = $2`, merchantID, externalOrderID))
}

// OrderByIdempotencyKey finds an order claimed by a client idempotency key.
func (r *Repository) OrderByIdempotencyKey(ctx context.Context, merchantID, key string) (orders.Order, error) {
	return scanOrder(r.db.QueryRow(ctx, `SELECT `+orderColumns+`
		FROM merchant.merchant_orders o
		WHERE o.merchant_id = $1 AND o.idempotency_key = $2`, merchantID, key))
}

// CreateOrder persists the snapshot and the initial CREATED timeline row in one
// transaction. Unique violations are reported as orders.ErrDuplicateOrder.
func (r *Repository) CreateOrder(ctx context.Context, params orders.CreateParams) (orders.Order, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return orders.Order{}, fmt.Errorf("begin merchant order creation: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	breakdown, err := json.Marshal(params.Fee)
	if err != nil {
		return orders.Order{}, fmt.Errorf("encode fee snapshot: %w", err)
	}
	row := tx.QueryRow(ctx, `INSERT INTO merchant.merchant_orders (
			merchant_id, external_order_id, idempotency_key, request_hash,
			parcel_id, tracking_number, pickup_name, pickup_phone, pickup_address,
			pickup_city, pickup_zone_id, recipient_name, recipient_phone,
			recipient_address, recipient_zone_id, dropoff_city, weight_kg,
			size_category, description, fee_amount, fee_breakdown, currency,
			last_known_status, last_status_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15,
			$16, $17, $18, $19, $20, $21, 'LKR', $22, now()
		) RETURNING `+orderReturning,
		params.MerchantID, params.ExternalOrderID, params.IdempotencyKey, params.RequestHash,
		params.ParcelID, params.TrackingNumber, params.Pickup.Name, params.Pickup.Phone,
		params.Pickup.Address, params.Pickup.City, params.Pickup.ZoneID,
		params.Recipient.Name, params.Recipient.Phone, params.Recipient.Address,
		params.Recipient.ZoneID, params.Recipient.City, params.WeightKG.String(),
		params.SizeCategory, params.Description, params.Fee.Total.String(),
		string(breakdown), params.Status,
	)
	order, err := scanOrder(row)
	if err != nil {
		return orders.Order{}, mapOrderWriteError(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO merchant.merchant_order_status_events
		(order_id, event_id, previous_status, status, occurred_at)
		VALUES ($1, gen_random_uuid(), NULL, $2, $3)`, order.ID, order.Status, order.CreatedAt); err != nil {
		return orders.Order{}, fmt.Errorf("insert initial merchant order timeline: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return orders.Order{}, fmt.Errorf("commit merchant order creation: %w", err)
	}
	return order, nil
}

func mapOrderWriteError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		switch pgErr.ConstraintName {
		case "merchant_orders_external_uq", "merchant_orders_idem_uq",
			"merchant_orders_parcel_id_key", "merchant_orders_tracking_number_key":
			return orders.ErrDuplicateOrder
		}
	}
	return err
}

// OrderDetail returns one merchant-scoped order and its timeline in one query.
func (r *Repository) OrderDetail(ctx context.Context, merchantID, orderID string) (orders.Detail, error) {
	var detail orders.Detail
	var weightCents, feeCents int64
	var breakdown, timeline []byte
	err := r.db.QueryRow(ctx, `SELECT `+orderColumns+`,
			COALESCE((
				SELECT jsonb_agg(jsonb_build_object('status', e.status, 'at', e.occurred_at)
					ORDER BY e.occurred_at ASC, e.created_at ASC)
				FROM merchant.merchant_order_status_events e
				WHERE e.order_id = o.id
			), '[]'::jsonb)
		FROM merchant.merchant_orders o
		WHERE o.id = $1 AND o.merchant_id = $2`, orderID, merchantID,
	).Scan(
		&detail.ID, &detail.MerchantID, &detail.ExternalOrderID, &detail.IdempotencyKey, &detail.RequestHash,
		&detail.ParcelID, &detail.TrackingNumber,
		&detail.Pickup.Name, &detail.Pickup.Phone, &detail.Pickup.Address, &detail.Pickup.City, &detail.Pickup.ZoneID,
		&detail.Recipient.Name, &detail.Recipient.Phone, &detail.Recipient.Address, &detail.Recipient.ZoneID,
		&detail.Recipient.City, &weightCents, &detail.SizeCategory, &detail.Description,
		&feeCents, &breakdown, &detail.Fee.Currency,
		&detail.Status, &detail.LastStatusAt, &detail.CreatedAt, &detail.UpdatedAt,
		&timeline,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return orders.Detail{}, orders.ErrNotFound
	}
	if err != nil {
		return orders.Detail{}, fmt.Errorf("query merchant order detail: %w", err)
	}
	detail.WeightKG = orders.Decimal(weightCents)
	detail.Fee.Total = orders.Decimal(feeCents)
	if len(breakdown) > 0 {
		if err := json.Unmarshal(breakdown, &detail.Fee); err != nil {
			return orders.Detail{}, fmt.Errorf("decode fee snapshot: %w", err)
		}
		detail.Fee.Total = orders.Decimal(feeCents)
	}
	detail.Timeline = make([]orders.TimelineEvent, 0)
	if len(timeline) > 0 {
		if err := json.Unmarshal(timeline, &detail.Timeline); err != nil {
			return orders.Detail{}, fmt.Errorf("decode merchant order timeline: %w", err)
		}
	}
	return detail, nil
}

// ListOrders returns a merchant-scoped page and total. Sort fields and direction
// are resolved here, never interpolated from the request.
func (r *Repository) ListOrders(ctx context.Context, filter orders.ListFilter) ([]orders.Order, int, error) {
	where := []string{"o.merchant_id = $1"}
	args := []any{filter.MerchantID}
	if filter.Status != "" {
		args = append(args, filter.Status)
		where = append(where, fmt.Sprintf("o.last_known_status = $%d", len(args)))
	}
	if filter.ExternalOrderID != "" {
		args = append(args, filter.ExternalOrderID)
		where = append(where, fmt.Sprintf("o.external_order_id = $%d", len(args)))
	}
	if filter.From != nil {
		args = append(args, *filter.From)
		where = append(where, fmt.Sprintf("o.created_at >= $%d", len(args)))
	}
	if filter.To != nil {
		args = append(args, *filter.To)
		where = append(where, fmt.Sprintf("o.created_at < $%d", len(args)))
	}

	whereSQL := strings.Join(where, " AND ")
	countQuery := `SELECT count(*) FROM merchant.merchant_orders o WHERE ` + whereSQL
	var total int
	if err := r.db.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count merchant orders: %w", err)
	}

	sortColumn := "o.created_at"
	switch filter.Sort {
	case "updatedAt":
		sortColumn = "o.updated_at"
	case "externalOrderId":
		sortColumn = "o.external_order_id"
	case "createdAt", "":
	}
	direction := "DESC"
	if !filter.Desc {
		direction = "ASC"
	}
	args = append(args, filter.PageSize, (filter.Page-1)*filter.PageSize)
	query := fmt.Sprintf(`SELECT %s FROM merchant.merchant_orders o
		WHERE %s
		ORDER BY %s %s, o.id %s
		LIMIT $%d OFFSET $%d`,
		orderColumns, whereSQL, sortColumn, direction, direction, len(args)-1, len(args))
	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list merchant orders: %w", err)
	}
	defer rows.Close()
	items := make([]orders.Order, 0, filter.PageSize)
	for rows.Next() {
		order, err := scanOrder(rows)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, order)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate merchant orders: %w", err)
	}
	return items, total, nil
}

// ApplyStatusEvent applies one parcel.status_changed.v1 event exactly once and
// only when it is newer than the stored status. The order row lock serializes
// concurrent events, so an older event cannot overwrite a newer one.
func (r *Repository) ApplyStatusEvent(ctx context.Context, event orders.StatusEvent) (bool, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("begin merchant status sync: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var orderID, trackingNumber string
	var lastStatusAt time.Time
	err = tx.QueryRow(ctx, `SELECT id, tracking_number, last_status_at
		FROM merchant.merchant_orders
		WHERE parcel_id = $1 AND merchant_id = $2
		FOR UPDATE`, event.ParcelID, event.MerchantID).Scan(&orderID, &trackingNumber, &lastStatusAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, orders.ErrNotFound
	}
	if err != nil {
		return false, fmt.Errorf("lock merchant order for status sync: %w", err)
	}
	if trackingNumber != event.TrackingNumber {
		return false, orders.ErrTrackingMismatch
	}
	if !event.OccurredAt.After(lastStatusAt) {
		return false, nil
	}

	result, err := tx.Exec(ctx, `INSERT INTO merchant.merchant_order_status_events
		(order_id, event_id, previous_status, status, occurred_at)
		VALUES ($1, $2, NULLIF($3, ''), $4, $5)
		ON CONFLICT (event_id) DO NOTHING`,
		orderID, event.EventID, event.PreviousStatus, event.Status, event.OccurredAt)
	if err != nil {
		return false, fmt.Errorf("insert merchant status event: %w", err)
	}
	if result.RowsAffected() == 0 {
		return false, nil
	}
	if _, err := tx.Exec(ctx, `UPDATE merchant.merchant_orders
		SET last_known_status = $2, last_status_at = $3
		WHERE id = $1`, orderID, event.Status, event.OccurredAt); err != nil {
		return false, fmt.Errorf("update merchant order status: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("commit merchant status sync: %w", err)
	}
	return true, nil
}
