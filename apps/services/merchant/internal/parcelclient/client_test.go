package parcelclient

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Vineth-R/swiftdrop/apps/services/merchant/internal/orders"
)

func testInput() orders.ParcelInput {
	return orders.ParcelInput{
		MerchantID: "11111111-1111-4111-8111-111111111111", CreatedByUserID: "22222222-2222-4222-8222-222222222222",
		ExternalOrderID: "SZ-100245", IdempotencyKey: "key-1",
		Pickup:    orders.Contact{Name: "Shop", Phone: "+94112223344", Address: "1 Road", City: "Colombo", ZoneID: "33333333-3333-4333-8333-333333333333"},
		Recipient: orders.Contact{Name: "Amali", Phone: "+94712345678", Address: "2 Road", City: "Kandy", ZoneID: "44444444-4444-4444-8444-444444444444"},
		WeightKG:  150, SizeCategory: "SMALL", Description: ptr("Phone case"),
	}
}

func ptr(value string) *string { return &value }

func TestCreateMerchantSendsTheRealContract(t *testing.T) {
	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/parcels" || r.Method != http.MethodPost {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("X-Internal-Token") != "internal-token" {
			t.Errorf("internal token = %q", r.Header.Get("X-Internal-Token"))
		}
		if r.Header.Get("X-Request-Id") == "" {
			t.Error("request id missing")
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"parcelId":"55555555-5555-4555-8555-555555555555","trackingNumber":"SDN4T8W2LC6","status":"CREATED","fee":{"currency":"LKR","baseFee":450.00,"weightCharge":100.00,"sizeSurcharge":0.00,"total":550.00}}}`)
	}))
	defer server.Close()

	client, err := New(server.URL+"/", "internal-token", time.Second)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	result, err := client.CreateMerchant(context.Background(), testInput())
	if err != nil {
		t.Fatalf("CreateMerchant() error = %v", err)
	}
	if result.ParcelID != "55555555-5555-4555-8555-555555555555" || result.TrackingNumber != "SDN4T8W2LC6" || result.Fee.Total != 55000 {
		t.Fatalf("result = %+v", result)
	}
	if body["merchantId"] != "11111111-1111-4111-8111-111111111111" || body["createdByUserId"] != "22222222-2222-4222-8222-222222222222" {
		t.Fatalf("identity body = %+v", body)
	}
	if body["externalOrderId"] != "SZ-100245" || body["idempotencyKey"] != "key-1" {
		t.Fatalf("idempotency body = %+v", body)
	}
	if body["weightKg"] != 1.5 {
		t.Fatalf("weightKg = %v", body["weightKg"])
	}
}

func TestCreateMerchantMapsParcelValidation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = io.WriteString(w, `{"error":{"code":"ROUTE_NOT_SERVICEABLE","message":"no route","details":[{"field":"recipient.zoneId","issue":"is not serviceable"}]}}`)
	}))
	defer server.Close()
	client, _ := New(server.URL, "", time.Second)

	_, err := client.CreateMerchant(context.Background(), testInput())
	var upstream *orders.UpstreamInvalid
	if !errors.As(err, &upstream) || upstream.Code != "ROUTE_NOT_SERVICEABLE" || len(upstream.Issues) != 1 {
		t.Fatalf("error = %v, want upstream validation", err)
	}
}

func TestCreateMerchantMapsConflictsAndOutages(t *testing.T) {
	for _, test := range []struct {
		name   string
		status int
		body   string
		want   error
	}{
		{"conflict", http.StatusConflict, `{"error":{"code":"IDEMPOTENCY_CONFLICT"}}`, orders.ErrDuplicateOrder},
		{"unauthorized", http.StatusUnauthorized, `{"error":{"code":"UNAUTHENTICATED"}}`, orders.ErrParcelUnavailable},
		{"server error", http.StatusInternalServerError, `{"error":{"code":"INTERNAL_ERROR"}}`, orders.ErrParcelUnavailable},
		{"invalid success body", http.StatusCreated, `not-json`, orders.ErrParcelUnavailable},
		{"missing parcel id", http.StatusCreated, `{"data":{"trackingNumber":"SDN4T8W2LC6"}}`, orders.ErrParcelUnavailable},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(test.status)
				_, _ = io.WriteString(w, test.body)
			}))
			defer server.Close()
			client, _ := New(server.URL, "", time.Second)
			_, err := client.CreateMerchant(context.Background(), testInput())
			if !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestCreateMerchantTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()
	client, _ := New(server.URL, "", 10*time.Millisecond)
	if _, err := client.CreateMerchant(context.Background(), testInput()); !errors.Is(err, orders.ErrParcelUnavailable) {
		t.Fatalf("error = %v, want ErrParcelUnavailable", err)
	}
}

func TestNewRejectsInvalidBaseURL(t *testing.T) {
	for _, value := range []string{"", "parcel:8082", "ftp://parcel"} {
		if _, err := New(value, "", time.Second); err == nil {
			t.Fatalf("New(%q) accepted an invalid URL", value)
		}
	}
}
