package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Vineth-R/swiftdrop/apps/services/ai-assistant/internal/client"
	"github.com/Vineth-R/swiftdrop/apps/services/ai-assistant/internal/contract"
	"github.com/Vineth-R/swiftdrop/apps/services/ai-assistant/internal/conversation"
	"github.com/Vineth-R/swiftdrop/apps/services/ai-assistant/internal/knowledge"
	"github.com/Vineth-R/swiftdrop/apps/services/ai-assistant/internal/provider/mock"
	"github.com/Vineth-R/swiftdrop/apps/services/ai-assistant/internal/tools"
)

type upstreams struct {
	pricing *httptest.Server
	parcel  *httptest.Server
}

func newOrchestratorWithUpstreams(t *testing.T, provider contract.Provider, servers upstreams) *Orchestrator {
	t.Helper()
	pricing, err := client.NewPricingClient(client.Config{BaseURL: servers.pricing.URL, Timeout: time.Second})
	if err != nil {
		t.Fatalf("pricing client: %v", err)
	}
	parcel, err := client.NewParcelClient(client.Config{BaseURL: servers.parcel.URL, Timeout: time.Second})
	if err != nil {
		t.Fatalf("parcel client: %v", err)
	}
	registry, err := tools.NewRegistry(pricing, parcel)
	if err != nil {
		t.Fatalf("tools.NewRegistry: %v", err)
	}
	base, err := knowledge.Load()
	if err != nil {
		t.Fatalf("knowledge.Load: %v", err)
	}
	orchestrator, err := New(Options{
		Provider:        provider,
		Tools:           registry,
		Conversations:   conversation.New(100, time.Minute, 40, conversation.WithRandom(&testReader{})),
		Knowledge:       base,
		Temperature:     0.2,
		MaxOutputTokens: 512,
		RequestTimeout:  2 * time.Second,
		ToolTimeout:     time.Second,
		MaxToolRounds:   3,
		MaxToolCalls:    4,
		MaxHistory:      12,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return orchestrator
}

func TestEndToEndPricingUsesAuthoritativeQuote(t *testing.T) {
	var estimateBody map[string]any
	pricing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/zones":
			_, _ = w.Write([]byte(`{"data":[
				{"id":"zone-cmb","code":"CMB","name":"Colombo","districts":["Colombo"]},
				{"id":"zone-kdy","code":"KDY","name":"Kandy","districts":["Kandy"]}
			]}`))
		case "/pricing/estimate":
			if err := json.NewDecoder(r.Body).Decode(&estimateBody); err != nil {
				t.Errorf("decode estimate: %v", err)
			}
			_, _ = w.Write([]byte(`{"data":{
				"originZoneId":"zone-cmb","destinationZoneId":"zone-kdy",
				"weightKg":2,"sizeCategory":"SMALL","currency":"LKR",
				"baseFee":450,"weightCharge":100,"sizeSurcharge":0,"total":550
			}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer pricing.Close()
	parcel := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer parcel.Close()

	provider := mock.New(
		contract.GenerateResponse{ToolCalls: []contract.FunctionCall{{
			ID: "call-1", Name: "calculate_delivery_fee",
			Arguments: map[string]any{"origin": "Colombo", "destination": "Kandy", "weightKg": float64(2), "sizeCategory": "SMALL"},
		}}},
		contract.GenerateResponse{Text: "Sending 2 kg from Colombo to Kandy costs LKR 550.00."},
	)

	orchestrator := newOrchestratorWithUpstreams(t, provider, upstreams{pricing: pricing, parcel: parcel})
	output, err := orchestrator.Chat(context.Background(), ChatInput{Message: "How much to send 2 kg from Colombo to Kandy?"})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if len(output.ToolCalls) != 1 || output.ToolCalls[0].Status != "OK" {
		t.Fatalf("toolCalls = %+v", output.ToolCalls)
	}
	if estimateBody["originZoneId"] != "zone-cmb" || estimateBody["destinationZoneId"] != "zone-kdy" {
		t.Fatalf("estimate request = %+v", estimateBody)
	}
	if estimateBody["weightKg"] != float64(2) || estimateBody["sizeCategory"] != "SMALL" {
		t.Fatalf("estimate request = %+v", estimateBody)
	}

	response := findFunctionResponse(provider.Requests()[1], "calculate_delivery_fee")
	if response == nil {
		t.Fatal("function response not sent to the model")
	}
	data, ok := response.Response["data"].(map[string]any)
	if !ok {
		t.Fatalf("function response data = %#v", response.Response["data"])
	}
	if data["total"] != float64(550) || data["currency"] != "LKR" {
		t.Fatalf("quote = %+v", data)
	}
	if strings.Contains(output.Reply, "450") {
		t.Fatalf("reply included an invented component layout: %q", output.Reply)
	}
}

func TestEndToEndTrackingStripsPrivateFields(t *testing.T) {
	pricing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) }))
	defer pricing.Close()
	parcel := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/tracking/SDK7M2Q9XP4" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"data":{
			"trackingNumber":"SDK7M2Q9XP4","status":"IN_TRANSIT",
			"pickupCity":"Colombo","dropoffCity":"Kandy",
			"lastUpdatedAt":"2026-11-01T01:00:00Z",
			"timeline":[{"status":"CREATED","at":"2026-11-01T00:00:00Z"}],
			"senderName":"Private Sender","pickupAddress":"1 Secret Lane","phone":"0770000000",
			"recipientName":"Private Recipient","internalNotes":"do not leak"
		}}`))
	}))
	defer parcel.Close()

	provider := mock.New(
		contract.GenerateResponse{ToolCalls: []contract.FunctionCall{{
			Name: "get_public_parcel_status", Arguments: map[string]any{"trackingNumber": "SDK7M2Q9XP4"},
		}}},
		contract.GenerateResponse{Text: "Your parcel is in transit."},
	)
	orchestrator := newOrchestratorWithUpstreams(t, provider, upstreams{pricing: pricing, parcel: parcel})

	output, err := orchestrator.Chat(context.Background(), ChatInput{Message: "Track SDK7M2Q9XP4"})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if output.ToolCalls[0].Status != "OK" {
		t.Fatalf("toolCalls = %+v", output.ToolCalls)
	}

	response := findFunctionResponse(provider.Requests()[1], "get_public_parcel_status")
	encoded, _ := json.Marshal(response)
	text := string(encoded)
	for _, secret := range []string{"Private Sender", "Secret Lane", "0770000000", "Private Recipient", "internalNotes", "do not leak"} {
		if strings.Contains(text, secret) {
			t.Fatalf("private field %q leaked to the model: %s", secret, text)
		}
	}
}

