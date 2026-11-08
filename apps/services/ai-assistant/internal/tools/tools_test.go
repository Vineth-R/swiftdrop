package tools_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Vineth-R/swiftdrop/apps/services/ai-assistant/internal/client"
	"github.com/Vineth-R/swiftdrop/apps/services/ai-assistant/internal/contract"
	"github.com/Vineth-R/swiftdrop/apps/services/ai-assistant/internal/tools"
)

const (
	testParcelID = "b1f0f9d0-2a63-4a1f-9d7e-3c2c2f4a5b6c"
	testTracking = "SDK7M2Q9XP4"
	testToken    = "secret-access-token"
	testUserID   = "0c5f8e3a-1d2b-4c6e-8f90-123456789abc"
)

type fakePricing struct {
	zones         []client.Zone
	zonesErr      error
	zonesCalls    int
	estimate      client.Estimate
	estimateErr   error
	estimateCalls int
	lastEstimate  client.EstimateInput
}

func (f *fakePricing) Zones(context.Context) ([]client.Zone, error) {
	f.zonesCalls++
	return f.zones, f.zonesErr
}

func (f *fakePricing) Estimate(_ context.Context, input client.EstimateInput) (client.Estimate, error) {
	f.estimateCalls++
	f.lastEstimate = input
	return f.estimate, f.estimateErr
}

type fakeParcel struct {
	tracking          client.Tracking
	trackingErr       error
	trackingCalls     int
	lastTracking      string
	lastTrackingToken string
	history           []client.HistoryItem
	historyErr        error
	historyCalls      int
	lastParcelID      string
	lastHistoryToken  string
	list              client.ParcelList
	listErr           error
	listCalls         int
	lastStatus        string
	lastListToken     string
	adminList         client.AdminParcelList
	adminErr          error
	adminCalls        int
	lastAdminStatus   string
	lastAdminToken    string
}

func (f *fakeParcel) Tracking(_ context.Context, trackingNumber, accessToken string) (client.Tracking, error) {
	f.trackingCalls++
	f.lastTracking = trackingNumber
	f.lastTrackingToken = accessToken
	return f.tracking, f.trackingErr
}

func (f *fakeParcel) History(_ context.Context, parcelID, accessToken string) ([]client.HistoryItem, error) {
	f.historyCalls++
	f.lastParcelID = parcelID
	f.lastHistoryToken = accessToken
	return f.history, f.historyErr
}

func (f *fakeParcel) List(_ context.Context, status, accessToken string) (client.ParcelList, error) {
	f.listCalls++
	f.lastStatus = status
	f.lastListToken = accessToken
	return f.list, f.listErr
}

func (f *fakeParcel) AdminParcels(_ context.Context, status, accessToken string) (client.AdminParcelList, error) {
	f.adminCalls++
	f.lastAdminStatus = status
	f.lastAdminToken = accessToken
	return f.adminList, f.adminErr
}

func newRegistry(t *testing.T, pricing tools.PricingClient, parcel tools.ParcelClient) *tools.Registry {
	t.Helper()
	registry, err := tools.NewRegistry(pricing, parcel)
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	return registry
}

func execute(t *testing.T, registry *tools.Registry, name string, args map[string]any, caller contract.CallerContext) contract.ToolResult {
	t.Helper()
	result, err := registry.Execute(context.Background(), name, args, caller)
	if err != nil {
		t.Fatalf("Execute(%q) returned unexpected error: %v", name, err)
	}
	return result
}

func marshal(t *testing.T, value any) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(encoded)
}

func decodeObject(t *testing.T, value any) map[string]json.RawMessage {
	t.Helper()
	var object map[string]json.RawMessage
	if err := json.Unmarshal([]byte(marshal(t, value)), &object); err != nil {
		t.Fatalf("decode object: %v", err)
	}
	return object
}

func authenticated() contract.CallerContext {
	return contract.CallerContext{
		UserID:        testUserID,
		Role:          "CUSTOMER",
		Authenticated: true,
		AccessToken:   testToken,
	}
}

func TestRegistryHasExactlyFourTools(t *testing.T) {
	registry := newRegistry(t, &fakePricing{}, &fakeParcel{})

	definitions := registry.List()
	if len(definitions) != 4 {
		t.Fatalf("expected 4 tools, got %d: %+v", len(definitions), definitions)
	}

	want := map[string]bool{
		"calculate_delivery_fee":   false,
		"get_public_parcel_status": false,
		"get_my_parcel_history":    false,
		"get_my_active_deliveries": false,
	}
	for _, definition := range definitions {
		if _, ok := want[definition.Name]; !ok {
			t.Fatalf("unexpected tool %q", definition.Name)
		}
		want[definition.Name] = true
		if _, ok := registry.Get(definition.Name); !ok {
			t.Fatalf("Get(%q) failed", definition.Name)
		}
	}
	for name, found := range want {
		if !found {
			t.Fatalf("missing tool %q", name)
		}
	}
}

