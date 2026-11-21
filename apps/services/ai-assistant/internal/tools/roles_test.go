package tools_test

import (
	"context"
	"strings"
	"testing"

	"github.com/Vineth-R/swiftdrop/apps/services/ai-assistant/internal/client"
	"github.com/Vineth-R/swiftdrop/apps/services/ai-assistant/internal/contract"
)

// toolFixtures returns a fake client set whose every read succeeds, so the role
// matrix can distinguish authorization from upstream failures.
func toolFixtures() (*fakePricing, *fakeParcel, *fakeMerchant) {
	pricing := &fakePricing{
		zones:    []client.Zone{{ID: "zone-cmb", Code: "CMB", Name: "Colombo", Districts: []string{"Colombo"}}},
		estimate: client.Estimate{Currency: "LKR", BaseFee: "450.00", WeightCharge: "100.00", SizeSurcharge: "0.00", Total: "550.00", WeightKg: "2", SizeCategory: "SMALL"},
	}
	parcel := &fakeParcel{
		tracking:       client.Tracking{TrackingNumber: testTracking, Status: "IN_TRANSIT"},
		publicTracking: client.Tracking{TrackingNumber: testTracking, Status: "IN_TRANSIT"},
		history:        []client.HistoryItem{{Status: "CREATED", ChangedAt: "2026-11-01T00:00:00Z"}},
		list:           client.ParcelList{Items: []client.ParcelListItem{{ID: testParcelID, TrackingNumber: testTracking, Status: "IN_TRANSIT", Total: "550.00"}}},
		adminList:      client.AdminParcelList{Items: []client.AdminParcelListItem{{ID: testParcelID, TrackingNumber: testTracking, Status: "FAILED"}}},
		detail:         client.ParcelDetail{ID: testParcelID, TrackingNumber: testTracking, Status: "FAILED", WeightKg: "2", Total: "550.00", Currency: "LKR"},
		unassigned:     client.UnassignedList{Items: []client.UnassignedParcel{{ID: testParcelID, TrackingNumber: testTracking, Status: "CREATED", WeightKg: "2"}}},
		workload:       []client.CourierWorkload{{CourierID: testUserID, FullName: "Courier", ActiveDeliveries: 2}},
		courierList:    client.CourierDeliveryList{Items: []client.CourierDeliveryItem{{ID: testParcelID, TrackingNumber: testTracking, WeightKg: "2"}}},
		courierDetail:  client.CourierDeliveryDetail{ID: testParcelID, TrackingNumber: testTracking, WeightKg: "2"},
	}
	merchant := &fakeMerchant{
		orders:  client.MerchantOrderList{Items: []client.MerchantOrderItem{{OrderID: "order-1", TrackingNumber: "SDN4T8W2LC6", Total: "550.00", Currency: "LKR"}}},
		webhook: client.WebhookDeliveryList{Items: []client.WebhookDeliveryAttempt{{EventID: "event-1", TrackingNumber: "SDN4T8W2LC6", AttemptNumber: 1}}},
	}
	return pricing, parcel, merchant
}

func upstreamCalls(pricing *fakePricing, parcel *fakeParcel, merchant *fakeMerchant) int {
	return pricing.zonesCalls + pricing.estimateCalls +
		parcel.trackingCalls + parcel.historyCalls + parcel.listCalls + parcel.adminCalls +
		parcel.publicCalls + parcel.detailCalls + parcel.unassignedCalls + parcel.workloadCalls +
		parcel.courierListCalls + parcel.courierDetailCalls +
		merchant.ordersCalls + merchant.webhookCalls
}

type toolCase struct {
	name     string
	args     map[string]any
	allowed  map[string]bool
	merchant bool
}

