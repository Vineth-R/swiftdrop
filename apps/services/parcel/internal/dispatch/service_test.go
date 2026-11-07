package dispatch

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/Vineth-R/swiftdrop/apps/services/parcel/internal/identityclient"
	"github.com/Vineth-R/swiftdrop/apps/services/parcel/internal/statemachine"
)

type mockDispatchRepo struct {
	listUnassignedFn func(ctx context.Context, filter UnassignedFilter) (UnassignedPage, error)
	assignCourierFn  func(ctx context.Context, input AssignInput) (AssignResult, error)
	workloadCountsFn func(ctx context.Context) (map[string]CourierStatusCounts, error)
	listAdminFn      func(ctx context.Context, filter AdminParcelFilter) (AdminParcelPage, error)
}

func (m *mockDispatchRepo) ListUnassigned(ctx context.Context, filter UnassignedFilter) (UnassignedPage, error) {
	if m.listUnassignedFn != nil {
		return m.listUnassignedFn(ctx, filter)
	}
	return UnassignedPage{}, nil
}

func (m *mockDispatchRepo) AssignCourier(ctx context.Context, input AssignInput) (AssignResult, error) {
	if m.assignCourierFn != nil {
		return m.assignCourierFn(ctx, input)
	}
	return AssignResult{}, nil
}

func (m *mockDispatchRepo) GetActiveWorkloadCounts(ctx context.Context) (map[string]CourierStatusCounts, error) {
	if m.workloadCountsFn != nil {
		return m.workloadCountsFn(ctx)
	}
	return make(map[string]CourierStatusCounts), nil
}

func (m *mockDispatchRepo) ListAdminParcels(ctx context.Context, filter AdminParcelFilter) (AdminParcelPage, error) {
	if m.listAdminFn != nil {
		return m.listAdminFn(ctx, filter)
	}
	return AdminParcelPage{}, nil
}

type mockCourierClient struct {
	eligibilityFn func(ctx context.Context, courierID string) error
	listFn        func(ctx context.Context, authHeader string, cookie *http.Cookie) ([]identityclient.Courier, error)
}

func (m *mockCourierClient) CheckEligibility(ctx context.Context, courierID string) error {
	if m.eligibilityFn != nil {
		return m.eligibilityFn(ctx, courierID)
	}
	return nil
}

func (m *mockCourierClient) ListCouriers(ctx context.Context, authHeader string, cookie *http.Cookie) ([]identityclient.Courier, error) {
	if m.listFn != nil {
		return m.listFn(ctx, authHeader, cookie)
	}
	return nil, nil
}

func TestListUnassigned(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		repo := &mockDispatchRepo{
			listUnassignedFn: func(ctx context.Context, filter UnassignedFilter) (UnassignedPage, error) {
				return UnassignedPage{
					Items: []UnassignedParcel{
						{ID: "p1", TrackingNumber: "SDK11111111", Status: "CREATED"},
					},
					Page:       1,
					PageSize:   20,
					TotalItems: 1,
				}, nil
			},
		}
		svc := NewService(repo, nil)

		res, err := svc.ListUnassigned(context.Background(), UnassignedFilter{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.TotalItems != 1 || len(res.Items) != 1 {
			t.Fatalf("unexpected result: %+v", res)
		}
	})

	t.Run("invalid zone UUID", func(t *testing.T) {
		svc := NewService(&mockDispatchRepo{}, nil)
		badZone := "not-a-uuid"
		_, err := svc.ListUnassigned(context.Background(), UnassignedFilter{ZoneID: &badZone})
		if !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("expected ErrInvalidInput, got: %v", err)
		}
	})
}