func TestEndToEndAnonymousPrivateToolIsDeniedBeforeUpstream(t *testing.T) {
	var parcelCalls atomic.Int32
	pricing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) }))
	defer pricing.Close()
	parcel := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		parcelCalls.Add(1)
		http.NotFound(w, r)
	}))
	defer parcel.Close()

	provider := mock.New(
		contract.GenerateResponse{ToolCalls: []contract.FunctionCall{{
			Name: "get_my_parcel_history", Arguments: map[string]any{"parcelId": "b1d1b7f0-3a7b-4f1a-9d1d-1f0f5c3a2b10"},
		}}},
		contract.GenerateResponse{Text: "Please sign in to view your parcel history."},
	)
	orchestrator := newOrchestratorWithUpstreams(t, provider, upstreams{pricing: pricing, parcel: parcel})

	output, err := orchestrator.Chat(context.Background(), ChatInput{Message: "Show my parcel history"})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if output.ToolCalls[0].Status != "DENIED" {
		t.Fatalf("toolCalls = %+v", output.ToolCalls)
	}
	if parcelCalls.Load() != 0 {
		t.Fatalf("parcel upstream calls = %d, want 0", parcelCalls.Load())
	}
}

func TestEndToEndCrossCustomerIsSafeNotFound(t *testing.T) {
	var authorization string
	pricing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) }))
	defer pricing.Close()
	parcel := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorization = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":{"code":"NOT_FOUND","message":"parcel not found"}}`))
	}))
	defer parcel.Close()

	provider := mock.New(
		contract.GenerateResponse{ToolCalls: []contract.FunctionCall{{
			Name: "get_my_parcel_history", Arguments: map[string]any{"parcelId": "b1d1b7f0-3a7b-4f1a-9d1d-1f0f5c3a2b10"},
		}}},
		contract.GenerateResponse{Text: "I could not find that parcel for your account."},
	)
	orchestrator := newOrchestratorWithUpstreams(t, provider, upstreams{pricing: pricing, parcel: parcel})

	output, err := orchestrator.Chat(context.Background(), ChatInput{
		Message: "Show my parcel history",
		Caller:  contract.CallerContext{UserID: "user-a", Authenticated: true, AccessToken: "token-a"},
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if output.ToolCalls[0].Status != "INVALID" {
		t.Fatalf("toolCalls = %+v", output.ToolCalls)
	}
	if authorization != "Bearer token-a" {
		t.Fatalf("Authorization = %q", authorization)
	}
	response := findFunctionResponse(provider.Requests()[1], "get_my_parcel_history")
	encoded, _ := json.Marshal(response)
	if strings.Contains(string(encoded), "token-a") || strings.Contains(string(encoded), "user-a") {
		t.Fatalf("credential or identity leaked to the model: %s", encoded)
	}
}

func TestEndToEndModelSuppliedIdentityIsRejected(t *testing.T) {
	var parcelCalls atomic.Int32
	pricing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) }))
	defer pricing.Close()
	parcel := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		parcelCalls.Add(1)
		http.NotFound(w, r)
	}))
	defer parcel.Close()

	provider := mock.New(
		contract.GenerateResponse{ToolCalls: []contract.FunctionCall{{
			Name: "get_my_active_deliveries",
			Arguments: map[string]any{
				"status":  "IN_TRANSIT",
				"userId":  "victim-user",
				"ownerId": "victim-user",
			},
		}}},
		contract.GenerateResponse{Text: "I cannot use an identity supplied in the request."},
	)
	orchestrator := newOrchestratorWithUpstreams(t, provider, upstreams{pricing: pricing, parcel: parcel})

	output, err := orchestrator.Chat(context.Background(), ChatInput{
		Message: "Show my deliveries",
		Caller:  contract.CallerContext{UserID: "user-a", Authenticated: true, AccessToken: "token-a"},
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if output.ToolCalls[0].Status != "INVALID" {
		t.Fatalf("toolCalls = %+v", output.ToolCalls)
	}
	if parcelCalls.Load() != 0 {
		t.Fatalf("parcel upstream calls = %d, want 0", parcelCalls.Load())
	}
}

func TestEndToEndActiveDeliveriesAllowlist(t *testing.T) {
	pricing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) }))
	defer pricing.Close()
	parcel := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/parcels" || r.URL.Query().Get("status") != "IN_TRANSIT" || r.URL.Query().Get("pageSize") != "20" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Authorization") != "Bearer token-a" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"data":[{
			"id":"b1d1b7f0-3a7b-4f1a-9d1d-1f0f5c3a2b10","trackingNumber":"SDK7M2Q9XP4",
			"status":"IN_TRANSIT","recipientName":"Nimal","dropoffCity":"Kandy",
			"total":550,"currency":"LKR","createdAt":"2026-11-01T00:00:00Z",
			"dropoffAddress":"1 Secret Lane","pickupAddress":"2 Hidden Road","phone":"0770000000"
		}],"meta":{"pagination":{"page":1,"pageSize":20,"totalItems":1,"totalPages":1}}}`))
	}))
	defer parcel.Close()

	provider := mock.New(
		contract.GenerateResponse{ToolCalls: []contract.FunctionCall{{
			Name: "get_my_active_deliveries", Arguments: map[string]any{"status": "IN_TRANSIT"},
		}}},
		contract.GenerateResponse{Text: "You have one parcel in transit."},
	)
	orchestrator := newOrchestratorWithUpstreams(t, provider, upstreams{pricing: pricing, parcel: parcel})

	output, err := orchestrator.Chat(context.Background(), ChatInput{
		Message: "Which of my deliveries are in transit?",
		Caller:  contract.CallerContext{UserID: "user-a", Authenticated: true, AccessToken: "token-a"},
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if output.ToolCalls[0].Status != "OK" {
		t.Fatalf("toolCalls = %+v", output.ToolCalls)
	}
	response := findFunctionResponse(provider.Requests()[1], "get_my_active_deliveries")
	encoded, _ := json.Marshal(response)
	text := string(encoded)
	for _, secret := range []string{"Secret Lane", "Hidden Road", "0770000000"} {
		if strings.Contains(text, secret) {
			t.Fatalf("private field %q leaked: %s", secret, text)
		}
	}
	if !strings.Contains(text, "SDK7M2Q9XP4") {
		t.Fatalf("expected allowlisted tracking number, got %s", text)
	}
}

func TestEndToEndPricingUnavailableDoesNotFabricate(t *testing.T) {
	pricing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/zones" {
			_, _ = w.Write([]byte(`{"data":[{"id":"zone-cmb","code":"CMB","name":"Colombo","districts":["Colombo"]},{"id":"zone-kdy","code":"KDY","name":"Kandy","districts":["Kandy"]}]}`))
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":{"code":"INTERNAL_ERROR","message":"secret sql error"}}`))
	}))
	defer pricing.Close()
	parcel := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) }))
	defer parcel.Close()

	provider := mock.New(
		contract.GenerateResponse{ToolCalls: []contract.FunctionCall{{
			Name:      "calculate_delivery_fee",
			Arguments: map[string]any{"origin": "Colombo", "destination": "Kandy", "weightKg": float64(2), "sizeCategory": "SMALL"},
		}}},
		contract.GenerateResponse{Text: "I cannot retrieve a delivery estimate right now."},
	)
	orchestrator := newOrchestratorWithUpstreams(t, provider, upstreams{pricing: pricing, parcel: parcel})

	output, err := orchestrator.Chat(context.Background(), ChatInput{Message: "How much to send 2 kg Colombo to Kandy?"})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if output.ToolCalls[0].Status != "UNAVAILABLE" {
		t.Fatalf("toolCalls = %+v", output.ToolCalls)
	}
	response := findFunctionResponse(provider.Requests()[1], "calculate_delivery_fee")
	encoded, _ := json.Marshal(response)
	if strings.Contains(string(encoded), "secret sql error") {
		t.Fatalf("upstream body leaked to the model: %s", encoded)
	}
	if _, ok := response.Response["data"]; ok {
		t.Fatalf("failed pricing returned fabricated data: %+v", response.Response)
	}
}

