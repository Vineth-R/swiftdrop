// Package tools implements the four approved read-only AI assistant tools and
// the strict allowlist registry that exposes them to the model.
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
	Tracking(ctx context.Context, trackingNumber string) (client.Tracking, error)
	History(ctx context.Context, parcelID, accessToken string) ([]client.HistoryItem, error)
	List(ctx context.Context, status, accessToken string) (client.ParcelList, error)
	AdminParcels(ctx context.Context, status, accessToken string) (client.AdminParcelList, error)
}

// Registry is the closed allowlist of approved tools. It implements
// contract.ToolRegistry.
type Registry struct {
	tools map[string]contract.Tool
}

// NewRegistry builds the registry with exactly the four approved tools. It
// fails when either dependency is missing rather than registering tools that
// would panic at execution time.
func NewRegistry(pricing PricingClient, parcel ParcelClient) (*Registry, error) {
	if pricing == nil || parcel == nil {
		return nil, errors.New("ai-assistant tools require pricing and parcel clients")
	}

	registry := &Registry{tools: make(map[string]contract.Tool, 4)}
	for _, tool := range []contract.Tool{
		&feeTool{pricing: pricing},
		&trackingTool{parcel: parcel},
		&historyTool{parcel: parcel},
		&deliveriesTool{parcel: parcel},
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

// List returns the approved tool declarations in deterministic name order.
func (r *Registry) List() []contract.ToolDeclaration {
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
		declarations = append(declarations, r.tools[name].Definition())
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