func TestNewRegistryRequiresClients(t *testing.T) {
	if _, err := tools.NewRegistry(nil, &fakeParcel{}); err == nil {
		t.Fatalf("expected error for nil pricing client")
	}
	if _, err := tools.NewRegistry(&fakePricing{}, nil); err == nil {
		t.Fatalf("expected error for nil parcel client")
	}
}

func TestUnknownToolIsNeverExecuted(t *testing.T) {
	pricing := &fakePricing{}
	parcel := &fakeParcel{}
	registry := newRegistry(t, pricing, parcel)

	result := execute(t, registry, "delete_everything", nil, authenticated())
	if result.Status != contract.StatusInvalid {
		t.Fatalf("expected INVALID, got %q", result.Status)
	}
	if result.Message != "tool is not available" {
		t.Fatalf("unexpected message %q", result.Message)
	}
	if result.Data != nil {
		t.Fatalf("unknown tool returned data")
	}
	if pricing.zonesCalls+pricing.estimateCalls+parcel.trackingCalls+parcel.historyCalls+parcel.listCalls != 0 {
		t.Fatalf("unknown tool reached a client")
	}
}

func TestCalculateFeeHappyPath(t *testing.T) {
	pricing := &fakePricing{
		zones: []client.Zone{
			{ID: "zone-cmb", Code: "CMB", Name: "Colombo", Districts: []string{"Colombo", "Gampaha"}},
			{ID: "zone-kdy", Code: "KDY", Name: "Kandy", Districts: []string{"Kandy"}},
		},
		estimate: client.Estimate{
			Currency:      "LKR",
			BaseFee:       "450.00",
			WeightCharge:  "100.00",
			SizeSurcharge: "0.00",
			Total:         "550.00",
		},
	}
	registry := newRegistry(t, pricing, &fakeParcel{})

	result := execute(t, registry, "calculate_delivery_fee", map[string]any{
		"origin":       "colombo",
		"destination":  "KDY",
		"weightKg":     2.5,
		"sizeCategory": "SMALL",
	}, contract.CallerContext{})

	if result.Status != contract.StatusOK {
		t.Fatalf("expected OK, got %q (%s)", result.Status, result.Message)
	}
	if pricing.estimateCalls != 1 {
		t.Fatalf("expected one estimate call, got %d", pricing.estimateCalls)
	}
	if pricing.lastEstimate.OriginZoneID != "zone-cmb" || pricing.lastEstimate.DestinationZoneID != "zone-kdy" {
		t.Fatalf("unexpected resolved zones: %+v", pricing.lastEstimate)
	}
	if pricing.lastEstimate.WeightKg.String() != "2.5" || pricing.lastEstimate.SizeCategory != "SMALL" {
		t.Fatalf("unexpected estimate input: %+v", pricing.lastEstimate)
	}

	encoded := marshal(t, result.Data)
	for _, expected := range []string{
		`"origin":"Colombo"`,
		`"destination":"Kandy"`,
		`"weightKg":2.5`,
		`"sizeCategory":"SMALL"`,
		`"currency":"LKR"`,
		`"baseFee":450.00`,
		`"weightCharge":100.00`,
		`"sizeSurcharge":0.00`,
		`"total":550.00`,
	} {
		if !strings.Contains(encoded, expected) {
			t.Fatalf("fee result missing %q: %s", expected, encoded)
		}
	}
}

func TestCalculateFeeResolvesDistrict(t *testing.T) {
	pricing := &fakePricing{
		zones: []client.Zone{
			{ID: "zone-cmb", Code: "CMB", Name: "Colombo", Districts: []string{"Colombo", "Gampaha"}},
			{ID: "zone-kdy", Code: "KDY", Name: "Kandy", Districts: []string{"Kandy"}},
		},
		estimate: client.Estimate{Currency: "LKR", BaseFee: "1", WeightCharge: "2", SizeSurcharge: "3", Total: "6"},
	}
	registry := newRegistry(t, pricing, &fakeParcel{})

	result := execute(t, registry, "calculate_delivery_fee", map[string]any{
		"origin":       "gampaha",
		"destination":  "kandy",
		"weightKg":     1,
		"sizeCategory": "MEDIUM",
	}, contract.CallerContext{})

	if result.Status != contract.StatusOK {
		t.Fatalf("expected OK, got %q (%s)", result.Status, result.Message)
	}
	if pricing.lastEstimate.OriginZoneID != "zone-cmb" || pricing.lastEstimate.DestinationZoneID != "zone-kdy" {
		t.Fatalf("unexpected resolved zones: %+v", pricing.lastEstimate)
	}
}

