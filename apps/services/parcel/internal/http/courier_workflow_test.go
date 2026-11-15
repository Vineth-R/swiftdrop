package servicehttp

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Vineth-R/swiftdrop/apps/services/parcel/internal/courier"
	"github.com/Vineth-R/swiftdrop/apps/services/parcel/internal/statemachine"
	"github.com/Vineth-R/swiftdrop/packages/go-common/auth"
)

type courierWorkflowFixture struct {
	pickup         courier.PickupInput
	delivery       courier.DeliveryInput
	pickupErr      error
	deliveryErr    error
	pickupCalls    int
	deliveryCalls  int
	pickupResult   courier.WorkflowResult
	deliveryResult courier.WorkflowResult
}

func (f *courierWorkflowFixture) ConfirmPickup(_ context.Context, input courier.PickupInput) (courier.WorkflowResult, error) {
	f.pickup, f.pickupCalls = input, f.pickupCalls+1
	if f.pickupErr != nil {
		return courier.WorkflowResult{}, f.pickupErr
	}
	return f.pickupResult, nil
}

func (f *courierWorkflowFixture) ConfirmDelivery(_ context.Context, input courier.DeliveryInput) (courier.WorkflowResult, error) {
	f.delivery, f.deliveryCalls = input, f.deliveryCalls+1
	if f.deliveryErr != nil {
		return courier.WorkflowResult{}, f.deliveryErr
	}
	return f.deliveryResult, nil
}

func workflowRouter(t *testing.T, store CourierWorkflowStore) *gin.Engine {
	t.Helper()
	return reportRouter(t, Options{AccessSecret: reportTestSecret, SessionCheck: allowSessions(),
		CourierWorkflow: store})
}

func workflowRequest(t *testing.T, router *gin.Engine, role auth.Role, path string, body []byte, contentType string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	req.Header.Set("Content-Type", contentType)
	if role != "" {
		manager, err := auth.NewTokenManager(reportTestSecret, time.Hour)
		if err != nil {
			t.Fatal(err)
		}
		token, _, err := manager.Issue(courierTestID, role)
		if err != nil {
			t.Fatal(err)
		}
		req.AddCookie(&http.Cookie{Name: "sd_access", Value: token})
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	return recorder
}

func workflowPNG(t *testing.T) []byte {
	t.Helper()
	var data bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.White)
	if err := png.Encode(&data, img); err != nil {
		t.Fatal(err)
	}
	return data.Bytes()
}

func TestCourierWorkflowAuthorization(t *testing.T) {
	for _, tc := range []struct {
		name string
		role auth.Role
		want int
	}{
		{name: "signed out", want: 401},
		{name: "customer", role: auth.RoleCustomer, want: 403},
		{name: "merchant", role: auth.RoleMerchant, want: 403},
		{name: "dispatcher", role: auth.RoleDispatcher, want: 403},
		{name: "admin", role: auth.RoleAdmin, want: 403},
		{name: "courier", role: auth.RoleCourier, want: 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &courierWorkflowFixture{
				pickupResult: courier.WorkflowResult{ParcelID: courierParcelID,
					TrackingNumber: "SDK7M2Q9XP4", PreviousStatus: statemachine.StatusCreated,
					Status: statemachine.StatusInTransit, UpdatedAt: time.Now().UTC(),
					History: []courier.History{}},
			}
			body, contentType := proofForm(t, map[string]string{"note": "At the shop"},
				workflowPNG(t), "image/png")
			recorder := workflowRequest(t, workflowRouter(t, store), tc.role,
				"/courier/deliveries/"+courierParcelID+"/pickup", body, contentType)
			if recorder.Code != tc.want {
				t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
			}
			if tc.want == 200 && store.pickup.CourierID != courierTestID {
				t.Fatalf("courier = %q", store.pickup.CourierID)
			}
		})
	}
}

