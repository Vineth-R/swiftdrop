package parcels

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Vineth-R/swiftdrop/packages/go-common/requestid"
	"github.com/gin-gonic/gin"
)

func TestPricingAdapter(t *testing.T) {
	for _, tc := range []struct {
		name        string
		status      int
		body        string
		code        string
		unavailable bool
	}{{"quote", 200, `{"data":{"currency":"LKR","baseFee":450,"weightCharge":100,"sizeSurcharge":0,"total":550}}`, "", false}, {"route", 422, `{"error":{"code":"ROUTE_NOT_SERVICEABLE"}}`, "ROUTE_NOT_SERVICEABLE", false}, {"weight", 422, `{"error":{"code":"WEIGHT_NOT_PRICED"}}`, "WEIGHT_NOT_PRICED", false}, {"validation", 422, `{"error":{"code":"VALIDATION_ERROR"}}`, "VALIDATION_ERROR", false}, {"zone", 404, `{"error":{"code":"ZONE_NOT_FOUND"}}`, "zone", false}, {"down", 503, `{"error":{"code":"SERVICE_UNAVAILABLE"}}`, "", true}, {"malformed", 200, `broken`, "", true}, {"trailing", 200, `{"data":{}} {}`, "", true}} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/internal/pricing/quote" || r.Method != "POST" || r.Header.Get("X-Internal-Token") != "test-internal-token" {
					t.Errorf("incorrect internal request")
				}
				var input map[string]any
				if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
					t.Error(err)
				}
				if input["originZoneId"] != testUser || input["destinationZoneId"] != testOther || input["weightKg"] != float64(2) {
					t.Errorf("wrong input %+v", input)
				}
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			fee, err := NewPricingClient(server.URL, "test-internal-token", server.Client()).Quote(context.Background(), validInput())
			var pricing *PricingError
			var invalid *Invalid
			switch {
			case tc.unavailable:
				if !errors.Is(err, ErrUnavailable) {
					t.Fatal(err)
				}
			case tc.code == "zone":
				if !errors.As(err, &invalid) {
					t.Fatal(err)
				}
			case tc.code != "":
				if !errors.As(err, &pricing) || pricing.Code != tc.code {
					t.Fatal(err)
				}
			default:
				if err != nil || fee.Total != 55000 {
					t.Fatalf("%+v %v", fee, err)
				}
			}
		})
	}
	if _, err := NewPricingClient("", "", nil).Quote(context.Background(), validInput()); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
}
func TestPricingTimeoutAndRequestID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(requestid.Header) != "m5-request" {
			t.Error("missing request id")
		}
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"data":{"currency":"LKR","baseFee":0,"weightCharge":0,"sizeSurcharge":0,"total":0}}`))
	}))
	defer server.Close()
	router := gin.New()
	router.Use(requestid.Middleware())
	router.GET("/", func(c *gin.Context) {
		_, err := NewPricingClient(server.URL, "test-internal-token", server.Client()).Quote(c.Request.Context(), validInput())
		if err != nil {
			t.Error(err)
		}
	})
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set(requestid.Header, "m5-request")
	router.ServeHTTP(httptest.NewRecorder(), req)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := NewPricingClient(server.URL, "test-internal-token", &http.Client{Timeout: time.Second}).Quote(ctx, validInput()); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
}
func TestCourierAdapter(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
		valid      bool
	}{{"valid", `{"data":{"fullName":"Courier","phone":"+94771111111"}}`, 200, true}, {"missing", `{"error":{"code":"NOT_FOUND"}}`, 404, false}, {"bad", `broken`, 200, false}, {"empty", `{"data":{}}`, 200, false}} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("X-Internal-Token") != "test-internal-token" || r.URL.Path != "/internal/users/"+testUser+"/contact" {
					t.Error("bad request")
				}
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			result, err := NewCourierClient(server.URL, "test-internal-token", server.Client()).Lookup(context.Background(), testUser)
			if tc.valid {
				if err != nil || result.FullName != "Courier" || result.ID != nil {
					t.Fatalf("%+v %v", result, err)
				}
			} else if !errors.Is(err, ErrUnavailable) {
				t.Fatal(err)
			}
		})
	}
	if _, err := NewCourierClient("", "", nil).Lookup(context.Background(), "bad"); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
}

type failedCourier struct{}

func (failedCourier) Lookup(context.Context, string) (Courier, error) {
	return Courier{}, ErrUnavailable
}

type knownCourier struct{}

func (knownCourier) Lookup(context.Context, string) (Courier, error) {
	return Courier{FullName: "Assigned Courier", Phone: "+94771111111"}, nil
}
func TestAssignedCourierVisibility(t *testing.T) {
	ctx := context.Background()
	store := &memoryStore{}
	s := NewService(store, quote550()).WithCourierReader(knownCourier{})
	p, err := s.CreateCustomer(ctx, testUser, "", validInput())
	if err != nil {
		t.Fatal(err)
	}
	id := testOther
	store.p.CurrentCourierID = &id
	customer, err := s.Detail(ctx, p.ID, testUser, false)
	if err != nil {
		t.Fatal(err)
	}
	contact, ok := customer.Courier.(Courier)
	if !ok || contact.ID != nil || contact.FullName != "Assigned Courier" {
		t.Fatalf("unsafe customer contact %+v", customer.Courier)
	}
	staff, err := s.Detail(ctx, p.ID, testOther, true)
	if err != nil {
		t.Fatal(err)
	}
	assigned, ok := staff.Courier.(Courier)
	if !ok || assigned.ID == nil || *assigned.ID != id {
		t.Fatalf("staff contact missing id %+v", staff.Courier)
	}
}
func TestHistoryAndReceiptIndependentOfCourier(t *testing.T) {
	ctx := context.Background()
	store := &memoryStore{}
	s := NewService(store, quote550()).WithCourierReader(failedCourier{})
	p, err := s.CreateCustomer(ctx, testUser, "", validInput())
	if err != nil {
		t.Fatal(err)
	}
	courier := testOther
	store.p.CurrentCourierID = &courier
	if _, err := s.Detail(ctx, p.ID, testUser, false); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("assigned detail requires courier contact: %v", err)
	}
	if _, err := s.History(ctx, p.ID, testUser, false); err != nil {
		t.Fatalf("history called identity: %v", err)
	}
	if _, err := s.ReceiptData(ctx, p.ID, testUser, false); err != nil {
		t.Fatalf("receipt called identity: %v", err)
	}
}