func TestCalculateFeeZoneErrors(t *testing.T) {
	t.Run("ambiguous", func(t *testing.T) {
		pricing := &fakePricing{zones: []client.Zone{
			{ID: "z1", Code: "A", Name: "Alpha", Districts: []string{"Shared"}},
			{ID: "z2", Code: "B", Name: "Beta", Districts: []string{"Shared"}},
		}}
		registry := newRegistry(t, pricing, &fakeParcel{})
		result := execute(t, registry, "calculate_delivery_fee", map[string]any{
			"origin": "Shared", "destination": "Alpha", "weightKg": 1, "sizeCategory": "SMALL",
		}, contract.CallerContext{})
		if result.Status != contract.StatusInvalid || !strings.Contains(result.Message, "more than one zone") {
			t.Fatalf("expected ambiguous INVALID, got %q (%s)", result.Status, result.Message)
		}
		if pricing.estimateCalls != 0 {
			t.Fatalf("ambiguous zone should not call estimate")
		}
	})

	t.Run("not found", func(t *testing.T) {
		pricing := &fakePricing{zones: []client.Zone{
			{ID: "z1", Code: "A", Name: "Alpha", Districts: []string{"One"}},
		}}
		registry := newRegistry(t, pricing, &fakeParcel{})
		result := execute(t, registry, "calculate_delivery_fee", map[string]any{
			"origin": "Nowhere", "destination": "Alpha", "weightKg": 1, "sizeCategory": "SMALL",
		}, contract.CallerContext{})
		if result.Status != contract.StatusInvalid || !strings.Contains(result.Message, "not in the serviceable zone directory") {
			t.Fatalf("expected not-found INVALID, got %q (%s)", result.Status, result.Message)
		}
		if pricing.estimateCalls != 0 {
			t.Fatalf("unknown zone should not call estimate")
		}
	})
}

func TestCalculateFeeValidation(t *testing.T) {
	cases := []struct {
		name string
		args map[string]any
	}{
		{"missing origin", map[string]any{"destination": "Kandy", "weightKg": 2, "sizeCategory": "SMALL"}},
		{"wrong weight type", map[string]any{"origin": "Colombo", "destination": "Kandy", "weightKg": "2", "sizeCategory": "SMALL"}},
		{"unknown extra arg", map[string]any{"origin": "Colombo", "destination": "Kandy", "weightKg": 2, "sizeCategory": "SMALL", "userId": testUserID}},
		{"zero weight", map[string]any{"origin": "Colombo", "destination": "Kandy", "weightKg": 0, "sizeCategory": "SMALL"}},
		{"too many decimals", map[string]any{"origin": "Colombo", "destination": "Kandy", "weightKg": 1.005, "sizeCategory": "SMALL"}},
		{"above maximum", map[string]any{"origin": "Colombo", "destination": "Kandy", "weightKg": 1000000, "sizeCategory": "SMALL"}},
		{"lowercase enum", map[string]any{"origin": "Colombo", "destination": "Kandy", "weightKg": 2, "sizeCategory": "small"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pricing := &fakePricing{}
			registry := newRegistry(t, pricing, &fakeParcel{})
			result := execute(t, registry, "calculate_delivery_fee", tc.args, contract.CallerContext{})
			if result.Status != contract.StatusInvalid {
				t.Fatalf("expected INVALID, got %q (%s)", result.Status, result.Message)
			}
			if pricing.zonesCalls != 0 || pricing.estimateCalls != 0 {
				t.Fatalf("validation failure reached upstream")
			}
		})
	}
}

func TestCalculateFeePricingErrors(t *testing.T) {
	zones := []client.Zone{
		{ID: "zone-cmb", Code: "CMB", Name: "Colombo"},
		{ID: "zone-kdy", Code: "KDY", Name: "Kandy"},
	}
	args := map[string]any{"origin": "Colombo", "destination": "Kandy", "weightKg": 2, "sizeCategory": "SMALL"}

	t.Run("route not serviceable", func(t *testing.T) {
		pricing := &fakePricing{zones: zones, estimateErr: &client.Error{Kind: client.KindInvalid, Status: 422, Code: "ROUTE_NOT_SERVICEABLE"}}
		registry := newRegistry(t, pricing, &fakeParcel{})
		result := execute(t, registry, "calculate_delivery_fee", args, contract.CallerContext{})
		if result.Status != contract.StatusInvalid || result.Data != nil {
			t.Fatalf("expected INVALID without data, got %q %+v", result.Status, result.Data)
		}
		if !strings.Contains(result.Message, "route") {
			t.Fatalf("unexpected message %q", result.Message)
		}
	})

	t.Run("weight not priced", func(t *testing.T) {
		pricing := &fakePricing{zones: zones, estimateErr: &client.Error{Kind: client.KindInvalid, Status: 422, Code: "WEIGHT_NOT_PRICED"}}
		registry := newRegistry(t, pricing, &fakeParcel{})
		result := execute(t, registry, "calculate_delivery_fee", args, contract.CallerContext{})
		if result.Status != contract.StatusInvalid || result.Data != nil {
			t.Fatalf("expected INVALID without data, got %q %+v", result.Status, result.Data)
		}
		if !strings.Contains(result.Message, "weight") {
			t.Fatalf("unexpected message %q", result.Message)
		}
	})

	t.Run("unavailable", func(t *testing.T) {
		pricing := &fakePricing{zones: zones, estimateErr: &client.Error{Kind: client.KindUnavailable, Status: 503}}
		registry := newRegistry(t, pricing, &fakeParcel{})
		result := execute(t, registry, "calculate_delivery_fee", args, contract.CallerContext{})
		if result.Status != contract.StatusUnavailable || result.Data != nil {
			t.Fatalf("expected UNAVAILABLE without data, got %q %+v", result.Status, result.Data)
		}
	})

	t.Run("zone directory unavailable", func(t *testing.T) {
		pricing := &fakePricing{zonesErr: &client.Error{Kind: client.KindUnavailable, Status: 503}}
		registry := newRegistry(t, pricing, &fakeParcel{})
		result := execute(t, registry, "calculate_delivery_fee", args, contract.CallerContext{})
		if result.Status != contract.StatusUnavailable || result.Data != nil {
			t.Fatalf("expected UNAVAILABLE without data, got %q %+v", result.Status, result.Data)
		}
		if pricing.estimateCalls != 0 {
			t.Fatalf("estimate should not run when zones fail")
		}
	})
}

