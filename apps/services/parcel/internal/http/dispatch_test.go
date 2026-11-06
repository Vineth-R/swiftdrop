package servicehttp

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Vineth-R/swiftdrop/apps/services/parcel/internal/dispatch"
	"github.com/Vineth-R/swiftdrop/packages/go-common/auth"
)

type mockDispatchService struct {
	unassignedFn func(ctx context.Context, filter dispatch.UnassignedFilter) (dispatch.UnassignedPage, error)
	assignFn     func(ctx context.Context, input dispatch.AssignInput) (dispatch.AssignResult, error)
	workloadFn   func(ctx context.Context, authHeader string, cookie *http.Cookie) ([]dispatch.CourierWorkload, error)
	adminFn      func(ctx context.Context, filter dispatch.AdminParcelFilter) (dispatch.AdminParcelPage, error)
}

func (m *mockDispatchService) ListUnassigned(ctx context.Context, filter dispatch.UnassignedFilter) (dispatch.UnassignedPage, error) {
	if m.unassignedFn != nil {
		return m.unassignedFn(ctx, filter)
	}
	return dispatch.UnassignedPage{}, nil
}

func (m *mockDispatchService) AssignCourier(ctx context.Context, input dispatch.AssignInput) (dispatch.AssignResult, error) {
	if m.assignFn != nil {
		return m.assignFn(ctx, input)
	}
	return dispatch.AssignResult{}, nil
}

func (m *mockDispatchService) GetWorkload(ctx context.Context, authHeader string, cookie *http.Cookie) ([]dispatch.CourierWorkload, error) {
	if m.workloadFn != nil {
		return m.workloadFn(ctx, authHeader, cookie)
	}
	return nil, nil
}

func (m *mockDispatchService) ListAdminParcels(ctx context.Context, filter dispatch.AdminParcelFilter) (dispatch.AdminParcelPage, error) {
	if m.adminFn != nil {
		return m.adminFn(ctx, filter)
	}
	return dispatch.AdminParcelPage{}, nil
}

var testAccessSecret = strings.Repeat("s", 32)

func testToken(t *testing.T, role auth.Role) string {
	t.Helper()
	tm, err := auth.NewTokenManager(testAccessSecret, 15*time.Minute)
	if err != nil {
		t.Fatalf("NewTokenManager() error: %v", err)
	}
	token, _, err := tm.Issue("user-123", role)
	if err != nil {
		t.Fatalf("Issue() error: %v", err)
	}
	return token
}

func setupTestRouter(t *testing.T, svc DispatchService, accessSecret string) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	router, err := New(logger, Options{
		Dispatch:     svc,
		AccessSecret: accessSecret,
	})
	if err != nil {
		t.Fatalf("New() router error: %v", err)
	}
	return router
}

func executeRequest(router *gin.Engine, req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestDispatchUnassignedHandler(t *testing.T) {
	svc := &mockDispatchService{
		unassignedFn: func(ctx context.Context, filter dispatch.UnassignedFilter) (dispatch.UnassignedPage, error) {
			return dispatch.UnassignedPage{
				Items: []dispatch.UnassignedParcel{
					{
						ID:             "p1",
						TrackingNumber: "SDK12345678",
						Status:         "CREATED",
						PickupCity:     "Colombo",
						DropoffCity:    "Kandy",
						WeightKG:       2.5,
						SizeCategory:   "SMALL",
					},
				},
				Page:       filter.Page,
				PageSize:   filter.PageSize,
				TotalItems: 1,
			}, nil
		},
	}
	router := setupTestRouter(t, svc, testAccessSecret)

	t.Run("401 when no token", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, "/dispatch/unassigned", nil)
		rec := executeRequest(router, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want 401", rec.Code)
		}
	})

	t.Run("403 for courier or customer", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, "/dispatch/unassigned", nil)
		req.Header.Set("Authorization", "Bearer "+testToken(t, auth.RoleCourier))
		rec := executeRequest(router, req)
		if rec.Code != http.StatusForbidden {
			t.Errorf("status = %d, want 403", rec.Code)
		}
	})

	t.Run("200 for dispatcher", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, "/dispatch/unassigned?page=1&pageSize=10", nil)
		req.Header.Set("Authorization", "Bearer "+testToken(t, auth.RoleDispatcher))
		rec := executeRequest(router, req)
		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want 200", rec.Code)
		}
	})

	t.Run("200 for admin", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, "/dispatch/unassigned", nil)
		req.Header.Set("Authorization", "Bearer "+testToken(t, auth.RoleAdmin))
		rec := executeRequest(router, req)
		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want 200", rec.Code)
		}
	})
}

