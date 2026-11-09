package tools

import (
	"context"
	"encoding/json"

	"github.com/Vineth-R/swiftdrop/apps/services/ai-assistant/internal/contract"
)

const (
	toolNameMyActiveDeliveries = "get_my_active_deliveries"
	maxDeliveries              = 20
)

var parcelStatuses = []string{
	"CREATED",
	"PICKED_UP",
	"IN_TRANSIT",
	"OUT_FOR_DELIVERY",
	"DELIVERED",
	"FAILED",
}

type deliveriesTool struct {
	parcel ParcelClient
}

func (t *deliveriesTool) Definition() contract.ToolDeclaration {
	return contract.ToolDeclaration{
		Name: toolNameMyActiveDeliveries,
		Description: "List recent parcels. Signed-in customers receive their own deliveries; dispatchers and administrators receive " +
			"operational parcels across customers. Requires a customer, dispatcher or administrator session. " +
			"Never invent or accept a user ID, customer ID, merchant ID or owner ID; identity and authorization come from the verified session. " +
			"For a customer asking about their own parcel status without a tracking number, use this tool: if exactly one relevant delivery " +
			"exists, report its real status; if several exist, ask which one by tracking number or destination; if none exist, say so.",
		Parameters: contract.Parameters{
			Properties: map[string]contract.Parameter{
				"status": {
					Type:        "string",
					Description: "Optional parcel status filter.",
					Enum:        parcelStatuses,
				},
			},
			Required: nil,
		},
	}
}

type deliveryEntry struct {
	ID             string      `json:"id"`
	TrackingNumber string      `json:"trackingNumber"`
	Status         string      `json:"status"`
	PickupCity     string      `json:"pickupCity,omitempty"`
	DropoffCity    string      `json:"dropoffCity,omitempty"`
	RecipientName  string      `json:"recipientName,omitempty"`
	Total          json.Number `json:"total,omitempty"`
	Currency       string      `json:"currency,omitempty"`
	CourierID      *string     `json:"courierId,omitempty"`
	CreatedAt      string      `json:"createdAt"`
	Active         bool        `json:"active"`
}

type deliveriesResult struct {
	Scope       string          `json:"scope"`
	Deliveries  []deliveryEntry `json:"deliveries"`
	Count       int             `json:"count"`
	ActiveCount int             `json:"activeCount"`
	TotalItems  int             `json:"totalItems"`
}

func (t *deliveriesTool) Execute(ctx context.Context, args map[string]any, caller contract.CallerContext) (contract.ToolResult, error) {
	if denial, ok := requireParcelAccess(caller, "Please sign in to view your deliveries."); !ok {
		return denial, nil
	}

	values, message := validateArguments(args, []argSpec{
		{name: "status", kind: argString, required: false, enum: parcelStatuses},
	})
	if message != "" {
		return invalidResult(message), nil
	}

	status := ""
	if value, ok := values["status"].(string); ok {
		status = value
	}

	if isPrivilegedParcelRole(caller.Role) {
		list, err := t.parcel.AdminParcels(ctx, status, caller.AccessToken)
		if err != nil {
			return privateFailure(err), nil
		}
		entries := make([]deliveryEntry, 0, min(len(list.Items), maxDeliveries))
		for _, item := range list.Items {
			if len(entries) >= maxDeliveries {
				break
			}
			entries = append(entries, deliveryEntry{
				ID:             item.ID,
				TrackingNumber: item.TrackingNumber,
				Status:         item.Status,
				PickupCity:     item.PickupCity,
				DropoffCity:    item.DropoffCity,
				CourierID:      item.CourierID,
				CreatedAt:      item.CreatedAt,
				Active:         isActiveStatus(item.Status),
			})
		}
		return deliveryResult("operations", entries, list.Pagination.TotalItems), nil
	}

	list, err := t.parcel.List(ctx, status, caller.AccessToken)
	if err != nil {
		return privateFailure(err), nil
	}
	entries := make([]deliveryEntry, 0, min(len(list.Items), maxDeliveries))
	for _, item := range list.Items {
		if len(entries) >= maxDeliveries {
			break
		}
		entries = append(entries, deliveryEntry{
			ID:             item.ID,
			TrackingNumber: item.TrackingNumber,
			Status:         item.Status,
			DropoffCity:    item.DropoffCity,
			RecipientName:  item.RecipientName,
			Total:          item.Total,
			Currency:       item.Currency,
			CreatedAt:      item.CreatedAt,
			Active:         isActiveStatus(item.Status),
		})
	}
	return deliveryResult("customer", entries, list.Pagination.TotalItems), nil
}

func deliveryResult(scope string, entries []deliveryEntry, totalItems int) contract.ToolResult {
	activeCount := 0
	for _, entry := range entries {
		if entry.Active {
			activeCount++
		}
	}
	if totalItems < len(entries) {
		totalItems = len(entries)
	}
	return contract.ToolResult{
		Status: contract.StatusOK,
		Data: deliveriesResult{
			Scope:       scope,
			Deliveries:  entries,
			Count:       len(entries),
			ActiveCount: activeCount,
			TotalItems:  totalItems,
		},
	}
}