func TestRoleMatrixForAllTwelveTools(t *testing.T) {
	cases := []toolCase{
		{name: "calculate_delivery_fee", args: map[string]any{"origin": "Colombo", "destination": "Colombo", "weightKg": 2, "sizeCategory": "SMALL"},
			allowed: map[string]bool{"": true, "CUSTOMER": true, "COURIER": true, "MERCHANT": true, "DISPATCHER": true, "ADMIN": true}},
		{name: "get_public_parcel_status", args: map[string]any{"trackingNumber": testTracking},
			allowed: map[string]bool{"": true, "CUSTOMER": true, "DISPATCHER": true, "ADMIN": true}},
		{name: "get_parcel_history", args: map[string]any{"parcelId": testParcelID},
			allowed: map[string]bool{"CUSTOMER": true, "DISPATCHER": true, "ADMIN": true}},
		{name: "get_active_parcels", args: nil,
			allowed: map[string]bool{"CUSTOMER": true, "DISPATCHER": true, "ADMIN": true}},
		{name: "get_parcel_details", args: map[string]any{"parcelId": testParcelID},
			allowed: map[string]bool{"CUSTOMER": true, "DISPATCHER": true, "ADMIN": true}},
		{name: "search_parcels", args: nil,
			allowed: map[string]bool{"DISPATCHER": true, "ADMIN": true}},
		{name: "list_unassigned_parcels", args: nil,
			allowed: map[string]bool{"DISPATCHER": true, "ADMIN": true}},
		{name: "get_courier_workload", args: nil,
			allowed: map[string]bool{"DISPATCHER": true, "ADMIN": true}},
		{name: "get_my_courier_deliveries", args: nil,
			allowed: map[string]bool{"COURIER": true}},
		{name: "get_my_courier_delivery", args: map[string]any{"parcelId": testParcelID},
			allowed: map[string]bool{"COURIER": true}},
		{name: "get_my_merchant_orders", args: nil, merchant: true,
			allowed: map[string]bool{"MERCHANT": true}},
		{name: "get_my_webhook_deliveries", args: nil, merchant: true,
			allowed: map[string]bool{"MERCHANT": true}},
	}

	roles := []string{"", "CUSTOMER", "COURIER", "MERCHANT", "DISPATCHER", "ADMIN"}
	for _, testCase := range cases {
		for _, role := range roles {
			t.Run(testCase.name+"/"+roleLabel(role), func(t *testing.T) {
				pricing, parcel, merchant := toolFixtures()
				registry := newRegistryWithMerchant(t, pricing, parcel, merchant)
				caller := callerWithRole(role)
				if role == "" {
					caller = contract.CallerContext{}
				}

				result := execute(t, registry, testCase.name, testCase.args, caller)
				allowed := testCase.allowed[role]

				if allowed {
					if result.Status == contract.StatusDenied {
						t.Fatalf("allowed role %q was denied: %s", role, result.Message)
					}
					if upstreamCalls(pricing, parcel, merchant) == 0 {
						t.Fatalf("allowed role %q made no upstream call", role)
					}
					return
				}

				if result.Status != contract.StatusDenied {
					t.Fatalf("role %q status = %q, want DENIED", role, result.Status)
				}
				if upstreamCalls(pricing, parcel, merchant) != 0 {
					t.Fatalf("denied role %q reached an upstream service", role)
				}
			})
		}
	}
}

func TestHiddenToolExecutionIsDeniedBeforeUpstream(t *testing.T) {
	cases := []struct {
		role string
		tool string
		args map[string]any
	}{
		{role: "", tool: "search_parcels", args: nil},
		{role: "CUSTOMER", tool: "list_unassigned_parcels", args: nil},
		{role: "COURIER", tool: "get_parcel_details", args: map[string]any{"parcelId": testParcelID}},
		{role: "MERCHANT", tool: "get_courier_workload", args: nil},
		{role: "DISPATCHER", tool: "get_my_merchant_orders", args: nil},
		{role: "ADMIN", tool: "get_my_courier_deliveries", args: nil},
	}
	for _, testCase := range cases {
		t.Run(roleLabel(testCase.role)+"/"+testCase.tool, func(t *testing.T) {
			pricing, parcel, merchant := toolFixtures()
			registry := newRegistryWithMerchant(t, pricing, parcel, merchant)
			caller := callerWithRole(testCase.role)
			if testCase.role == "" {
				caller = contract.CallerContext{}
			}

			// The tool is not even visible to the role.
			for _, definition := range registry.List(caller) {
				if definition.Name == testCase.tool {
					t.Fatalf("tool %q is visible to role %q", testCase.tool, testCase.role)
				}
			}

			result := execute(t, registry, testCase.tool, testCase.args, caller)
			if result.Status != contract.StatusDenied {
				t.Fatalf("hidden tool status = %q, want DENIED", result.Status)
			}
			if upstreamCalls(pricing, parcel, merchant) != 0 {
				t.Fatal("hidden tool reached an upstream service")
			}
		})
	}
}