func TestAssignCourierHandler(t *testing.T) {
	validParcelID := "3f1c7a52-0000-4000-8000-000000000001"
	validCourierID := "3f1c7a52-0000-4000-8000-000000000002"

	tests := []struct {
		name       string
		role       auth.Role
		parcelID   string
		body       string
		svcErr     error
		wantStatus int
	}{
		{
			name:       "success",
			role:       auth.RoleDispatcher,
			parcelID:   validParcelID,
			body:       `{"courierId":"` + validCourierID + `","reason":"reassignment"}`,
			wantStatus: http.StatusOK,
		},
		{
			name:       "parcel not found 404",
			role:       auth.RoleDispatcher,
			parcelID:   validParcelID,
			body:       `{"courierId":"` + validCourierID + `"}`,
			svcErr:     dispatch.ErrParcelNotFound,
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "parcel closed 409",
			role:       auth.RoleDispatcher,
			parcelID:   validParcelID,
			body:       `{"courierId":"` + validCourierID + `"}`,
			svcErr:     dispatch.ErrParcelClosed,
			wantStatus: http.StatusConflict,
		},
		{
			name:       "courier not eligible 422",
			role:       auth.RoleDispatcher,
			parcelID:   validParcelID,
			body:       `{"courierId":"` + validCourierID + `"}`,
			svcErr:     dispatch.ErrCourierNotEligible,
			wantStatus: http.StatusUnprocessableEntity,
		},
		{
			name:       "identity unavailable 503",
			role:       auth.RoleDispatcher,
			parcelID:   validParcelID,
			body:       `{"courierId":"` + validCourierID + `"}`,
			svcErr:     dispatch.ErrServiceUnavailable,
			wantStatus: http.StatusServiceUnavailable,
		},
		{
			name:       "bad json body 400",
			role:       auth.RoleDispatcher,
			parcelID:   validParcelID,
			body:       `{not-valid-json`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "forbidden role 403",
			role:       auth.RoleCustomer,
			parcelID:   validParcelID,
			body:       `{"courierId":"` + validCourierID + `"}`,
			wantStatus: http.StatusForbidden,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := &mockDispatchService{
				assignFn: func(ctx context.Context, input dispatch.AssignInput) (dispatch.AssignResult, error) {
					if tc.svcErr != nil {
						return dispatch.AssignResult{}, tc.svcErr
					}
					return dispatch.AssignResult{
						ParcelID:       input.ParcelID,
						TrackingNumber: "SDK12345678",
						CourierID:      input.CourierID,
						AssignedBy:     input.ActorUserID,
						AssignedAt:     time.Now().UTC(),
					}, nil
				},
			}
			router := setupTestRouter(t, svc, testAccessSecret)

			req, _ := http.NewRequest(http.MethodPut, "/dispatch/parcels/"+tc.parcelID+"/assignment", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+testToken(t, tc.role))

			rec := executeRequest(router, req)
			if rec.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tc.wantStatus)
			}
		})
	}
}

