package servicehttp

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Vineth-R/swiftdrop/apps/services/merchant/internal/identityclient"
	"github.com/Vineth-R/swiftdrop/apps/services/merchant/internal/onboarding"
	"github.com/Vineth-R/swiftdrop/apps/services/merchant/internal/pricingclient"
	"github.com/Vineth-R/swiftdrop/apps/services/merchant/internal/repository"
)

const registrationZoneID = "7a1e0000-0000-4000-8000-000000000001"

type registrationStore struct {
	merchant repository.Merchant
	attempt  repository.RegistrationAttempt
}

func (s *registrationStore) EnsureRegistrationAttempt(_ context.Context, id, key, email string) (repository.RegistrationAttempt, error) {
	s.attempt = repository.RegistrationAttempt{ID: id, IdempotencyKey: key, Email: email, Status: "PENDING"}
	return s.attempt, nil
}
func (s *registrationStore) SetRegistrationAttemptUser(_ context.Context, _, userID string) error {
	s.attempt.UserID = &userID
	return nil
}
func (s *registrationStore) CreatePendingMerchant(_ context.Context, _ string, params repository.CreateMerchantParams) (repository.Merchant, error) {
	s.merchant = repository.Merchant{
		ID: params.ID, OwnerUserID: params.OwnerUserID, BusinessName: params.BusinessName,
		BusinessRegNo: &params.BusinessRegNo, ContactEmail: params.ContactEmail,
		Status: repository.MerchantStatusPending, CreatedAt: time.Now(),
	}
	merchantID := s.merchant.ID
	s.attempt.MerchantID = &merchantID
	s.attempt.Status = "COMPLETED"
	return s.merchant, nil
}
func (s *registrationStore) CompleteRegistrationAttempt(_ context.Context, _, merchantID string) error {
	s.attempt.MerchantID = &merchantID
	return nil
}
func (s *registrationStore) FailRegistrationAttempt(context.Context, string, string) error {
	return nil
}
func (s *registrationStore) MerchantByID(context.Context, string) (repository.Merchant, error) {
	return s.merchant, nil
}
func (s *registrationStore) BeginApprovalTransition(context.Context, string, string, string) (repository.Merchant, repository.StatusTransition, error) {
	return repository.Merchant{}, repository.StatusTransition{}, repository.ErrNotFound
}
func (s *registrationStore) ApplyApprovalTransition(context.Context, string, string, string) (repository.Merchant, error) {
	return repository.Merchant{}, repository.ErrNotFound
}
func (s *registrationStore) BeginRejectionTransition(context.Context, string, string, string, *string) (repository.Merchant, repository.StatusTransition, error) {
	return repository.Merchant{}, repository.StatusTransition{}, repository.ErrNotFound
}
func (s *registrationStore) BeginSuspensionTransition(context.Context, string, string, string, *string) (repository.Merchant, repository.StatusTransition, error) {
	return repository.Merchant{}, repository.StatusTransition{}, repository.ErrNotFound
}
func (s *registrationStore) MarkTransitionResult(context.Context, string, bool, string) error {
	return nil
}
func (s *registrationStore) LatestTransition(context.Context, string, repository.MerchantStatus) (repository.StatusTransition, error) {
	return repository.StatusTransition{}, repository.ErrNotFound
}

type registrationIdentity struct{}

func (registrationIdentity) CreateMerchantOwner(context.Context, identityclient.OwnerInput) (identityclient.OwnerResult, error) {
	return identityclient.OwnerResult{UserID: "user-1", Created: true}, nil
}
func (registrationIdentity) ApplyLifecycle(context.Context, string, identityclient.LifecycleInput) (identityclient.LifecycleResult, error) {
	return identityclient.LifecycleResult{}, nil
}
func (registrationIdentity) DeleteMerchantOwner(context.Context, string) error { return nil }
func (registrationIdentity) Contact(context.Context, string) (identityclient.Contact, error) {
	return identityclient.Contact{}, nil
}