func TestTrackingHappyPath(t *testing.T) {
	parcel := &fakeParcel{tracking: client.Tracking{
		TrackingNumber: testTracking,
		Status:         "IN_TRANSIT",
		PickupCity:     "Colombo",
		DropoffCity:    "Kandy",
		LastUpdatedAt:  "2026-11-01T01:00:00Z",
		Timeline: []client.TrackingEvent{
			{Status: "CREATED", At: "2026-11-01T00:00:00Z"},
			{Status: "IN_TRANSIT", At: "2026-11-01T01:00:00Z"},
		},
	}}
	registry := newRegistry(t, &fakePricing{}, parcel)

	result := execute(t, registry, "get_public_parcel_status", map[string]any{"trackingNumber": testTracking}, authenticated())
	if result.Status != contract.StatusOK {
		t.Fatalf("expected OK, got %q (%s)", result.Status, result.Message)
	}
	if parcel.lastTracking != testTracking || parcel.lastTrackingToken != testToken {
		t.Fatalf("tracking request not forwarded with the caller token: number=%q token=%q", parcel.lastTracking, parcel.lastTrackingToken)
	}

	object := decodeObject(t, result.Data)
	if len(object) != 6 {
		t.Fatalf("expected exactly 6 fields, got %d: %s", len(object), marshal(t, result.Data))
	}
	encoded := marshal(t, result.Data)
	for _, private := range []string{"recipientName", "recipientPhone", "dropoffAddress", "internalId", "note", "phone", "address"} {
		if strings.Contains(encoded, private) {
			t.Fatalf("tracking output leaked %q: %s", private, encoded)
		}
	}
}

func TestTrackingInvalidFormat(t *testing.T) {
	for _, number := range []string{"sdk7m2q9xp4", "BAD", "SDK7M2Q9XP"} {
		parcel := &fakeParcel{}
		registry := newRegistry(t, &fakePricing{}, parcel)
		result := execute(t, registry, "get_public_parcel_status", map[string]any{"trackingNumber": number}, authenticated())
		if result.Status != contract.StatusInvalid {
			t.Fatalf("%q: expected INVALID, got %q", number, result.Status)
		}
		if parcel.trackingCalls != 0 {
			t.Fatalf("%q: invalid format reached upstream", number)
		}
	}
}

func TestTrackingErrors(t *testing.T) {
	t.Run("not found", func(t *testing.T) {
		parcel := &fakeParcel{trackingErr: &client.Error{Kind: client.KindNotFound, Status: 404, Code: "TRACKING_NOT_FOUND"}}
		registry := newRegistry(t, &fakePricing{}, parcel)
		result := execute(t, registry, "get_public_parcel_status", map[string]any{"trackingNumber": testTracking}, authenticated())
		if result.Status != contract.StatusInvalid || !strings.Contains(result.Message, "not found") {
			t.Fatalf("expected INVALID not found, got %q (%s)", result.Status, result.Message)
		}
	})

	t.Run("unavailable", func(t *testing.T) {
		parcel := &fakeParcel{trackingErr: &client.Error{Kind: client.KindUnavailable, Status: 503}}
		registry := newRegistry(t, &fakePricing{}, parcel)
		result := execute(t, registry, "get_public_parcel_status", map[string]any{"trackingNumber": testTracking}, authenticated())
		if result.Status != contract.StatusUnavailable {
			t.Fatalf("expected UNAVAILABLE, got %q", result.Status)
		}
	})
}

