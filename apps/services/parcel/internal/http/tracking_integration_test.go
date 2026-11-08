//go:build integration && postgres

package servicehttp

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/Vineth-R/swiftdrop/apps/services/parcel/internal/parcels"
	"github.com/Vineth-R/swiftdrop/apps/services/parcel/internal/tracking"
	"github.com/Vineth-R/swiftdrop/packages/go-common/auth"
	"github.com/Vineth-R/swiftdrop/tests/integration/parceltest"
)

// TestOwnedTrackingAuthorizationAgainstPostgres proves that the Parcel service
// itself scopes tracking by parcel.customer_user_id. It is the boundary the AI
// assistant relies on, so a direct call cannot bypass it.
func TestOwnedTrackingAuthorizationAgainstPostgres(t *testing.T) {
	database := parceltest.New(t)
	quotes := &httpWorkflowQuotes{}
	quotes.total.Store(55000)
	service := parcels.NewService(parcels.NewStore(database.Pool), quotes)
	router, err := New(slog.New(slog.NewTextHandler(io.Discard, nil)), Options{
		Parcels:      service,
		Tracking:     tracking.NewRepository(database.Pool),
		AccessSecret: parcelWorkflowSecret,
	})
	if err != nil {
		t.Fatal(err)
	}

	owner := parceltest.UUID(t)
	ownerToken := httpWorkflowToken(t, owner, auth.RoleCustomer)
	otherToken := httpWorkflowToken(t, parceltest.UUID(t), auth.RoleCustomer)
	dispatcherToken := httpWorkflowToken(t, parceltest.UUID(t), auth.RoleDispatcher)
	adminToken := httpWorkflowToken(t, parceltest.UUID(t), auth.RoleAdmin)
	merchantToken := httpWorkflowToken(t, parceltest.UUID(t), auth.RoleMerchant)
	courierToken := httpWorkflowToken(t, parceltest.UUID(t), auth.RoleCourier)

	input := parcels.CreateInput{
		Sender:    parcels.Contact{Name: "Nimal Perera", Phone: "+94771234567", Address: "12 Temple Road", City: "Colombo", ZoneID: parceltest.UUID(t)},
		Recipient: parcels.Contact{Name: "Amali Fernando", Phone: "+94712345678", Address: "45 Lake Drive", City: "Kandy", ZoneID: parceltest.UUID(t)},
		WeightKG:  200, SizeCategory: "SMALL",
	}
	expected := parcels.Decimal(55000)
	input.ExpectedFee = &expected
	created := httpWorkflowRequest(t, router, http.MethodPost, "/parcels", ownerToken, "", parceltest.UUID(t), input)
	if created.Code != http.StatusCreated {
		t.Fatalf("create HTTP %d: %s", created.Code, created.Body.String())
	}
	var envelope struct {
		Data parcels.Detail `json:"data"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	number := envelope.Data.TrackingNumber
	path := "/parcels/tracking/" + number

	tests := []struct {
		name  string
		token string
		want  int
	}{
		{name: "owner", token: ownerToken, want: http.StatusOK},
		{name: "another customer", token: otherToken, want: http.StatusNotFound},
		{name: "dispatcher", token: dispatcherToken, want: http.StatusOK},
		{name: "admin", token: adminToken, want: http.StatusOK},
		{name: "merchant", token: merchantToken, want: http.StatusForbidden},
		{name: "courier", token: courierToken, want: http.StatusForbidden},
		{name: "no credentials", want: http.StatusUnauthorized},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := httpWorkflowRequest(t, router, http.MethodGet, path, test.token, "", "", nil)
			if response.Code != test.want {
				t.Fatalf("HTTP %d, want %d: %s", response.Code, test.want, response.Body.String())
			}
			body := response.Body.String()
			for _, secret := range []string{input.Sender.Name, input.Sender.Phone, input.Sender.Address, input.Recipient.Name, input.Recipient.Phone, input.Recipient.Address} {
				if strings.Contains(body, secret) {
					t.Fatalf("%s response leaked %q: %s", test.name, secret, body)
				}
			}
			if test.want != http.StatusOK {
				return
			}
			var payload struct {
				Data map[string]json.RawMessage `json:"data"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
				t.Fatal(err)
			}
			assertKeys(t, payload.Data, "dropoffCity", "lastUpdatedAt", "pickupCity", "status", "timeline", "trackingNumber")
		})
	}

	if response := httpWorkflowRequest(t, router, http.MethodGet, "/parcels/tracking/SDK00000000", ownerToken, "", "", nil); response.Code != http.StatusNotFound {
		t.Fatalf("unknown tracking number HTTP %d, want 404", response.Code)
	}
	if response := httpWorkflowRequest(t, router, http.MethodGet, "/parcels/tracking/bad", ownerToken, "", "", nil); response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid tracking number HTTP %d, want 422", response.Code)
	}
}
