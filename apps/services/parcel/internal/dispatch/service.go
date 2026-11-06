package dispatch

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/Vineth-R/swiftdrop/apps/services/parcel/internal/identityclient"
	"github.com/Vineth-R/swiftdrop/packages/go-common/validation"
)

// DispatchRepository defines repository operations required by the dispatch service.
type DispatchRepository interface {
	ListUnassigned(ctx context.Context, filter UnassignedFilter) (UnassignedPage, error)
	AssignCourier(ctx context.Context, input AssignInput) (AssignResult, error)
	GetActiveWorkloadCounts(ctx context.Context) (map[string]CourierStatusCounts, error)
	ListAdminParcels(ctx context.Context, filter AdminParcelFilter) (AdminParcelPage, error)
}

// Service implements the business logic for dispatch operations.
type Service struct {
	repo          DispatchRepository
	courierClient identityclient.CourierClient
}

// NewService constructs a dispatch Service.
func NewService(repo DispatchRepository, courierClient identityclient.CourierClient) *Service {
	return &Service{
		repo:          repo,
		courierClient: courierClient,
	}
}

// ListUnassigned returns unassigned parcels matching the filter.
func (s *Service) ListUnassigned(ctx context.Context, filter UnassignedFilter) (UnassignedPage, error) {
	if s.repo == nil {
		return UnassignedPage{}, errors.New("repository is not configured")
	}
	if filter.ZoneID != nil {
		if !validation.UUID(strings.TrimSpace(*filter.ZoneID)) {
			return UnassignedPage{}, fmt.Errorf("%w: invalid zoneId UUID", ErrInvalidInput)
		}
	}
	return s.repo.ListUnassigned(ctx, filter)
}

// AssignCourier checks courier eligibility and assigns/reassigns the parcel atomically.
func (s *Service) AssignCourier(ctx context.Context, input AssignInput) (AssignResult, error) {
	if s.repo == nil {
		return AssignResult{}, errors.New("repository is not configured")
	}

	if !validation.UUID(strings.TrimSpace(input.ParcelID)) {
		return AssignResult{}, fmt.Errorf("%w: invalid parcelId UUID", ErrInvalidInput)
	}
	if !validation.UUID(strings.TrimSpace(input.CourierID)) {
		return AssignResult{}, fmt.Errorf("%w: invalid courierId UUID", ErrCourierNotEligible)
	}
	if len(input.Reason) > 255 {
		return AssignResult{}, fmt.Errorf("%w: reason exceeds 255 characters", ErrInvalidInput)
	}

	// Verify target courier eligibility via Identity service
	if s.courierClient == nil {
		return AssignResult{}, fmt.Errorf("%w: courier eligibility client is not configured", ErrServiceUnavailable)
	}
	if err := s.courierClient.CheckEligibility(ctx, input.CourierID); err != nil {
		if errors.Is(err, identityclient.ErrCourierNotEligible) {
			return AssignResult{}, ErrCourierNotEligible
		}
		return AssignResult{}, fmt.Errorf("%w: %v", ErrServiceUnavailable, err)
	}

	return s.repo.AssignCourier(ctx, input)
}

// GetWorkload retrieves each courier's availability and active deliveries.
func (s *Service) GetWorkload(ctx context.Context, authHeader string, cookie *http.Cookie) ([]CourierWorkload, error) {
	if s.repo == nil {
		return nil, errors.New("repository is not configured")
	}

	// 1. Get database delivery counts for active assignments
	dbCounts, err := s.repo.GetActiveWorkloadCounts(ctx)
	if err != nil {
		return nil, fmt.Errorf("load workload counts: %w", err)
	}

	// 2. Retrieve couriers from Identity (fail-closed if client is not configured)
	if s.courierClient == nil {
		return nil, fmt.Errorf("%w: courier client is not configured", ErrServiceUnavailable)
	}
	couriers, err := s.courierClient.ListCouriers(ctx, authHeader, cookie)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to fetch couriers: %v", ErrServiceUnavailable, err)
	}

	result := make([]CourierWorkload, 0, len(couriers))
	for _, courier := range couriers {
		counts := dbCounts[courier.ID]
		byStatus := map[string]int{
			"CREATED":          counts.ByStatus["CREATED"],
			"PICKED_UP":        counts.ByStatus["PICKED_UP"],
			"IN_TRANSIT":       counts.ByStatus["IN_TRANSIT"],
			"OUT_FOR_DELIVERY": counts.ByStatus["OUT_FOR_DELIVERY"],
		}
		result = append(result, CourierWorkload{
			CourierID:        courier.ID,
			FullName:         courier.FullName,
			IsAvailable:      courier.IsAvailable,
			ActiveDeliveries: counts.Total,
			ByStatus:         byStatus,
		})
	}

	return result, nil
}

// ListAdminParcels searches parcels with the given admin filters.
func (s *Service) ListAdminParcels(ctx context.Context, filter AdminParcelFilter) (AdminParcelPage, error) {
	if s.repo == nil {
		return AdminParcelPage{}, errors.New("repository is not configured")
	}
	if filter.CourierID != nil && strings.TrimSpace(*filter.CourierID) != "" {
		if !validation.UUID(strings.TrimSpace(*filter.CourierID)) {
			return AdminParcelPage{}, fmt.Errorf("%w: invalid courierId UUID", ErrInvalidInput)
		}
	}
	if filter.MerchantID != nil && strings.TrimSpace(*filter.MerchantID) != "" {
		if !validation.UUID(strings.TrimSpace(*filter.MerchantID)) {
			return AdminParcelPage{}, fmt.Errorf("%w: invalid merchantId UUID", ErrInvalidInput)
		}
	}
	if filter.CustomerID != nil && strings.TrimSpace(*filter.CustomerID) != "" {
		if !validation.UUID(strings.TrimSpace(*filter.CustomerID)) {
			return AdminParcelPage{}, fmt.Errorf("%w: invalid customerId UUID", ErrInvalidInput)
		}
	}
	return s.repo.ListAdminParcels(ctx, filter)
}
