package orders

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// Store is the merchant-owned order persistence seam.
type Store interface {
	OrderByExternalOrderID(ctx context.Context, merchantID, externalOrderID string) (Order, error)
	OrderByIdempotencyKey(ctx context.Context, merchantID, key string) (Order, error)
	CreateOrder(ctx context.Context, params CreateParams) (Order, error)
	OrderDetail(ctx context.Context, merchantID, orderID string) (Detail, error)
	ListOrders(ctx context.Context, filter ListFilter) ([]Order, int, error)
}

// ParcelClient creates a parcel through the trusted internal contract.
type ParcelClient interface {
	CreateMerchant(ctx context.Context, input ParcelInput) (ParcelResult, error)
}

// Service owns merchant order orchestration. It never reads another service's
// tables: pricing and parcel work happen through their internal HTTP APIs.
type Service struct {
	store   Store
	parcels ParcelClient
	zones   ZoneLookup
}

// New returns a merchant order service.
func New(store Store, parcels ParcelClient, zones ZoneLookup) *Service {
	return &Service{store: store, parcels: parcels, zones: zones}
}

// CreateResult tells the handler whether the order is new (201) or a replay (200).
type CreateResult struct {
	Order   Order
	Created bool
}

// Create applies API 8.16 idempotency, creates the parcel through Parcel
// Operations, and persists the snapshot plus the initial CREATED timeline row.
func (s *Service) Create(ctx context.Context, merchant MerchantProfile, idempotencyKey string, in CreateInput) (CreateResult, error) {
	if s == nil || s.store == nil {
		return CreateResult{}, ErrParcelUnavailable
	}
	normalized, err := s.normalize(ctx, merchant, idempotencyKey, in)
	if err != nil {
		return CreateResult{}, err
	}
	hash := Hash(normalized)

	// Check the caller's idempotency key first. If it already names a different
	// order, the hash differs and the request is a conflict even when the
	// external order reference also happens to exist.
	if normalized.IdempotencyKey != "" {
		existing, err := s.store.OrderByIdempotencyKey(ctx, merchant.ID, normalized.IdempotencyKey)
		if err == nil {
			return replay(existing, hash)
		}
		if !errors.Is(err, ErrNotFound) {
			return CreateResult{}, fmt.Errorf("look up idempotency key: %w", err)
		}
	}
	existing, err := s.store.OrderByExternalOrderID(ctx, merchant.ID, normalized.ExternalOrderID)
	if err == nil {
		return replay(existing, hash)
	}
	if !errors.Is(err, ErrNotFound) {
		return CreateResult{}, fmt.Errorf("look up merchant order: %w", err)
	}

	if s.parcels == nil {
		return CreateResult{}, ErrParcelUnavailable
	}
	parcel, err := s.parcels.CreateMerchant(ctx, ParcelInput{
		MerchantID:      merchant.ID,
		CreatedByUserID: merchant.OwnerUserID,
		ExternalOrderID: normalized.ExternalOrderID,
		IdempotencyKey:  normalized.IdempotencyKey,
		Pickup:          normalized.Pickup,
		Recipient:       normalized.Recipient,
		WeightKG:        normalized.WeightKG,
		SizeCategory:    normalized.SizeCategory,
		Description:     normalized.Description,
	})
	if err != nil {
		return CreateResult{}, err
	}
	if err := validateParcelResult(parcel); err != nil {
		return CreateResult{}, err
	}

	key := (*string)(nil)
	if normalized.IdempotencyKey != "" {
		value := normalized.IdempotencyKey
		key = &value
	}
	status := parcel.Status
	if status == "" {
		status = "CREATED"
	}
	order, err := s.store.CreateOrder(ctx, CreateParams{
		MerchantID:      merchant.ID,
		ExternalOrderID: normalized.ExternalOrderID,
		IdempotencyKey:  key,
		RequestHash:     hash,
		ParcelID:        parcel.ParcelID,
		TrackingNumber:  parcel.TrackingNumber,
		Pickup:          normalized.Pickup,
		Recipient:       normalized.Recipient,
		WeightKG:        normalized.WeightKG,
		SizeCategory:    normalized.SizeCategory,
		Description:     normalized.Description,
		Fee:             parcel.Fee,
		Status:          status,
	})
	if err == nil {
		return CreateResult{Order: order, Created: true}, nil
	}
	if !errors.Is(err, ErrDuplicateOrder) {
		return CreateResult{}, fmt.Errorf("persist merchant order: %w", err)
	}

	// A concurrent request won the insert. Replay the winner when the payload
	// is the same, otherwise report the conflict. Check the idempotency key
	// first for the same reason as the pre-check above. The Parcel call is safe
	// to repeat because Parcel replays by (merchantId, externalOrderId).
	if normalized.IdempotencyKey != "" {
		existing, lookupErr := s.store.OrderByIdempotencyKey(ctx, merchant.ID, normalized.IdempotencyKey)
		if lookupErr == nil {
			return replay(existing, hash)
		}
	}
	existing, lookupErr := s.store.OrderByExternalOrderID(ctx, merchant.ID, normalized.ExternalOrderID)
	if lookupErr == nil {
		return replay(existing, hash)
	}
	return CreateResult{}, ErrDuplicateOrder
}

func replay(existing Order, hash string) (CreateResult, error) {
	if existing.RequestHash != hash {
		return CreateResult{}, ErrDuplicateOrder
	}
	return CreateResult{Order: existing, Created: false}, nil
}

func validateParcelResult(parcel ParcelResult) error {
	switch {
	case strings.TrimSpace(parcel.ParcelID) == "" || strings.TrimSpace(parcel.TrackingNumber) == "":
		return ErrParcelUnavailable
	case parcel.Fee.Currency != "LKR":
		return ErrParcelUnavailable
	case parcel.Fee.BaseFee < 0 || parcel.Fee.WeightCharge < 0 || parcel.Fee.SizeSurcharge < 0 || parcel.Fee.Total < 0:
		return ErrParcelUnavailable
	case parcel.Fee.Total != parcel.Fee.BaseFee+parcel.Fee.WeightCharge+parcel.Fee.SizeSurcharge:
		return ErrParcelUnavailable
	case parcel.Status != "" && !validStatus(parcel.Status):
		return ErrParcelUnavailable
	}
	return nil
}

// ValidStatus reports whether status is one of the API v1.1 parcel statuses.
func ValidStatus(status string) bool { return validStatus(status) }

func validStatus(status string) bool {
	switch status {
	case "CREATED", "PICKED_UP", "IN_TRANSIT", "OUT_FOR_DELIVERY", "DELIVERED", "FAILED":
		return true
	default:
		return false
	}
}

// Detail returns one merchant-scoped order and its timeline.
func (s *Service) Detail(ctx context.Context, merchantID, orderID string) (Detail, error) {
	if s == nil || s.store == nil {
		return Detail{}, ErrNotFound
	}
	return s.store.OrderDetail(ctx, merchantID, orderID)
}

// List returns one page of the merchant's own orders.
func (s *Service) List(ctx context.Context, filter ListFilter) ([]Order, int, error) {
	if s == nil || s.store == nil {
		return nil, 0, ErrNotFound
	}
	return s.store.ListOrders(ctx, filter)
}