func TestToolOutputsExcludeSensitiveFixtureFields(t *testing.T) {
	_, parcel, merchant := toolFixtures()
	parcel.detail = client.ParcelDetail{
		ID: testParcelID, TrackingNumber: testTracking, Status: "FAILED",
		PickupCity: "Colombo", DropoffCity: "Kandy", WeightKg: "2", SizeCategory: "SMALL",
		Total: "550.00", Currency: "LKR", CourierName: stringPtr("Courier Name"),
		FailureReason: stringPtr("Recipient unavailable"), CreatedAt: "2026-11-01T00:00:00Z", UpdatedAt: "2026-11-02T00:00:00Z",
	}
	parcel.courierDetail = client.CourierDeliveryDetail{
		ID: testParcelID, TrackingNumber: testTracking, Status: "FAILED", DropoffCity: "Kandy",
		RecipientName: "Recipient", WeightKg: "2", SizeCategory: "SMALL", AssignedAt: "2026-11-01T00:00:00Z",
		History: []client.CourierHistoryItem{{Status: "FAILED", FailureReason: stringPtr("No access"), ChangedAt: "2026-11-02T00:00:00Z"}},
	}
	merchant.orders = client.MerchantOrderList{Items: []client.MerchantOrderItem{{
		OrderID: "order-1", ExternalOrderID: "SZ-1", TrackingNumber: "SDN4T8W2LC6", Status: "FAILED",
		RecipientName: "Recipient", DropoffCity: "Kandy", Total: "550.00", Currency: "LKR",
		CreatedAt: "2026-11-01T00:00:00Z", UpdatedAt: "2026-11-02T00:00:00Z",
	}}}

	registry := newRegistryWithMerchant(t, &fakePricing{}, parcel, merchant)
	checks := []struct {
		tool   string
		args   map[string]any
		caller contract.CallerContext
	}{
		{"get_parcel_details", map[string]any{"parcelId": testParcelID}, callerWithRole("CUSTOMER")},
		{"get_my_courier_delivery", map[string]any{"parcelId": testParcelID}, callerWithRole("COURIER")},
		{"get_my_merchant_orders", nil, callerWithRole("MERCHANT")},
	}
	for _, check := range checks {
		result := execute(t, registry, check.tool, check.args, check.caller)
		if result.Status != contract.StatusOK {
			t.Fatalf("%s = %q (%s)", check.tool, result.Status, result.Message)
		}
		encoded := marshal(t, result.Data)
		for _, forbidden := range []string{
			"phone", "Phone", "address", "Address", "customerId", "merchantId", "ownerId",
			"proof", "proofs", "senderName", "recipientPhone", "note\"", "/proofs/",
		} {
			if strings.Contains(encoded, forbidden) {
				t.Fatalf("%s leaked %q: %s", check.tool, forbidden, encoded)
			}
		}
	}
}