func TestCourierPickupPassesPhotoAndNote(t *testing.T) {
	store := &courierWorkflowFixture{
		pickupResult: courier.WorkflowResult{
			ParcelID: courierParcelID, TrackingNumber: "SDK7M2Q9XP4",
			PreviousStatus: statemachine.StatusCreated, Status: statemachine.StatusInTransit,
			UpdatedAt: time.Now().UTC(), History: []courier.History{},
		},
	}
	body, contentType := proofForm(t, map[string]string{"note": " At the shop "},
		workflowPNG(t), "image/png")
	recorder := workflowRequest(t, workflowRouter(t, store), auth.RoleCourier,
		"/courier/deliveries/"+courierParcelID+"/pickup", body, contentType)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
	}
	if store.pickup.ParcelID != courierParcelID || store.pickup.Note != " At the shop " ||
		store.pickup.Photo == nil || store.pickup.Photo.MIMEType != "image/png" {
		t.Fatalf("input = %+v", store.pickup)
	}
	var response struct {
		Data courier.WorkflowResult `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Data.Status != statemachine.StatusInTransit {
		t.Fatalf("data = %+v", response.Data)
	}
}

func TestCourierDeliveryPassesRecipientAndPhoto(t *testing.T) {
	store := &courierWorkflowFixture{
		deliveryResult: courier.WorkflowResult{
			ParcelID: courierParcelID, TrackingNumber: "SDK7M2Q9XP4",
			PreviousStatus: statemachine.StatusOutForDelivery,
			Status:         statemachine.StatusDelivered, UpdatedAt: time.Now().UTC(),
			History: []courier.History{},
		},
	}
	body, contentType := proofForm(t, map[string]string{
		"recipientName": " Amali Fernando ", "note": "Handed over",
	}, workflowPNG(t), "image/png")
	recorder := workflowRequest(t, workflowRouter(t, store), auth.RoleCourier,
		"/courier/deliveries/"+courierParcelID+"/delivery", body, contentType)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
	}
	if store.delivery.RecipientName != " Amali Fernando " || store.delivery.Note != "Handed over" ||
		store.delivery.Photo == nil {
		t.Fatalf("input = %+v", store.delivery)
	}
}

func TestCourierWorkflowErrorMapping(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want int
		code string
	}{
		{name: "photo required", err: courier.ErrPhotoRequired, want: 422, code: "PROOF_REQUIRED"},
		{name: "recipient required", err: statemachine.ErrRecipientRequired, want: 422, code: "RECIPIENT_REQUIRED"},
		{name: "invalid transition", err: statemachine.ErrInvalidTransition, want: 409, code: "INVALID_TRANSITION"},
		{name: "forbidden", err: courier.ErrForbidden, want: 403, code: "FORBIDDEN"},
		{name: "not found", err: courier.ErrNotFound, want: 404, code: "PARCEL_NOT_FOUND"},
		{name: "unsupported media", err: courier.ErrProofUnsupported, want: 415, code: "UNSUPPORTED_MEDIA_TYPE"},
		{name: "proof limit", err: courier.ErrProofLimitReached, want: 409, code: "PROOF_LIMIT_REACHED"},
		{name: "unavailable", err: courier.ErrUnavailable, want: 503, code: "SERVICE_UNAVAILABLE"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &courierWorkflowFixture{pickupErr: tc.err}
			body, contentType := proofForm(t, nil, workflowPNG(t), "image/png")
			recorder := workflowRequest(t, workflowRouter(t, store), auth.RoleCourier,
				"/courier/deliveries/"+courierParcelID+"/pickup", body, contentType)
			if recorder.Code != tc.want || !strings.Contains(recorder.Body.String(), tc.code) {
				t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
			}
		})
	}
}

func TestCourierWorkflowRejectsMalformedRequests(t *testing.T) {
	store := &courierWorkflowFixture{pickupResult: courier.WorkflowResult{}}
	router := workflowRouter(t, store)

	t.Run("json body", func(t *testing.T) {
		recorder := workflowRequest(t, router, auth.RoleCourier,
			"/courier/deliveries/"+courierParcelID+"/pickup",
			[]byte(`{"status":"PICKED_UP"}`), "application/json")
		if recorder.Code != http.StatusUnsupportedMediaType {
			t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
		}
	})

	t.Run("unknown field", func(t *testing.T) {
		body, contentType := proofForm(t, map[string]string{"courierId": "x"}, nil, "")
		recorder := workflowRequest(t, router, auth.RoleCourier,
			"/courier/deliveries/"+courierParcelID+"/pickup", body, contentType)
		if recorder.Code != http.StatusUnprocessableEntity {
			t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
		}
	})

	if store.pickupCalls != 0 {
		t.Fatal("malformed request reached the store")
	}
}

func TestCourierWorkflowMissingConfigFailsClosed(t *testing.T) {
	// No access secret means authentication is not configured.
	router := reportRouter(t, Options{CourierWorkflow: &courierWorkflowFixture{}})
	body, contentType := proofForm(t, nil, workflowPNG(t), "image/png")
	recorder := workflowRequest(t, router, auth.RoleCourier,
		"/courier/deliveries/"+courierParcelID+"/pickup", body, contentType)
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
	}
}
