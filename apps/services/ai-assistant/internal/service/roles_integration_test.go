package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Vineth-R/swiftdrop/apps/services/ai-assistant/internal/contract"
	"github.com/Vineth-R/swiftdrop/apps/services/ai-assistant/internal/provider/mock"
)

const (
	testCourierToken  = "courier-access-token"
	testMerchantToken = "merchant-access-token"
	testStaffToken    = "staff-access-token"
)

func testCaller(role, token string) contract.CallerContext {
	return contract.CallerContext{
		UserID:        "0c5f8e3a-1d2b-4c6e-8f90-123456789abc",
		Role:          role,
		Authenticated: true,
		AccessToken:   token,
	}
}

func toolNames(request contract.GenerateRequest) map[string]bool {
	names := map[string]bool{}
	for _, declaration := range request.Tools {
		names[declaration.Name] = true
	}
	return names
}

func TestEndToEndCourierToolsUseOwnScopedData(t *testing.T) {
	pricing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) }))
	defer pricing.Close()

	var calls atomic.Int32
	parcel := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path != "/courier/deliveries" {
			t.Errorf("courier call path = %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Authorization") != "Bearer "+testCourierToken {
			t.Errorf("courier token not forwarded: %q", r.Header.Get("Authorization"))
		}
		if r.URL.Query().Get("courierId") != "" {
			t.Errorf("courier id was sent as a filter")
		}
		_, _ = io.WriteString(w, `{"data":[{"id":"p1","trackingNumber":"SDK7M2Q9XP4","status":"CREATED","pickupAddress":"1 Secret Lane","dropoffAddress":"2 Secret Lane","dropoffCity":"Kandy","recipientName":"Recipient","recipientPhone":"0770000000","weightKg":2.00,"assignedAt":"2026-11-01T00:00:00Z"}],"meta":{"pagination":{"page":1,"pageSize":20,"totalItems":1,"totalPages":1}}}`)
	}))
	defer parcel.Close()

	provider := mock.New(
		contract.GenerateResponse{ToolCalls: []contract.FunctionCall{{
			Name: "get_my_courier_deliveries", Arguments: map[string]any{"status": "CREATED"},
		}}},
		contract.GenerateResponse{Text: "You have one delivery waiting."},
	)
	orchestrator := newOrchestratorWithUpstreams(t, provider, upstreams{pricing: pricing, parcel: parcel})

	output, err := orchestrator.Chat(context.Background(), ChatInput{
		Message: "What deliveries are assigned to me?",
		Caller:  testCaller("COURIER", testCourierToken),
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if len(output.ToolCalls) != 1 || output.ToolCalls[0].Status != "OK" {
		t.Fatalf("toolCalls = %+v", output.ToolCalls)
	}
	if calls.Load() != 1 {
		t.Fatalf("courier upstream calls = %d", calls.Load())
	}

	declared := toolNames(provider.Requests()[0])
	for _, want := range []string{"calculate_delivery_fee", "get_my_courier_deliveries", "get_my_courier_delivery"} {
		if !declared[want] {
			t.Fatalf("courier tool %q is not declared: %v", want, declared)
		}
	}
	for _, forbidden := range []string{"search_parcels", "get_my_merchant_orders", "get_active_parcels"} {
		if declared[forbidden] {
			t.Fatalf("courier sees a hidden tool %q", forbidden)
		}
	}

	response := findFunctionResponse(provider.Requests()[1], "get_my_courier_deliveries")
	encoded, _ := json.Marshal(response)
	for _, secret := range []string{"0770000000", "Secret Lane", "recipientPhone", "pickupAddress"} {
		if strings.Contains(string(encoded), secret) {
			t.Fatalf("courier result leaked %q: %s", secret, encoded)
		}
	}
	if !strings.Contains(provider.Requests()[0].SystemInstruction, "Caller role: courier") {
		t.Fatal("courier role guidance is missing")
	}
}

func TestEndToEndMerchantTrackingUsesExactRecentMatch(t *testing.T) {
	pricing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) }))
	defer pricing.Close()
	parcel := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) }))
	defer parcel.Close()

	merchant := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/merchant/orders" {
			t.Errorf("merchant path = %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Authorization") != "Bearer "+testMerchantToken {
			t.Errorf("merchant token not forwarded")
		}
		_, _ = io.WriteString(w, `{"data":[
			{"orderId":"11111111-1111-1111-1111-111111111111","externalOrderId":"SZ-1","trackingNumber":"SDN4T8W2LC6","status":"IN_TRANSIT","recipientName":"Recipient","dropoffCity":"Kandy","total":550.00,"currency":"LKR","createdAt":"2026-11-01T00:00:00Z","updatedAt":"2026-11-02T00:00:00Z","recipientPhone":"0770000000"},
			{"orderId":"22222222-2222-2222-2222-222222222222","externalOrderId":"SZ-2","trackingNumber":"SDN4T8W2LC7","status":"FAILED","recipientName":"Recipient","dropoffCity":"Galle","total":600.00,"currency":"LKR","createdAt":"2026-11-01T00:00:00Z","updatedAt":"2026-11-02T00:00:00Z"}
		],"meta":{"pagination":{"page":1,"pageSize":100,"totalItems":2,"totalPages":1}}}`)
	}))
	defer merchant.Close()

	provider := mock.New(
		contract.GenerateResponse{ToolCalls: []contract.FunctionCall{{
			Name: "get_my_merchant_orders", Arguments: map[string]any{"trackingNumber": "SDN4T8W2LC7"},
		}}},
		contract.GenerateResponse{Text: "That order failed."},
	)
	orchestrator := newOrchestratorWithUpstreams(t, provider, upstreams{pricing: pricing, parcel: parcel, merchant: merchant})

	output, err := orchestrator.Chat(context.Background(), ChatInput{
		Message: "Track order SDN4T8W2LC7",
		Caller:  testCaller("MERCHANT", testMerchantToken),
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if output.ToolCalls[0].Status != "OK" {
		t.Fatalf("toolCalls = %+v", output.ToolCalls)
	}

	response := findFunctionResponse(provider.Requests()[1], "get_my_merchant_orders")
	encoded, _ := json.Marshal(response)
	if !strings.Contains(string(encoded), "SDN4T8W2LC7") || strings.Contains(string(encoded), "SDN4T8W2LC6") {
		t.Fatalf("tracking match returned the wrong orders: %s", encoded)
	}
	if strings.Contains(string(encoded), "0770000000") {
		t.Fatalf("merchant result leaked a phone: %s", encoded)
	}
	if !strings.Contains(provider.Requests()[0].SystemInstruction, "Caller role: merchant") {
		t.Fatal("merchant role guidance is missing")
	}
}

func TestEndToEndStaffToolsAndHiddenToolDenial(t *testing.T) {
	pricing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) }))
	defer pricing.Close()

	var staffCalls atomic.Int32
	parcel := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/dispatch/unassigned":
			staffCalls.Add(1)
			_, _ = io.WriteString(w, `{"data":[{"id":"p1","trackingNumber":"SDK7M2Q9XP4","status":"CREATED","pickupAddress":"1 Secret Lane","pickupCity":"Colombo","dropoffAddress":"2 Secret Lane","dropoffCity":"Kandy","weightKg":2.00,"sizeCategory":"SMALL","createdAt":"2026-11-01T00:00:00Z"}],"meta":{"pagination":{"page":1,"pageSize":20,"totalItems":1,"totalPages":1}}}`)
		case "/dispatch/workload":
			staffCalls.Add(1)
			_, _ = io.WriteString(w, `{"data":[{"courierId":"c1","fullName":"Courier","phone":"0770000000","isAvailable":true,"activeDeliveries":4,"byStatus":{"IN_TRANSIT":4}}],"meta":{}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer parcel.Close()

	t.Run("dispatcher sees and can use staff tools", func(t *testing.T) {
		provider := mock.New(
			contract.GenerateResponse{ToolCalls: []contract.FunctionCall{{Name: "list_unassigned_parcels", Arguments: map[string]any{}}}},
			contract.GenerateResponse{ToolCalls: []contract.FunctionCall{{Name: "get_courier_workload", Arguments: map[string]any{}}}},
			contract.GenerateResponse{Text: "One parcel is unassigned; one courier has four active deliveries."},
		)
		orchestrator := newOrchestratorWithUpstreams(t, provider, upstreams{pricing: pricing, parcel: parcel})
		output, err := orchestrator.Chat(context.Background(), ChatInput{
			Message: "Show me unassigned parcels and courier workload",
			Caller:  testCaller("DISPATCHER", testStaffToken),
		})
		if err != nil {
			t.Fatalf("Chat: %v", err)
		}
		if len(output.ToolCalls) != 2 || output.ToolCalls[0].Status != "OK" || output.ToolCalls[1].Status != "OK" {
			t.Fatalf("toolCalls = %+v", output.ToolCalls)
		}
		declared := toolNames(provider.Requests()[0])
		if !declared["search_parcels"] || !declared["list_unassigned_parcels"] || !declared["get_courier_workload"] {
			t.Fatalf("staff tools missing: %v", declared)
		}
		if declared["get_my_courier_deliveries"] || declared["get_my_merchant_orders"] {
			t.Fatalf("staff sees another role's tool: %v", declared)
		}
		response := findFunctionResponse(provider.Requests()[1], "list_unassigned_parcels")
		encoded, _ := json.Marshal(response)
		if strings.Contains(string(encoded), "Secret Lane") {
			t.Fatalf("unassigned leaked an address: %s", encoded)
		}
	})

	t.Run("courier naming a staff tool is denied before upstream", func(t *testing.T) {
		staffCalls.Store(0)
		provider := mock.New(
			contract.GenerateResponse{ToolCalls: []contract.FunctionCall{{Name: "list_unassigned_parcels", Arguments: map[string]any{}}}},
			contract.GenerateResponse{Text: "That information is only for dispatch staff."},
		)
		orchestrator := newOrchestratorWithUpstreams(t, provider, upstreams{pricing: pricing, parcel: parcel})
		output, err := orchestrator.Chat(context.Background(), ChatInput{
			Message: "Show every unassigned parcel",
			Caller:  testCaller("COURIER", testCourierToken),
		})
		if err != nil {
			t.Fatalf("Chat: %v", err)
		}
		if output.ToolCalls[0].Status != "DENIED" {
			t.Fatalf("toolCalls = %+v", output.ToolCalls)
		}
		if staffCalls.Load() != 0 {
			t.Fatalf("denied courier reached the staff upstream %d times", staffCalls.Load())
		}
	})
}

func TestEndToEndRoleToolDeclarations(t *testing.T) {
	pricing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) }))
	defer pricing.Close()
	parcel := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) }))
	defer parcel.Close()
	merchant := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) }))
	defer merchant.Close()

	cases := []struct {
		name   string
		caller contract.CallerContext
		want   []string
		deny   []string
	}{
		{
			name:   "guest",
			caller: contract.CallerContext{SessionID: testGuestSession},
			want:   []string{"calculate_delivery_fee", "get_public_parcel_status"},
			deny:   []string{"get_parcel_history", "search_parcels", "get_my_merchant_orders"},
		},
		{
			name:   "customer",
			caller: testCaller("CUSTOMER", "customer-token"),
			want:   []string{"calculate_delivery_fee", "get_public_parcel_status", "get_parcel_history", "get_active_parcels", "get_parcel_details"},
			deny:   []string{"search_parcels", "get_my_courier_deliveries", "get_my_webhook_deliveries"},
		},
		{
			name:   "merchant",
			caller: testCaller("MERCHANT", testMerchantToken),
			want:   []string{"calculate_delivery_fee", "get_my_merchant_orders", "get_my_webhook_deliveries"},
			deny:   []string{"get_public_parcel_status", "get_active_parcels", "list_unassigned_parcels"},
		},
		{
			name:   "admin",
			caller: testCaller("ADMIN", testStaffToken),
			want:   []string{"search_parcels", "list_unassigned_parcels", "get_courier_workload"},
			deny:   []string{"get_my_courier_deliveries", "get_my_merchant_orders"},
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			provider := mock.New(contract.GenerateResponse{Text: "Answer."})
			orchestrator := newOrchestratorWithUpstreams(t, provider, upstreams{pricing: pricing, parcel: parcel, merchant: merchant})
			if _, err := orchestrator.Chat(context.Background(), ChatInput{
				Message: "How much is delivery from Colombo to Kandy?",
				Caller:  testCase.caller,
			}); err != nil {
				t.Fatalf("Chat: %v", err)
			}
			declared := toolNames(provider.Requests()[0])
			for _, want := range testCase.want {
				if !declared[want] {
					t.Fatalf("%s is missing tool %q: %v", testCase.name, want, declared)
				}
			}
			for _, deny := range testCase.deny {
				if declared[deny] {
					t.Fatalf("%s sees hidden tool %q", testCase.name, deny)
				}
			}
		})
	}
}

func TestEndToEndMerchantWebhookNotConfiguredIsExplained(t *testing.T) {
	pricing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) }))
	defer pricing.Close()
	parcel := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) }))
	defer parcel.Close()
	merchant := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"error":{"code":"WEBHOOK_NOT_CONFIGURED","message":"no webhook is configured"},"meta":{}}`)
	}))
	defer merchant.Close()

	provider := mock.New(
		contract.GenerateResponse{ToolCalls: []contract.FunctionCall{{Name: "get_my_webhook_deliveries", Arguments: map[string]any{"success": false}}}},
		contract.GenerateResponse{Text: "You have no webhook configured yet."},
	)
	orchestrator := newOrchestratorWithUpstreams(t, provider, upstreams{pricing: pricing, parcel: parcel, merchant: merchant})
	output, err := orchestrator.Chat(context.Background(), ChatInput{
		Message: "Are there any webhook delivery failures?",
		Caller:  testCaller("MERCHANT", testMerchantToken),
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if output.ToolCalls[0].Status != "INVALID" {
		t.Fatalf("toolCalls = %+v", output.ToolCalls)
	}
	response := findFunctionResponse(provider.Requests()[1], "get_my_webhook_deliveries")
	encoded, _ := json.Marshal(response)
	if !strings.Contains(string(encoded), "No webhook is configured") {
		t.Fatalf("configuration state was not explained: %s", encoded)
	}
}