func TestSearchParcelsValidationAndFilters(t *testing.T) {
	parcel := &fakeParcel{adminList: client.AdminParcelList{
		Items:      []client.AdminParcelListItem{{ID: testParcelID, TrackingNumber: testTracking, Status: "FAILED", CourierID: stringPtr("courier-1")}},
		Pagination: client.Pagination{TotalItems: 3},
	}}
	registry := newRegistry(t, &fakePricing{}, parcel)
	caller := callerWithRole("DISPATCHER")

	result := execute(t, registry, "search_parcels", map[string]any{
		"status": "FAILED", "trackingNumber": "SDK", "from": "2026-11-01", "to": "2026-11-20",
	}, caller)
	if result.Status != contract.StatusOK {
		t.Fatalf("search = %q (%s)", result.Status, result.Message)
	}
	if parcel.lastAdminFilter.Status != "FAILED" || parcel.lastAdminFilter.TrackingNumber != "SDK" ||
		parcel.lastAdminFilter.FromDate != "2026-11-01" || parcel.lastAdminFilter.ToDate != "2026-11-20" {
		t.Fatalf("filter not forwarded: %+v", parcel.lastAdminFilter)
	}
	encoded := marshal(t, result.Data)
	if !strings.Contains(encoded, `"totalItems":3`) || !strings.Contains(encoded, `"hasMore":true`) {
		t.Fatalf("search result missing pagination truth: %s", encoded)
	}
	for _, forbidden := range []string{"customerId", "merchantId", "phone", "address", "pickupAddress", "dropoffAddress"} {
		if strings.Contains(encoded, forbidden) {
			t.Fatalf("search leaked %q: %s", forbidden, encoded)
		}
	}

	for name, args := range map[string]map[string]any{
		"unknown field":  {"ownerId": testUserID},
		"bad status":     {"status": "in_transit"},
		"bad date":       {"from": "2026-13-40"},
		"reversed range": {"from": "2026-11-20", "to": "2026-11-01"},
		"bad tracking":   {"trackingNumber": "SDK 123"},
	} {
		t.Run(name, func(t *testing.T) {
			parcel := &fakeParcel{}
			registry := newRegistry(t, &fakePricing{}, parcel)
			result := execute(t, registry, "search_parcels", args, callerWithRole("ADMIN"))
			if result.Status != contract.StatusInvalid {
				t.Fatalf("status = %q, want INVALID", result.Status)
			}
			if parcel.adminCalls != 0 {
				t.Fatal("invalid arguments reached upstream")
			}
		})
	}
}

func TestCourierToolValidationAndScope(t *testing.T) {
	parcel := &fakeParcel{courierList: client.CourierDeliveryList{
		Items:      []client.CourierDeliveryItem{{ID: testParcelID, TrackingNumber: testTracking, WeightKg: "2"}},
		Pagination: client.Pagination{TotalItems: 1},
	}}
	registry := newRegistry(t, &fakePricing{}, parcel)
	caller := callerWithRole("COURIER")

	result := execute(t, registry, "get_my_courier_deliveries", map[string]any{"status": "DELIVERED", "date": "2026-11-20"}, caller)
	if result.Status != contract.StatusOK {
		t.Fatalf("courier list = %q (%s)", result.Status, result.Message)
	}
	if parcel.lastCourierStatus != "DELIVERED" || parcel.lastCourierDate != "2026-11-20" || parcel.lastCourierToken != testToken {
		t.Fatalf("courier filter not forwarded: %+v", parcel.lastCourierStatus)
	}
	if strings.Contains(marshal(t, result.Data), "phone") {
		t.Fatalf("courier list leaked phone")
	}

	for name, args := range map[string]map[string]any{
		"courier id":  {"courierId": testUserID},
		"bad date":    {"date": "2026-02-30"},
		"bad status":  {"status": "UNKNOWN"},
		"extra field": {"status": "FAILED", "courierId": testUserID},
	} {
		t.Run(name, func(t *testing.T) {
			parcel := &fakeParcel{}
			registry := newRegistry(t, &fakePricing{}, parcel)
			result := execute(t, registry, "get_my_courier_deliveries", args, callerWithRole("COURIER"))
			if result.Status != contract.StatusInvalid {
				t.Fatalf("status = %q, want INVALID", result.Status)
			}
			if parcel.courierListCalls != 0 {
				t.Fatal("invalid courier arguments reached upstream")
			}
		})
	}
}

