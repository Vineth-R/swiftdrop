package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type ParcelStatus string

const (
	ParcelStatusCreated        ParcelStatus = "CREATED"
	ParcelStatusPickedUp       ParcelStatus = "PICKED_UP"
	ParcelStatusInTransit      ParcelStatus = "IN_TRANSIT"
	ParcelStatusOutForDelivery ParcelStatus = "OUT_FOR_DELIVERY"
	ParcelStatusDelivered      ParcelStatus = "DELIVERED"
	ParcelStatusFailed         ParcelStatus = "FAILED"
)

type SizeCategory string

const (
	SizeCategorySmall     SizeCategory = "SMALL"
	SizeCategoryMedium    SizeCategory = "MEDIUM"
	SizeCategoryLarge     SizeCategory = "LARGE"
	SizeCategoryOversized SizeCategory = "OVERSIZED"
)

type Parcel struct {
	ID               string
	TrackingNumber   string
	CustomerUserID   *string
	MerchantID       *string
	ExternalOrderID  *string
	IdempotencyKey   *string
	SenderName       string
	SenderPhone      string
	PickupAddress    string
	PickupCity       string
	PickupZoneID     string
	RecipientName    string
	RecipientPhone   string
	DropoffAddress   string
	DropoffCity      string
	DropoffZoneID    string
	WeightKG         pgtype.Numeric
	SizeCategory     SizeCategory
	Description      *string
	Status           ParcelStatus
	FailureReason    *string
	CurrentCourierID *string
	PriceAmount      pgtype.Numeric
	Currency         string
	PriceBreakdown   json.RawMessage
	CreatedByUserID  string
	CreatedAt        time.Time
	UpdatedAt        time.Time
	DeliveredAt      *time.Time
	FailedAt         *time.Time
	DeliveredToName  *string
}

type CreateParcelParams struct {
	ID              string
	TrackingNumber  string
	CustomerUserID  *string
	MerchantID      *string
	ExternalOrderID *string
	IdempotencyKey  *string
	SenderName      string
	SenderPhone     string
	PickupAddress   string
	PickupCity      string
	PickupZoneID    string
	RecipientName   string
	RecipientPhone  string
	DropoffAddress  string
	DropoffCity     string
	DropoffZoneID   string
	WeightKG        pgtype.Numeric
	SizeCategory    SizeCategory
	Description     *string
	PriceAmount     pgtype.Numeric
	Currency        string
	PriceBreakdown  json.RawMessage
	CreatedByUserID string
}

const parcelColumns = `id, tracking_number, customer_user_id, merchant_id,
	external_order_id, idempotency_key, sender_name, sender_phone, pickup_address,
	pickup_city, pickup_zone_id, recipient_name, recipient_phone, dropoff_address,
	dropoff_city, dropoff_zone_id, weight_kg, size_category, description, status,
	failure_reason, current_courier_id, price_amount, currency, price_breakdown,
	created_by_user_id, created_at, updated_at, delivered_at, failed_at, delivered_to_name`

// CreateParcel inserts a CREATED parcel. The service must use a transaction-bound
// repository to insert its initial history and outbox record before committing.
func (r *Repository) CreateParcel(ctx context.Context, params CreateParcelParams) (Parcel, error) {
	parcel, err := scanParcel(r.db.QueryRow(
		ctx,
		`INSERT INTO parcel.parcels (
			id, tracking_number, customer_user_id, merchant_id, external_order_id,
			idempotency_key, sender_name, sender_phone, pickup_address, pickup_city,
			pickup_zone_id, recipient_name, recipient_phone, dropoff_address,
			dropoff_city, dropoff_zone_id, weight_kg, size_category, description,
			price_amount, currency, price_breakdown, created_by_user_id
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12,
			$13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23
		) RETURNING `+parcelColumns,
		params.ID,
		params.TrackingNumber,
		params.CustomerUserID,
		params.MerchantID,
		params.ExternalOrderID,
		params.IdempotencyKey,
		params.SenderName,
		params.SenderPhone,
		params.PickupAddress,
		params.PickupCity,
		params.PickupZoneID,
		params.RecipientName,
		params.RecipientPhone,
		params.DropoffAddress,
		params.DropoffCity,
		params.DropoffZoneID,
		params.WeightKG,
		params.SizeCategory,
		params.Description,
		params.PriceAmount,
		params.Currency,
		params.PriceBreakdown,
		params.CreatedByUserID,
	))
	if err != nil {
		return Parcel{}, fmt.Errorf("creating parcel: %w", databaseError(err))
	}

	return parcel, nil
}

func (r *Repository) ParcelByID(ctx context.Context, parcelID string) (Parcel, error) {
	parcel, err := scanParcel(r.db.QueryRow(
		ctx,
		`SELECT `+parcelColumns+` FROM parcel.parcels WHERE id = $1`,
		parcelID,
	))
	if err != nil {
		return Parcel{}, fmt.Errorf("querying parcel: %w", databaseError(err))
	}

	return parcel, nil
}

func (r *Repository) ParcelByCustomerIdempotencyKey(
	ctx context.Context,
	customerUserID string,
	idempotencyKey string,
) (Parcel, error) {
	parcel, err := scanParcel(r.db.QueryRow(
		ctx,
		`SELECT `+parcelColumns+` FROM parcel.parcels
		 WHERE customer_user_id = $1 AND idempotency_key = $2`,
		customerUserID,
		idempotencyKey,
	))
	if err != nil {
		return Parcel{}, fmt.Errorf("querying customer parcel by idempotency key: %w", databaseError(err))
	}

	return parcel, nil
}

func (r *Repository) ParcelByMerchantExternalOrderID(
	ctx context.Context,
	merchantID string,
	externalOrderID string,
) (Parcel, error) {
	parcel, err := scanParcel(r.db.QueryRow(
		ctx,
		`SELECT `+parcelColumns+` FROM parcel.parcels
		 WHERE merchant_id = $1 AND external_order_id = $2`,
		merchantID,
		externalOrderID,
	))
	if err != nil {
		return Parcel{}, fmt.Errorf("querying merchant parcel by external order: %w", databaseError(err))
	}

	return parcel, nil
}

func scanParcel(row pgx.Row) (Parcel, error) {
	var parcel Parcel
	err := row.Scan(
		&parcel.ID,
		&parcel.TrackingNumber,
		&parcel.CustomerUserID,
		&parcel.MerchantID,
		&parcel.ExternalOrderID,
		&parcel.IdempotencyKey,
		&parcel.SenderName,
		&parcel.SenderPhone,
		&parcel.PickupAddress,
		&parcel.PickupCity,
		&parcel.PickupZoneID,
		&parcel.RecipientName,
		&parcel.RecipientPhone,
		&parcel.DropoffAddress,
		&parcel.DropoffCity,
		&parcel.DropoffZoneID,
		&parcel.WeightKG,
		&parcel.SizeCategory,
		&parcel.Description,
		&parcel.Status,
		&parcel.FailureReason,
		&parcel.CurrentCourierID,
		&parcel.PriceAmount,
		&parcel.Currency,
		&parcel.PriceBreakdown,
		&parcel.CreatedByUserID,
		&parcel.CreatedAt,
		&parcel.UpdatedAt,
		&parcel.DeliveredAt,
		&parcel.FailedAt,
		&parcel.DeliveredToName,
	)
	if err != nil {
		return Parcel{}, err
	}

	return parcel, nil
}