type registrationPricing struct{}

func (registrationPricing) Zone(context.Context, string) (pricingclient.Zone, error) {
	return pricingclient.Zone{ID: registrationZoneID, IsActive: true}, nil
}

type registrationNotifier struct{}

func (registrationNotifier) Received(context.Context, string, string, string, string, time.Time) {}
func (registrationNotifier) Approved(context.Context, string, string, string, time.Time)         {}
func (registrationNotifier) Rejected(context.Context, string, string, string, *string)           {}
func (registrationNotifier) Suspended(context.Context, string, string, string, *string)          {}

func registrationPayload() map[string]any {
	return map[string]any{
		"fullName": "Ruwan Jayasuriya",
		"email":    "owner@shop.lk",
		"phone":    "+94773334455",
		"password": "Str0ngPass!",
		"business": map[string]any{
			"name":               "ShopZone Lanka",
			"registrationNumber": "pv   123456",
			"contactEmail":       "support@shop.lk",
			"contactPhone":       "+94112223344",
			"businessAddress":    "88 Galle Road, Colombo 03",
			"pickupAddress":      "88 Galle Road, Colombo 03",
			"pickupZoneId":       registrationZoneID,
		},
	}
}

func registrationRouter(t *testing.T) (*gin.Engine, *registrationStore) {
	t.Helper()
	store := &registrationStore{}
	service, err := onboarding.New(store, registrationIdentity{}, registrationPricing{}, registrationNotifier{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	registerMerchantRoutes(router, Options{
		Onboarding: service,
		Logger:     slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	return router, store
}

func postRegistration(t *testing.T, router *gin.Engine, payload map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/merchants/register", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}

func assertRegistrationValidation(t *testing.T, recorder *httptest.ResponseRecorder) {
	t.Helper()
	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Error struct {
			Code    string `json:"code"`
			Details []struct {
				Field string `json:"field"`
			} `json:"details"`
		} `json:"error"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Error.Code != "VALIDATION_ERROR" {
		t.Fatalf("error code = %q", response.Error.Code)
	}
	for _, detail := range response.Error.Details {
		if detail.Field == "business.registrationNumber" {
			return
		}
	}
	t.Fatalf("missing registrationNumber detail: %s", recorder.Body.String())
}

func TestMerchantRegistrationRequiresRegistrationNumber(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(map[string]any)
	}{
		{name: "missing", change: func(payload map[string]any) {
			delete(payload["business"].(map[string]any), "registrationNumber")
		}},
		{name: "null", change: func(payload map[string]any) {
			payload["business"].(map[string]any)["registrationNumber"] = nil
		}},
		{name: "empty", change: func(payload map[string]any) {
			payload["business"].(map[string]any)["registrationNumber"] = ""
		}},
		{name: "whitespace", change: func(payload map[string]any) {
			payload["business"].(map[string]any)["registrationNumber"] = " \t "
		}},
		{name: "invalid", change: func(payload map[string]any) {
			payload["business"].(map[string]any)["registrationNumber"] = "PV_123456"
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			router, store := registrationRouter(t)
			payload := registrationPayload()
			tc.change(payload)
			recorder := postRegistration(t, router, payload)
			assertRegistrationValidation(t, recorder)
			if store.merchant.ID != "" || store.attempt.ID != "" {
				t.Fatalf("invalid registration reached the store: merchant=%+v attempt=%+v", store.merchant, store.attempt)
			}
		})
	}
}

func TestMerchantRegistrationAcceptsAndNormalizesValidRegistrationNumber(t *testing.T) {
	router, store := registrationRouter(t)
	recorder := postRegistration(t, router, registrationPayload())
	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if store.merchant.BusinessRegNo == nil || *store.merchant.BusinessRegNo != "PV 123456" {
		t.Fatalf("registration number = %v", store.merchant.BusinessRegNo)
	}
}
