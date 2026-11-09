package tools

import (
	"context"
	"regexp"

	"github.com/Vineth-R/swiftdrop/apps/services/ai-assistant/internal/contract"
)

const toolNamePublicParcelStatus = "get_public_parcel_status"

var trackingNumberPattern = regexp.MustCompile(`^SD[KN][A-Z0-9]{8}$`)

type trackingTool struct {
	parcel ParcelClient
}

func (t *trackingTool) Definition() contract.ToolDeclaration {
	return contract.ToolDeclaration{
		Name: toolNamePublicParcelStatus,
		Description: "Look up the delivery status of a parcel by its SwiftDrop tracking number, for example SDK7M2Q9XP4. " +
			"Only signed-in customers (their own parcels) and authorized delivery staff may use this tool. " +
			"Guests must be asked to sign in; never treat a tracking number as proof of identity. " +
			"Never invent or accept a user, customer or owner ID.",
		Parameters: contract.Parameters{
			Properties: map[string]contract.Parameter{
				"trackingNumber": {
					Type:        "string",
					Description: "SwiftDrop tracking number matching SDK or SDN followed by eight uppercase letters or digits.",
				},
			},
			Required: []string{"trackingNumber"},
		},
	}
}

type trackingEvent struct {
	Status string `json:"status"`
	At     string `json:"at"`
}

type trackingResult struct {
	TrackingNumber string          `json:"trackingNumber"`
	Status         string          `json:"status"`
	PickupCity     string          `json:"pickupCity"`
	DropoffCity    string          `json:"dropoffCity"`
	LastUpdatedAt  string          `json:"lastUpdatedAt"`
	Timeline       []trackingEvent `json:"timeline"`
}

func (t *trackingTool) Execute(ctx context.Context, args map[string]any, caller contract.CallerContext) (contract.ToolResult, error) {
	if denial, ok := requireParcelAccess(caller, "Please sign in to track your parcel."); !ok {
		return denial, nil
	}

	values, message := validateArguments(args, []argSpec{
		{name: "trackingNumber", kind: argString, required: true, maxLen: 32},
	})
	if message != "" {
		return invalidResult(message), nil
	}

	number := values["trackingNumber"].(string)
	if !trackingNumberPattern.MatchString(number) {
		return invalidResult("The tracking number format is not valid. SwiftDrop tracking numbers look like SDK7M2Q9XP4."), nil
	}

	tracking, err := t.parcel.Tracking(ctx, number, caller.AccessToken)
	if err != nil {
		return trackingFailure(err), nil
	}

	timeline := make([]trackingEvent, 0, len(tracking.Timeline))
	for _, event := range tracking.Timeline {
		timeline = append(timeline, trackingEvent{Status: event.Status, At: event.At})
	}

	return contract.ToolResult{
		Status: contract.StatusOK,
		Data: trackingResult{
			TrackingNumber: tracking.TrackingNumber,
			Status:         tracking.Status,
			PickupCity:     tracking.PickupCity,
			DropoffCity:    tracking.DropoffCity,
			LastUpdatedAt:  tracking.LastUpdatedAt,
			Timeline:       timeline,
		},
	}, nil
}