func TestHistoryRequiresAuthenticationBeforeHTTP(t *testing.T) {
	cases := []struct {
		name   string
		caller contract.CallerContext
	}{
		{"anonymous", contract.CallerContext{}},
		{"authenticated without token", contract.CallerContext{UserID: testUserID, Authenticated: true}},
		{"authenticated without user id", contract.CallerContext{Authenticated: true, AccessToken: testToken}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			parcel := &fakeParcel{}
			registry := newRegistry(t, &fakePricing{}, parcel)
			result := execute(t, registry, "get_my_parcel_history", map[string]any{"parcelId": testParcelID}, tc.caller)
			if result.Status != contract.StatusDenied {
				t.Fatalf("expected DENIED, got %q", result.Status)
			}
			if parcel.historyCalls != 0 {
				t.Fatalf("denied call reached upstream")
			}
		})
	}
}

func TestHistoryHappyPath(t *testing.T) {
	note := "picked up by courier"
	parcel := &fakeParcel{history: []client.HistoryItem{
		{PreviousStatus: nil, Status: "CREATED", ChangedAt: "2026-11-01T00:00:00Z", Note: nil},
		{PreviousStatus: stringPtr("CREATED"), Status: "PICKED_UP", ChangedAt: "2026-11-01T01:00:00Z", Note: &note},
	}}
	registry := newRegistry(t, &fakePricing{}, parcel)

	result := execute(t, registry, "get_my_parcel_history", map[string]any{"parcelId": testParcelID}, authenticated())
	if result.Status != contract.StatusOK {
		t.Fatalf("expected OK, got %q (%s)", result.Status, result.Message)
	}
	if parcel.lastParcelID != testParcelID || parcel.lastHistoryToken != testToken {
		t.Fatalf("unexpected forward: id=%q token=%q", parcel.lastParcelID, parcel.lastHistoryToken)
	}

	encoded := marshal(t, result.Data)
	for _, private := range []string{"failureReason", "actorRole", testToken} {
		if strings.Contains(encoded, private) {
			t.Fatalf("history output leaked %q: %s", private, encoded)
		}
	}
	for _, expected := range []string{`"previousStatus":null`, `"status":"PICKED_UP"`, `"note":"picked up by courier"`} {
		if !strings.Contains(encoded, expected) {
			t.Fatalf("history output missing %q: %s", expected, encoded)
		}
	}
}

func TestHistoryValidation(t *testing.T) {
	t.Run("invalid uuid", func(t *testing.T) {
		parcel := &fakeParcel{}
		registry := newRegistry(t, &fakePricing{}, parcel)
		result := execute(t, registry, "get_my_parcel_history", map[string]any{"parcelId": "not-a-uuid"}, authenticated())
		if result.Status != contract.StatusInvalid {
			t.Fatalf("expected INVALID, got %q", result.Status)
		}
		if parcel.historyCalls != 0 {
			t.Fatalf("invalid uuid reached upstream")
		}
	})

	t.Run("model supplied identity rejected", func(t *testing.T) {
		parcel := &fakeParcel{}
		registry := newRegistry(t, &fakePricing{}, parcel)
		result := execute(t, registry, "get_my_parcel_history", map[string]any{
			"parcelId": testParcelID,
			"userId":   testUserID,
		}, authenticated())
		if result.Status != contract.StatusInvalid {
			t.Fatalf("expected INVALID, got %q", result.Status)
		}
		if parcel.historyCalls != 0 {
			t.Fatalf("identity-bearing argument reached upstream")
		}
	})
}

func TestHistoryUpstreamErrors(t *testing.T) {
	t.Run("not found for account", func(t *testing.T) {
		parcel := &fakeParcel{historyErr: &client.Error{Kind: client.KindNotFound, Status: 404}}
		registry := newRegistry(t, &fakePricing{}, parcel)
		result := execute(t, registry, "get_my_parcel_history", map[string]any{"parcelId": testParcelID}, authenticated())
		if result.Status != contract.StatusInvalid || !strings.Contains(result.Message, "not found for this account") {
			t.Fatalf("expected safe not-found, got %q (%s)", result.Status, result.Message)
		}
		if strings.Contains(result.Message, testToken) || strings.Contains(marshal(t, result.Data), testToken) {
			t.Fatalf("token leaked")
		}
	})

	t.Run("unauthorized", func(t *testing.T) {
		parcel := &fakeParcel{historyErr: &client.Error{Kind: client.KindUnauthorized, Status: 403}}
		registry := newRegistry(t, &fakePricing{}, parcel)
		result := execute(t, registry, "get_my_parcel_history", map[string]any{"parcelId": testParcelID}, authenticated())
		if result.Status != contract.StatusDenied {
			t.Fatalf("expected DENIED, got %q", result.Status)
		}
	})

	t.Run("unavailable", func(t *testing.T) {
		parcel := &fakeParcel{historyErr: &client.Error{Kind: client.KindUnavailable, Status: 503}}
		registry := newRegistry(t, &fakePricing{}, parcel)
		result := execute(t, registry, "get_my_parcel_history", map[string]any{"parcelId": testParcelID}, authenticated())
		if result.Status != contract.StatusUnavailable {
			t.Fatalf("expected UNAVAILABLE, got %q", result.Status)
		}
	})
}

