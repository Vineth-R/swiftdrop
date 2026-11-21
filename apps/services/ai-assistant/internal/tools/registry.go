// Package tools implements the approved read-only AI assistant tools and the
// strict allowlist registry that exposes them to the model.
package tools

import (
	"context"
	"errors"
	"sort"

	"github.com/Vineth-R/swiftdrop/apps/services/ai-assistant/internal/client"
	"github.com/Vineth-R/swiftdrop/apps/services/ai-assistant/internal/contract"
)

// PricingClient is the consumer-defined subset of the Pricing client the tools
// need. Tests provide a fake implementation.
type PricingClient interface {
	Zones(ctx context.Context) ([]client.Zone, error)
	Estimate(ctx context.Context, input client.EstimateInput) (client.Estimate, error)
}

// ParcelClient is the consumer-defined subset of the Parcel client the tools
// need. Tests provide a fake implementation.
type ParcelClient interface {
	PublicTracking(ctx context.Context, trackingNumber string) (client.Tracking, error)
	Tracking(ctx context.Context, trackingNumber, accessToken string) (client.Tracking, error)
	History(ctx context.Context, parcelID, accessToken string) ([]client.HistoryItem, error)
	Detail(ctx context.Context, parcelID, accessToken string) (client.ParcelDetail, error)
	List(ctx context.Context, status, accessToken string) (client.ParcelList, error)
	AdminParcels(ctx context.Context, filter client.AdminParcelFilter, accessToken string) (client.AdminParcelList, error)
	Unassigned(ctx context.Context, accessToken string) (client.UnassignedList, error)
	Workload(ctx context.Context, accessToken string) ([]client.CourierWorkload, error)
	CourierDeliveries(ctx context.Context, status, date, accessToken string) (client.CourierDeliveryList, error)
	CourierDelivery(ctx context.Context, parcelID, accessToken string) (client.CourierDeliveryDetail, error)
}

// MerchantClient is the consumer-defined subset of the Merchant client the
// tools need. Tests provide a fake implementation.
type MerchantClient interface {
	Orders(ctx context.Context, filter client.MerchantOrderFilter, accessToken string) (client.MerchantOrderList, error)
	WebhookDeliveries(ctx context.Context, success *bool, accessToken string) (client.WebhookDeliveryList, error)
}

// availability is the optional registry-visible filter. It only controls which
// declarations the model sees; every tool still authorizes each execution.
type availability interface {
	AvailableTo(caller contract.CallerContext) bool
}

// Registry is the closed allowlist of approved tools. It implements
// contract.ToolRegistry.
type Registry struct {
	tools map[string]contract.Tool
}

// ToolNames is the exact approved tool set. No other tool may be registered.
var ToolNames = []string{
	"calculate_delivery_fee",
	"get_public_parcel_status",
	"get_parcel_history",
	"get_active_parcels",
	"get_parcel_details",
	"search_parcels",
	"list_unassigned_parcels",
	"get_courier_workload",
	"get_my_courier_deliveries",
	"get_my_courier_delivery",
	"get_my_merchant_orders",
	"get_my_webhook_deliveries",
}

// NewRegistry builds the registry with exactly the approved tools. It fails
// when a dependency is missing rather than registering tools that would panic
// at execution time.
func NewRegistry(pricing PricingClient, parcel ParcelClient, merchant MerchantClient) (*Registry, error) {
	if pricing == nil || parcel == nil || merchant == nil {
		return nil, errors.New("ai-assistant tools require pricing, parcel and merchant clients")
	}

	registry := &Registry{tools: make(map[string]contract.Tool, len(ToolNames))}
	for _, tool := range []contract.Tool{
		&feeTool{pricing: pricing},
		&trackingTool{parcel: parcel},
		&historyTool{parcel: parcel},
		&deliveriesTool{parcel: parcel},
		&parcelDetailsTool{parcel: parcel},
		&searchParcelsTool{parcel: parcel},
		&unassignedParcelsTool{parcel: parcel},
		&courierWorkloadTool{parcel: parcel},
		&courierDeliveriesTool{parcel: parcel},
		&courierDeliveryTool{parcel: parcel},
		&merchantOrdersTool{merchant: merchant},
		&merchantWebhookDeliveriesTool{merchant: merchant},
	} {
		definition := tool.Definition()
		registry.tools[definition.Name] = tool
	}
	return registry, nil
}

// Get returns an approved tool by exact name.
func (r *Registry) Get(name string) (contract.Tool, bool) {
	if r == nil {
		return nil, false
	}
	tool, ok := r.tools[name]
	return tool, ok
}

// List returns the approved tool declarations available to the caller in
// deterministic name order. Hiding a declaration never grants or removes
// authorization; Execute re-checks the role.
func (r *Registry) List(caller contract.CallerContext) []contract.ToolDeclaration {
	if r == nil {
		return nil
	}
	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}
	sort.Strings(names)

	declarations := make([]contract.ToolDeclaration, 0, len(names))
	for _, name := range names {
		tool := r.tools[name]
		if gate, ok := tool.(availability); ok && !gate.AvailableTo(caller) {
			continue
		}
		declarations = append(declarations, tool.Definition())
	}
	return declarations
}

// Execute runs one approved tool. Unknown names are never executed; they
// return an INVALID result with no error so orchestration can report the tool
// as unavailable without treating it as a provider failure.
func (r *Registry) Execute(ctx context.Context, name string, args map[string]any, caller contract.CallerContext) (contract.ToolResult, error) {
	tool, ok := r.Get(name)
	if !ok {
		return contract.ToolResult{
			Status:  contract.StatusInvalid,
			Message: "tool is not available",
		}, nil
	}
	return tool.Execute(ctx, args, caller)
}