func TestMerchantTrackingExactMatchWithinRecentOrders(t *testing.T) {
	merchant := &fakeMerchant{orders: client.MerchantOrderList{
		Items: []client.MerchantOrderItem{
			{OrderID: "order-1", TrackingNumber: "SDN4T8W2LC6", Total: "550.00", Currency: "LKR"},
			{OrderID: "order-2", TrackingNumber: "SDN4T8W2LC7", Total: "600.00", Currency: "LKR"},
		},
		Pagination: client.Pagination{TotalItems: 2},
	}}
	registry := newRegistryWithMerchant(t, &fakePricing{}, &fakeParcel{}, merchant)
	caller := callerWithRole("MERCHANT")

	result := execute(t, registry, "get_my_merchant_orders", map[string]any{"trackingNumber": "SDN4T8W2LC7"}, caller)
	if result.Status != contract.StatusOK {
		t.Fatalf("match = %q (%s)", result.Status, result.Message)
	}
	encoded := marshal(t, result.Data)
	if !strings.Contains(encoded, `"orderId":"order-2"`) || strings.Contains(encoded, `"orderId":"order-1"`) {
		t.Fatalf("exact tracking match failed: %s", encoded)
	}
	if !strings.Contains(encoded, `"searchWindow":"most recent 100 orders"`) {
		t.Fatalf("scan window not disclosed: %s", encoded)
	}
	if merchant.lastOrderFilter.PageSize != 100 {
		t.Fatalf("scan page size = %d", merchant.lastOrderFilter.PageSize)
	}

	noMatch := execute(t, registry, "get_my_merchant_orders", map[string]any{"trackingNumber": "SDN99999999"}, caller)
	if noMatch.Status != contract.StatusInvalid {
		t.Fatalf("no-match status = %q, want INVALID", noMatch.Status)
	}
	if !strings.Contains(noMatch.Message, "most recent 100 orders") {
		t.Fatalf("no-match message must disclose the scan window: %q", noMatch.Message)
	}

	for name, args := range map[string]map[string]any{
		"merchant id": {"merchantId": "11111111-1111-1111-1111-111111111111"},
		"bad status":  {"status": "PENDING"},
		"bad track":   {"trackingNumber": "SD-1"},
	} {
		t.Run(name, func(t *testing.T) {
			merchant := &fakeMerchant{}
			registry := newRegistryWithMerchant(t, &fakePricing{}, &fakeParcel{}, merchant)
			result := execute(t, registry, "get_my_merchant_orders", args, caller)
			if result.Status != contract.StatusInvalid {
				t.Fatalf("status = %q, want INVALID", result.Status)
			}
			if merchant.ordersCalls != 0 {
				t.Fatal("invalid merchant arguments reached upstream")
			}
		})
	}
}

func TestWebhookDeliveriesFiltersAndNotConfigured(t *testing.T) {
	merchant := &fakeMerchant{webhook: client.WebhookDeliveryList{
		Items:      []client.WebhookDeliveryAttempt{{EventID: "event-1", TrackingNumber: "SDN4T8W2LC6", AttemptNumber: 2, Success: false, DeadLettered: true}},
		Pagination: client.Pagination{TotalItems: 1},
	}}
	registry := newRegistryWithMerchant(t, &fakePricing{}, &fakeParcel{}, merchant)

	result := execute(t, registry, "get_my_webhook_deliveries", map[string]any{"success": false}, callerWithRole("MERCHANT"))
	if result.Status != contract.StatusOK {
		t.Fatalf("webhook = %q (%s)", result.Status, result.Message)
	}
	if merchant.lastWebhookFilter == nil || *merchant.lastWebhookFilter {
		t.Fatalf("success filter not forwarded: %+v", merchant.lastWebhookFilter)
	}

	notConfigured := &fakeMerchant{webhookErr: &client.Error{Kind: client.KindNotFound, Status: 404, Code: "WEBHOOK_NOT_CONFIGURED"}}
	registry = newRegistryWithMerchant(t, &fakePricing{}, &fakeParcel{}, notConfigured)
	result = execute(t, registry, "get_my_webhook_deliveries", nil, callerWithRole("MERCHANT"))
	if result.Status != contract.StatusInvalid || !strings.Contains(result.Message, "No webhook is configured") {
		t.Fatalf("not-configured = %q %q", result.Status, result.Message)
	}

	parcel := &fakeParcel{}
	merchant = &fakeMerchant{}
	registry = newRegistryWithMerchant(t, &fakePricing{}, parcel, merchant)
	result = execute(t, registry, "get_my_webhook_deliveries", map[string]any{"success": "yes"}, callerWithRole("MERCHANT"))
	if result.Status != contract.StatusInvalid || merchant.webhookCalls != 0 {
		t.Fatalf("bad success filter = %q calls=%d", result.Status, merchant.webhookCalls)
	}
}