func TestRegistryContainsExactlyFourApprovedTools(t *testing.T) {
	pricing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) }))
	defer pricing.Close()
	parcel := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) }))
	defer parcel.Close()

	pricingClient, _ := client.NewPricingClient(client.Config{BaseURL: pricing.URL, Timeout: time.Second})
	parcelClient, _ := client.NewParcelClient(client.Config{BaseURL: parcel.URL, Timeout: time.Second})
	registry, err := tools.NewRegistry(pricingClient, parcelClient)
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	definitions := registry.List()
	if len(definitions) != 4 {
		t.Fatalf("definitions = %d, want 4", len(definitions))
	}
	names := map[string]bool{}
	for _, definition := range definitions {
		names[definition.Name] = true
	}
	for _, want := range []string{"calculate_delivery_fee", "get_public_parcel_status", "get_my_parcel_history", "get_my_active_deliveries"} {
		if !names[want] {
			t.Fatalf("missing tool %s in %v", want, names)
		}
	}
}

func findFunctionResponse(request contract.GenerateRequest, name string) *contract.FunctionResponse {
	for _, content := range request.Contents {
		for _, part := range content.Parts {
			if part.FunctionResponse != nil && part.FunctionResponse.Name == name {
				return part.FunctionResponse
			}
		}
	}
	return nil
}