func TestDeliveriesRequiresAuthenticationBeforeHTTP(t *testing.T) {
	parcel := &fakeParcel{}
	registry := newRegistry(t, &fakePricing{}, parcel)
	result := execute(t, registry, "get_my_active_deliveries", nil, contract.CallerContext{})
	if result.Status != contract.StatusDenied {
		t.Fatalf("expected DENIED, got %q", result.Status)
	}
	if parcel.listCalls != 0 {
		t.Fatalf("anonymous call reached upstream")
	}
}

func TestDeliveriesHappyPath(t *testing.T) {
	parcel := &fakeParcel{list: client.ParcelList{Items: []client.ParcelListItem{
		{ID: "p1", TrackingNumber: testTracking, Status: "IN_TRANSIT", DropoffCity: "Kandy", RecipientName: "Recipient", Total: "550.00", Currency: "LKR", CreatedAt: "2026-11-01T00:00:00Z"},
		{ID: "p2", TrackingNumber: "SDN12345678", Status: "CREATED", DropoffCity: "Galle", RecipientName: "Other", Total: "650.00", Currency: "LKR", CreatedAt: "2026-11-02T00:00:00Z"},
	}}}
	registry := newRegistry(t, &fakePricing{}, parcel)

	result := execute(t, registry, "get_my_active_deliveries", map[string]any{"status": "IN_TRANSIT"}, authenticated())
	if result.Status != contract.StatusOK {
		t.Fatalf("expected OK, got %q (%s)", result.Status, result.Message)
	}
	if parcel.lastStatus != "IN_TRANSIT" || parcel.lastListToken != testToken {
		t.Fatalf("unexpected forward: status=%q token=%q", parcel.lastStatus, parcel.lastListToken)
	}

	encoded := marshal(t, result.Data)
	for _, expected := range []string{`"count":2`, `"trackingNumber":"` + testTracking + `"`, `"total":550.00`, `"dropoffCity":"Kandy"`} {
		if !strings.Contains(encoded, expected) {
			t.Fatalf("deliveries output missing %q: %s", expected, encoded)
		}
	}
	for _, private := range []string{"dropoffAddress", "senderName", "recipientPhone", testToken} {
		if strings.Contains(encoded, private) {
			t.Fatalf("deliveries output leaked %q: %s", private, encoded)
		}
	}
}

func TestDeliveriesEmptyListIsOK(t *testing.T) {
	parcel := &fakeParcel{list: client.ParcelList{Items: []client.ParcelListItem{}}}
	registry := newRegistry(t, &fakePricing{}, parcel)

	result := execute(t, registry, "get_my_active_deliveries", nil, authenticated())
	if result.Status != contract.StatusOK {
		t.Fatalf("expected OK, got %q (%s)", result.Status, result.Message)
	}
	encoded := marshal(t, result.Data)
	if !strings.Contains(encoded, `"deliveries":[]`) || !strings.Contains(encoded, `"count":0`) {
		t.Fatalf("expected empty array and zero count: %s", encoded)
	}
}

func TestDeliveriesValidation(t *testing.T) {
	t.Run("invalid status enum", func(t *testing.T) {
		parcel := &fakeParcel{}
		registry := newRegistry(t, &fakePricing{}, parcel)
		result := execute(t, registry, "get_my_active_deliveries", map[string]any{"status": "in_transit"}, authenticated())
		if result.Status != contract.StatusInvalid {
			t.Fatalf("expected INVALID, got %q", result.Status)
		}
		if parcel.listCalls != 0 {
			t.Fatalf("invalid enum reached upstream")
		}
	})

	t.Run("model supplied owner id rejected", func(t *testing.T) {
		parcel := &fakeParcel{}
		registry := newRegistry(t, &fakePricing{}, parcel)
		result := execute(t, registry, "get_my_active_deliveries", map[string]any{"ownerId": testUserID}, authenticated())
		if result.Status != contract.StatusInvalid {
			t.Fatalf("expected INVALID, got %q", result.Status)
		}
		if parcel.listCalls != 0 {
			t.Fatalf("identity-bearing argument reached upstream")
		}
	})
}