func TestAssignCourier(t *testing.T) {
	validParcelID := "3f1c7a52-0000-4000-8000-000000000001"
	validCourierID := "3f1c7a52-0000-4000-8000-000000000002"

	t.Run("success", func(t *testing.T) {
		repo := &mockDispatchRepo{
			assignCourierFn: func(ctx context.Context, input AssignInput) (AssignResult, error) {
				return AssignResult{
					ParcelID:       input.ParcelID,
					TrackingNumber: "SDK12345678",
					CourierID:      input.CourierID,
					AssignedBy:     input.ActorUserID,
					AssignedAt:     time.Now().UTC(),
				}, nil
			},
		}
		client := &mockCourierClient{}
		svc := NewService(repo, client)

		res, err := svc.AssignCourier(context.Background(), AssignInput{
			ParcelID:    validParcelID,
			CourierID:   validCourierID,
			Reason:      "initial assignment",
			ActorUserID: "admin-1",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.ParcelID != validParcelID || res.CourierID != validCourierID {
			t.Fatalf("unexpected result: %+v", res)
		}
	})

	t.Run("invalid parcel UUID", func(t *testing.T) {
		svc := NewService(&mockDispatchRepo{}, nil)
		_, err := svc.AssignCourier(context.Background(), AssignInput{
			ParcelID:  "invalid-uuid",
			CourierID: validCourierID,
		})
		if !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("expected ErrInvalidInput, got: %v", err)
		}
	})

	t.Run("invalid courier UUID", func(t *testing.T) {
		svc := NewService(&mockDispatchRepo{}, nil)
		_, err := svc.AssignCourier(context.Background(), AssignInput{
			ParcelID:  validParcelID,
			CourierID: "invalid-uuid",
		})
		if !errors.Is(err, ErrCourierNotEligible) {
			t.Fatalf("expected ErrCourierNotEligible, got: %v", err)
		}
	})

	t.Run("courier client nil fails closed with ErrServiceUnavailable", func(t *testing.T) {
		svc := NewService(&mockDispatchRepo{}, nil)
		_, err := svc.AssignCourier(context.Background(), AssignInput{
			ParcelID:  validParcelID,
			CourierID: validCourierID,
		})
		if !errors.Is(err, ErrServiceUnavailable) {
			t.Fatalf("expected ErrServiceUnavailable when client is nil, got: %v", err)
		}
	})

	t.Run("courier not eligible from identity", func(t *testing.T) {
		client := &mockCourierClient{
			eligibilityFn: func(ctx context.Context, courierID string) error {
				return identityclient.ErrCourierNotEligible
			},
		}
		svc := NewService(&mockDispatchRepo{}, client)

		_, err := svc.AssignCourier(context.Background(), AssignInput{
			ParcelID:  validParcelID,
			CourierID: validCourierID,
		})
		if !errors.Is(err, ErrCourierNotEligible) {
			t.Fatalf("expected ErrCourierNotEligible, got: %v", err)
		}
	})

	t.Run("identity service unavailable", func(t *testing.T) {
		client := &mockCourierClient{
			eligibilityFn: func(ctx context.Context, courierID string) error {
				return identityclient.ErrServiceUnavailable
			},
		}
		svc := NewService(&mockDispatchRepo{}, client)

		_, err := svc.AssignCourier(context.Background(), AssignInput{
			ParcelID:  validParcelID,
			CourierID: validCourierID,
		})
		if !errors.Is(err, ErrServiceUnavailable) {
			t.Fatalf("expected ErrServiceUnavailable, got: %v", err)
		}
	})

	t.Run("closed parcel returns 409", func(t *testing.T) {
		repo := &mockDispatchRepo{
			assignCourierFn: func(ctx context.Context, input AssignInput) (AssignResult, error) {
				return AssignResult{}, statemachine.ErrParcelClosed
			},
		}
		svc := NewService(repo, &mockCourierClient{})

		_, err := svc.AssignCourier(context.Background(), AssignInput{
			ParcelID:  validParcelID,
			CourierID: validCourierID,
		})
		if !errors.Is(err, statemachine.ErrParcelClosed) {
			t.Fatalf("expected ErrParcelClosed, got: %v", err)
		}
	})

	t.Run("parcel not found returns 404", func(t *testing.T) {
		repo := &mockDispatchRepo{
			assignCourierFn: func(ctx context.Context, input AssignInput) (AssignResult, error) {
				return AssignResult{}, ErrParcelNotFound
			},
		}
		svc := NewService(repo, &mockCourierClient{})

		_, err := svc.AssignCourier(context.Background(), AssignInput{
			ParcelID:  validParcelID,
			CourierID: validCourierID,
		})
		if !errors.Is(err, ErrParcelNotFound) {
			t.Fatalf("expected ErrParcelNotFound, got: %v", err)
		}
	})
}

func TestGetWorkload(t *testing.T) {
	t.Run("success merging couriers and counts", func(t *testing.T) {
		repo := &mockDispatchRepo{
			workloadCountsFn: func(ctx context.Context) (map[string]CourierStatusCounts, error) {
				return map[string]CourierStatusCounts{
					"c1": {
						Total: 2,
						ByStatus: map[string]int{
							"CREATED":   1,
							"PICKED_UP": 1,
						},
					},
				}, nil
			},
		}
		client := &mockCourierClient{
			listFn: func(ctx context.Context, authHeader string, cookie *http.Cookie) ([]identityclient.Courier, error) {
				return []identityclient.Courier{
					{ID: "c1", FullName: "Kasun Silva", IsAvailable: true},
					{ID: "c2", FullName: "Amal Fernando", IsAvailable: false},
				}, nil
			},
		}
		svc := NewService(repo, client)

		workload, err := svc.GetWorkload(context.Background(), "Bearer token", nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(workload) != 2 {
			t.Fatalf("expected 2 couriers in workload, got %d", len(workload))
		}

		// Courier 1 has 2 active deliveries
		if workload[0].CourierID != "c1" || workload[0].ActiveDeliveries != 2 {
			t.Errorf("courier 1 activeDeliveries = %d, want 2", workload[0].ActiveDeliveries)
		}
		if workload[0].ByStatus["CREATED"] != 1 || workload[0].ByStatus["PICKED_UP"] != 1 {
			t.Errorf("courier 1 byStatus = %+v", workload[0].ByStatus)
		}

		// Courier 2 has 0 active deliveries
		if workload[1].CourierID != "c2" || workload[1].ActiveDeliveries != 0 {
			t.Errorf("courier 2 activeDeliveries = %d, want 0", workload[1].ActiveDeliveries)
		}
	})

	t.Run("courier client nil fails closed with ErrServiceUnavailable", func(t *testing.T) {
		svc := NewService(&mockDispatchRepo{}, nil)
		_, err := svc.GetWorkload(context.Background(), "Bearer token", nil)
		if !errors.Is(err, ErrServiceUnavailable) {
			t.Fatalf("expected ErrServiceUnavailable when client is nil, got: %v", err)
		}
	})

	t.Run("identity failure returns ErrServiceUnavailable", func(t *testing.T) {
		repo := &mockDispatchRepo{}
		client := &mockCourierClient{
			listFn: func(ctx context.Context, authHeader string, cookie *http.Cookie) ([]identityclient.Courier, error) {
				return nil, errors.New("connection refused")
			},
		}
		svc := NewService(repo, client)

		_, err := svc.GetWorkload(context.Background(), "Bearer token", nil)
		if !errors.Is(err, ErrServiceUnavailable) {
			t.Fatalf("expected ErrServiceUnavailable, got: %v", err)
		}
	})
}

func TestListAdminParcels(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		repo := &mockDispatchRepo{
			listAdminFn: func(ctx context.Context, filter AdminParcelFilter) (AdminParcelPage, error) {
				return AdminParcelPage{
					Items: []AdminParcelItem{
						{ID: "p1", TrackingNumber: "SDK11111111", Status: "CREATED", Total: 550.0},
					},
					Page:       1,
					PageSize:   20,
					TotalItems: 1,
				}, nil
			},
		}
		svc := NewService(repo, nil)

		res, err := svc.ListAdminParcels(context.Background(), AdminParcelFilter{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.TotalItems != 1 {
			t.Fatalf("expected 1 item, got %d", res.TotalItems)
		}
	})

	t.Run("invalid UUID filter", func(t *testing.T) {
		svc := NewService(&mockDispatchRepo{}, nil)
		badUUID := "bad"
		_, err := svc.ListAdminParcels(context.Background(), AdminParcelFilter{CourierID: &badUUID})
		if !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("expected ErrInvalidInput, got: %v", err)
		}
	})
}