func TestWorkloadHandler(t *testing.T) {
	svc := &mockDispatchService{
		workloadFn: func(ctx context.Context, authHeader string, cookie *http.Cookie) ([]dispatch.CourierWorkload, error) {
			return []dispatch.CourierWorkload{
				{
					CourierID:        "c1",
					FullName:         "Kasun Silva",
					IsAvailable:      true,
					ActiveDeliveries: 3,
					ByStatus: map[string]int{
						"CREATED":   1,
						"PICKED_UP": 2,
					},
				},
			}, nil
		},
	}
	router := setupTestRouter(t, svc, testAccessSecret)

	req, _ := http.NewRequest(http.MethodGet, "/dispatch/workload", nil)
	req.Header.Set("Authorization", "Bearer "+testToken(t, auth.RoleDispatcher))
	rec := executeRequest(router, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

func TestAdminParcelsHandler(t *testing.T) {
	svc := &mockDispatchService{
		adminFn: func(ctx context.Context, filter dispatch.AdminParcelFilter) (dispatch.AdminParcelPage, error) {
			return dispatch.AdminParcelPage{
				Items: []dispatch.AdminParcelItem{
					{
						ID:             "p1",
						TrackingNumber: "SDK12345678",
						Status:         "CREATED",
						Total:          550.0,
					},
				},
				Page:       1,
				PageSize:   20,
				TotalItems: 1,
			}, nil
		},
	}
	router := setupTestRouter(t, svc, testAccessSecret)

	for _, path := range []string{"/admin/parcels", "/admin/parcels/"} {
		t.Run(path, func(t *testing.T) {
			req, _ := http.NewRequest(http.MethodGet, path+"?status=CREATED", nil)
			req.Header.Set("Authorization", "Bearer "+testToken(t, auth.RoleAdmin))
			rec := executeRequest(router, req)

			if rec.Code != http.StatusOK {
				t.Errorf("%s status = %d, want 200", path, rec.Code)
			}
		})
	}
}

func TestDispatchProbes(t *testing.T) {
	router := setupTestRouter(t, &mockDispatchService{}, testAccessSecret)

	for _, path := range []string{"/dispatch/probe", "/admin/parcels/probe"} {
		req, _ := http.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Authorization", "Bearer "+testToken(t, auth.RoleDispatcher))
		rec := executeRequest(router, req)

		if rec.Code != http.StatusOK {
			t.Errorf("%s probe status = %d, want 200", path, rec.Code)
		}
	}
}

func TestDispatchFailClosedWithoutAccessSecret(t *testing.T) {
	router := setupTestRouter(t, &mockDispatchService{}, "") // Unset access secret

	for _, path := range []string{"/dispatch/unassigned", "/dispatch/workload", "/admin/parcels"} {
		req, _ := http.NewRequest(http.MethodGet, path, nil)
		rec := executeRequest(router, req)

		if rec.Code != http.StatusServiceUnavailable {
			t.Errorf("%s status = %d, want 503", path, rec.Code)
		}
	}
}

func TestDispatchValidationErrors(t *testing.T) {
	router := setupTestRouter(t, &mockDispatchService{}, testAccessSecret)

	t.Run("unassigned invalid query params return 422", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, "/dispatch/unassigned?page=bad&pageSize=200&from=not-a-date&zoneId=bad-uuid", nil)
		req.Header.Set("Authorization", "Bearer "+testToken(t, auth.RoleDispatcher))
		rec := executeRequest(router, req)

		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("status = %d, want 422", rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "VALIDATION_ERROR") {
			t.Errorf("body missing VALIDATION_ERROR: %s", rec.Body.String())
		}
	})

	t.Run("admin parcels invalid query params return 422", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, "/admin/parcels?page=-1&courierId=not-a-uuid&from=invalid", nil)
		req.Header.Set("Authorization", "Bearer "+testToken(t, auth.RoleAdmin))
		rec := executeRequest(router, req)

		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("status = %d, want 422", rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "VALIDATION_ERROR") {
			t.Errorf("body missing VALIDATION_ERROR: %s", rec.Body.String())
		}
	})

	t.Run("assign courier payload too large returns 413", func(t *testing.T) {
		bigBody := `{"courierId":"c1","reason":"` + strings.Repeat("A", 17000) + `"}`
		req, _ := http.NewRequest(http.MethodPut, "/dispatch/parcels/3f1c7a52-0000-4000-8000-000000000001/assignment", strings.NewReader(bigBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+testToken(t, auth.RoleDispatcher))
		rec := executeRequest(router, req)

		if rec.Code != http.StatusRequestEntityTooLarge {
			t.Fatalf("status = %d, want 413", rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "PAYLOAD_TOO_LARGE") {
			t.Errorf("body missing PAYLOAD_TOO_LARGE: %s", rec.Body.String())
		}
	})
}
