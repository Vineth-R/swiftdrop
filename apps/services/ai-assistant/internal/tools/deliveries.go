package tools

import (
	"context"
	"encoding/json"
	"strings"

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
		Description: "List the signed-in customer's own recent deliveries, optionally filtered by status. " +
			"Requires a signed-in user. Never invent or accept a user ID, customer ID or owner ID; ownership is taken from the session. " +
			"Do not ask the customer for identity details.",
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
	DropoffCity    string      `json:"dropoffCity"`
	RecipientName  string      `json:"recipientName"`
	Total          json.Number `json:"total"`
	Currency       string      `json:"currency"`
	CreatedAt      string      `json:"createdAt"`
}

type deliveriesResult struct {
	Deliveries []deliveryEntry `json:"deliveries"`
	Count      int             `json:"count"`
}

func (t *deliveriesTool) Execute(ctx context.Context, args map[string]any, caller contract.CallerContext) (contract.ToolResult, error) {
	if !caller.Authenticated || strings.TrimSpace(caller.AccessToken) == "" {
		return deniedResult("Please sign in to view your deliveries."), nil
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
		})
	}

	return contract.ToolResult{
		Status: contract.StatusOK,
		Data: deliveriesResult{
			Deliveries: entries,
			Count:      len(entries),
		},
	}, nil
}