func TestDeliveriesCapsAtTwenty(t *testing.T) {
	items := make([]client.ParcelListItem, 0, 25)
	for i := 0; i < 25; i++ {
		items = append(items, client.ParcelListItem{
			ID: "p", TrackingNumber: testTracking, Status: "CREATED", Total: "100.00", Currency: "LKR",
		})
	}
	parcel := &fakeParcel{list: client.ParcelList{Items: items}}
	registry := newRegistry(t, &fakePricing{}, parcel)

	result := execute(t, registry, "get_my_active_deliveries", nil, authenticated())
	if result.Status != contract.StatusOK {
		t.Fatalf("expected OK, got %q", result.Status)
	}
	encoded := marshal(t, result.Data)
	if !strings.Contains(encoded, `"count":20`) {
		t.Fatalf("expected count capped at 20: %s", encoded)
	}
}

func TestDeliveriesUpstreamErrors(t *testing.T) {
	t.Run("not found", func(t *testing.T) {
		parcel := &fakeParcel{listErr: &client.Error{Kind: client.KindNotFound, Status: 404}}
		registry := newRegistry(t, &fakePricing{}, parcel)
		result := execute(t, registry, "get_my_active_deliveries", nil, authenticated())
		if result.Status != contract.StatusInvalid || !strings.Contains(result.Message, "not found for this account") {
			t.Fatalf("expected safe not-found, got %q (%s)", result.Status, result.Message)
		}
	})

	t.Run("unavailable", func(t *testing.T) {
		parcel := &fakeParcel{listErr: &client.Error{Kind: client.KindUnavailable, Status: 503}}
		registry := newRegistry(t, &fakePricing{}, parcel)
		result := execute(t, registry, "get_my_active_deliveries", nil, authenticated())
		if result.Status != contract.StatusUnavailable {
			t.Fatalf("expected UNAVAILABLE, got %q", result.Status)
		}
	})
}

func TestTrackingForwardsCallerTokenWithoutLeakingIt(t *testing.T) {
	parcel := &fakeParcel{tracking: client.Tracking{TrackingNumber: testTracking, Status: "CREATED"}}
	registry := newRegistry(t, &fakePricing{}, parcel)

	result := execute(t, registry, "get_public_parcel_status", map[string]any{"trackingNumber": testTracking}, authenticated())
	if result.Status != contract.StatusOK {
		t.Fatalf("expected OK, got %q", result.Status)
	}
	if parcel.lastTracking != testTracking || parcel.lastTrackingToken != testToken {
		t.Fatalf("tracking call did not use the caller credentials: number=%q token=%q", parcel.lastTracking, parcel.lastTrackingToken)
	}
	if strings.Contains(marshal(t, result.Data), testToken) || strings.Contains(result.Message, testToken) {
		t.Fatalf("access token leaked into the tool result")
	}
}

func TestTrackingDeniesGuestsBeforeUpstream(t *testing.T) {
	parcel := &fakeParcel{tracking: client.Tracking{TrackingNumber: testTracking, Status: "CREATED"}}
	registry := newRegistry(t, &fakePricing{}, parcel)

	result := execute(t, registry, "get_public_parcel_status", map[string]any{"trackingNumber": testTracking}, contract.CallerContext{})
	if result.Status != contract.StatusDenied {
		t.Fatalf("expected DENIED for a guest, got %q", result.Status)
	}
	if !strings.Contains(strings.ToLower(result.Message), "sign in") {
		t.Fatalf("guest denial should explain sign-in: %q", result.Message)
	}
	if parcel.trackingCalls != 0 {
		t.Fatalf("guest tracking reached upstream %d times", parcel.trackingCalls)
	}
}

func stringPtr(value string) *string { return &value }

func callerWithRole(role string) contract.CallerContext {
	return contract.CallerContext{
		UserID:        testUserID,
		Role:          role,
		Authenticated: true,
		AccessToken:   testToken,
	}
}