func TestOwnedReadFailureMapping(t *testing.T) {
	cases := []struct {
		kind client.Kind
		want contract.ToolStatus
	}{
		{client.KindUnauthorized, contract.StatusDenied},
		{client.KindNotFound, contract.StatusInvalid},
		{client.KindInvalid, contract.StatusInvalid},
		{client.KindUnavailable, contract.StatusUnavailable},
	}
	for _, testCase := range cases {
		merchant := &fakeMerchant{ordersErr: &client.Error{Kind: testCase.kind, Status: 500}}
		registry := newRegistryWithMerchant(t, &fakePricing{}, &fakeParcel{}, merchant)
		result := execute(t, registry, "get_my_merchant_orders", nil, callerWithRole("MERCHANT"))
		if result.Status != testCase.want {
			t.Fatalf("kind %d status = %q, want %q", testCase.kind, result.Status, testCase.want)
		}

		parcel := &fakeParcel{courierListErr: &client.Error{Kind: testCase.kind, Status: 500}}
		registry = newRegistry(t, &fakePricing{}, parcel)
		result = execute(t, registry, "get_my_courier_deliveries", nil, callerWithRole("COURIER"))
		if result.Status != testCase.want {
			t.Fatalf("courier kind %d status = %q, want %q", testCase.kind, result.Status, testCase.want)
		}
	}
}

func TestWorkloadSortsByActiveDeliveries(t *testing.T) {
	parcel := &fakeParcel{workload: []client.CourierWorkload{
		{CourierID: "c1", FullName: "B", ActiveDeliveries: 1},
		{CourierID: "c2", FullName: "A", ActiveDeliveries: 5},
		{CourierID: "c3", FullName: "C", ActiveDeliveries: 1},
	}}
	registry := newRegistry(t, &fakePricing{}, parcel)
	result := execute(t, registry, "get_courier_workload", nil, callerWithRole("DISPATCHER"))
	if result.Status != contract.StatusOK {
		t.Fatalf("workload = %q (%s)", result.Status, result.Message)
	}
	encoded := marshal(t, result.Data)
	if strings.Index(encoded, `"courierId":"c2"`) > strings.Index(encoded, `"courierId":"c1"`) {
		t.Fatalf("workload not sorted by active deliveries: %s", encoded)
	}
	if parcel.lastWorkloadTok != testToken {
		t.Fatal("workload did not forward the caller token")
	}
}

func TestEmptyResultsAreOKNotFailures(t *testing.T) {
	parcel := &fakeParcel{}
	registry := newRegistry(t, &fakePricing{}, parcel)
	for _, testCase := range []struct {
		tool   string
		caller contract.CallerContext
	}{
		{"get_active_parcels", callerWithRole("CUSTOMER")},
		{"search_parcels", callerWithRole("ADMIN")},
		{"list_unassigned_parcels", callerWithRole("DISPATCHER")},
		{"get_courier_workload", callerWithRole("ADMIN")},
		{"get_my_courier_deliveries", callerWithRole("COURIER")},
	} {
		result := execute(t, registry, testCase.tool, nil, testCase.caller)
		if result.Status != contract.StatusOK {
			t.Fatalf("%s empty result = %q (%s)", testCase.tool, result.Status, result.Message)
		}
	}
}

func roleLabel(role string) string {
	if role == "" {
		return "GUEST"
	}
	return role
}

// Guard against a registry that forgets role filtering after a refactor.
func TestRegistryListRequiresVerifiedIdentityForPrivateTools(t *testing.T) {
	registry := newRegistry(t, &fakePricing{}, &fakeParcel{})
	spoofed := contract.CallerContext{UserID: testUserID, Role: "ADMIN"}
	names := map[string]bool{}
	for _, definition := range registry.List(spoofed) {
		names[definition.Name] = true
	}
	if names["search_parcels"] || names["get_active_parcels"] || names["get_parcel_details"] {
		t.Fatalf("an unauthenticated caller saw private tools: %v", names)
	}
	if !names["calculate_delivery_fee"] || !names["get_public_parcel_status"] {
		t.Fatalf("guest tools are missing: %v", names)
	}

	result, err := registry.Execute(context.Background(), "search_parcels", nil, spoofed)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Status != contract.StatusDenied {
		t.Fatalf("spoofed identity executed a staff tool: %q", result.Status)
	}
}
