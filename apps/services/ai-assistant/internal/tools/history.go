package tools

import (
	"context"
	"strings"

	"github.com/Vineth-R/swiftdrop/apps/services/ai-assistant/internal/contract"
	"github.com/Vineth-R/swiftdrop/packages/go-common/validation"
)

const toolNameMyParcelHistory = "get_my_parcel_history"

type historyTool struct {
	parcel ParcelClient
}

func (t *historyTool) Definition() contract.ToolDeclaration {
	return contract.ToolDeclaration{
		Name: toolNameMyParcelHistory,
		Description: "Read the signed-in customer's own parcel status history by parcel ID. " +
			"Requires a signed-in user. Never invent or accept a user ID, customer ID or owner ID; ownership is taken from the session. " +
			"Ask the customer for the parcel ID if it is missing.",
		Parameters: contract.Parameters{
			Properties: map[string]contract.Parameter{
				"parcelId": {
					Type:        "string",
					Description: "The parcel UUID from the customer's own parcel list.",
				},
			},
			Required: []string{"parcelId"},
		},
	}
}

type historyEntry struct {
	PreviousStatus *string `json:"previousStatus"`
	Status         string  `json:"status"`
	ChangedAt      string  `json:"changedAt"`
	Note           *string `json:"note"`
}

type historyResult struct {
	History []historyEntry `json:"history"`
}

func (t *historyTool) Execute(ctx context.Context, args map[string]any, caller contract.CallerContext) (contract.ToolResult, error) {
	if !caller.Authenticated || strings.TrimSpace(caller.AccessToken) == "" || strings.TrimSpace(caller.UserID) == "" {
		return deniedResult("Please sign in to view your parcel history."), nil
	}

	values, message := validateArguments(args, []argSpec{
		{name: "parcelId", kind: argString, required: true, maxLen: 64},
	})
	if message != "" {
		return invalidResult(message), nil
	}

	parcelID := values["parcelId"].(string)
	if !validation.UUID(parcelID) {
		return invalidResult("The parcel ID must be a valid UUID."), nil
	}

	items, err := t.parcel.History(ctx, parcelID, caller.AccessToken)
	if err != nil {
		return privateFailure(err), nil
	}

	entries := make([]historyEntry, 0, len(items))
	for _, item := range items {
		entries = append(entries, historyEntry{
			PreviousStatus: item.PreviousStatus,
			Status:         item.Status,
			ChangedAt:      item.ChangedAt,
			Note:           item.Note,
		})
	}

	return contract.ToolResult{
		Status: contract.StatusOK,
		Data:   historyResult{History: entries},
	}, nil
}