func TestPrivateToolRoleMatrix(t *testing.T) {
	historyArgs := map[string]any{"parcelId": testParcelID}
	cases := []struct {
		role            string
		allowed         bool
		expectAdminList bool
	}{
		{role: "CUSTOMER", allowed: true},
		{role: "DISPATCHER", allowed: true, expectAdminList: true},
		{role: "ADMIN", allowed: true, expectAdminList: true},
		{role: "MERCHANT", allowed: false},
		{role: "COURIER", allowed: false},
	}
	for _, testCase := range cases {
		t.Run(testCase.role, func(t *testing.T) {
			parcel := &fakeParcel{
				tracking: client.Tracking{TrackingNumber: testTracking, Status: "IN_TRANSIT"},
				history:  []client.HistoryItem{{Status: "IN_TRANSIT", ChangedAt: "2026-11-01T00:00:00Z"}},
				list: client.ParcelList{Items: []client.ParcelListItem{
					{ID: testParcelID, TrackingNumber: testTracking, Status: "IN_TRANSIT"},
				}},
				adminList: client.AdminParcelList{Items: []client.AdminParcelListItem{
					{ID: testParcelID, TrackingNumber: testTracking, Status: "IN_TRANSIT"},
				}},
			}
			registry := newRegistry(t, &fakePricing{}, parcel)
			caller := callerWithRole(testCase.role)

			history := execute(t, registry, "get_my_parcel_history", historyArgs, caller)
			deliveries := execute(t, registry, "get_my_active_deliveries", nil, caller)
			tracking := execute(t, registry, "get_public_parcel_status", map[string]any{"trackingNumber": testTracking}, caller)

			if testCase.allowed {
				if history.Status != contract.StatusOK || deliveries.Status != contract.StatusOK || tracking.Status != contract.StatusOK {
					t.Fatalf("history=%q deliveries=%q tracking=%q", history.Status, deliveries.Status, tracking.Status)
				}
				if parcel.lastHistoryToken != testToken || parcel.lastTrackingToken != testToken {
					t.Fatalf("history token = %q tracking token = %q", parcel.lastHistoryToken, parcel.lastTrackingToken)
				}
				if testCase.expectAdminList {
					if parcel.adminCalls != 1 || parcel.listCalls != 0 {
						t.Fatalf("operational calls admin=%d customer=%d", parcel.adminCalls, parcel.listCalls)
					}
				} else if parcel.listCalls != 1 || parcel.adminCalls != 0 {
					t.Fatalf("customer calls list=%d admin=%d", parcel.listCalls, parcel.adminCalls)
				}
				return
			}

			if history.Status != contract.StatusDenied || deliveries.Status != contract.StatusDenied || tracking.Status != contract.StatusDenied {
				t.Fatalf("expected DENIED, got history=%q deliveries=%q tracking=%q", history.Status, deliveries.Status, tracking.Status)
			}
			if parcel.historyCalls != 0 || parcel.listCalls != 0 || parcel.adminCalls != 0 || parcel.trackingCalls != 0 {
				t.Fatalf("denied role reached upstream: history=%d list=%d admin=%d tracking=%d",
					parcel.historyCalls, parcel.listCalls, parcel.adminCalls, parcel.trackingCalls)
			}
		})
	}

	t.Run("anonymous", func(t *testing.T) {
		parcel := &fakeParcel{}
		registry := newRegistry(t, &fakePricing{}, parcel)
		history := execute(t, registry, "get_my_parcel_history", historyArgs, contract.CallerContext{})
		deliveries := execute(t, registry, "get_my_active_deliveries", nil, contract.CallerContext{})
		tracking := execute(t, registry, "get_public_parcel_status", map[string]any{"trackingNumber": testTracking}, contract.CallerContext{})
		if history.Status != contract.StatusDenied || deliveries.Status != contract.StatusDenied || tracking.Status != contract.StatusDenied {
			t.Fatalf("expected DENIED")
		}
		if parcel.historyCalls != 0 || parcel.listCalls != 0 || parcel.adminCalls != 0 || parcel.trackingCalls != 0 {
			t.Fatal("anonymous call reached upstream")
		}
	})
}

func TestOperationalDeliveriesAllowlistAndScope(t *testing.T) {
	parcel := &fakeParcel{adminList: client.AdminParcelList{
		Items: []client.AdminParcelListItem{
			{
				ID: testParcelID, TrackingNumber: testTracking, Status: "IN_TRANSIT",
				PickupCity: "Colombo", DropoffCity: "Kandy", CreatedAt: "2026-11-01T00:00:00Z",
			},
			{
				ID: "p2", TrackingNumber: "SDN12345678", Status: "DELIVERED",
				PickupCity: "Galle", DropoffCity: "Colombo", CreatedAt: "2026-11-02T00:00:00Z",
			},
		},
		Pagination: client.Pagination{TotalItems: 7},
	}}
	registry := newRegistry(t, &fakePricing{}, parcel)

	result := execute(t, registry, "get_my_active_deliveries", map[string]any{"status": "IN_TRANSIT"}, callerWithRole("DISPATCHER"))
	if result.Status != contract.StatusOK {
		t.Fatalf("expected OK, got %q (%s)", result.Status, result.Message)
	}
	if parcel.lastAdminStatus != "IN_TRANSIT" || parcel.lastAdminToken != testToken {
		t.Fatalf("unexpected operational forward: status=%q token=%q", parcel.lastAdminStatus, parcel.lastAdminToken)
	}

	encoded := marshal(t, result.Data)
	for _, expected := range []string{
		`"scope":"operations"`, `"count":2`, `"activeCount":1`, `"totalItems":7`,
		`"trackingNumber":"` + testTracking + `"`, `"pickupCity":"Colombo"`,
	} {
		if !strings.Contains(encoded, expected) {
			t.Fatalf("operational output missing %q: %s", expected, encoded)
		}
	}
	for _, forbidden := range []string{"customerId", "merchantId", "recipientName", "phone", "address", testToken} {
		if strings.Contains(encoded, forbidden) {
			t.Fatalf("operational output leaked %q: %s", forbidden, encoded)
		}
	}
}
