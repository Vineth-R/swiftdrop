package tools

import (
	"context"
	"regexp"

	"github.com/Vineth-R/swiftdrop/apps/services/ai-assistant/internal/client"
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
			"Guests receive only the public tracking projection. Signed-in customers see their own parcels only; " +
			"dispatchers and administrators may look up parcels operationally. A tracking number is never proof of identity " +
			"and never grants access to private details. Never invent or accept a user, customer or owner ID.",
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

// AvailableTo is the registry-visible subset. Guests may use public tracking;
// MERCHANT and COURIER may not use this tool.
func (t *trackingTool) AvailableTo(caller contract.CallerContext) bool {
	if !caller.Authenticated {
		return true
	}
	return hasVerifiedIdentity(caller) &&
		(isCustomerRole(caller.Role) || isStaffRole(caller.Role))
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
	Scope          string          `json:"scope"`
}

func (t *trackingTool) Execute(ctx context.Context, args map[string]any, caller contract.CallerContext) (contract.ToolResult, error) {
	guest := !caller.Authenticated
	if !guest {
		if denial, ok := requireParcelAccess(caller, "Please sign in to track your parcel."); !ok {
			return denial, nil
		}
	}

	values, message := validateArguments(args, []argSpec{
		{name: "trackingNumber", kind: argString, required: true, maxLen: 32, pattern: trackingNumberPattern},
	})
	if message != "" {
		return invalidResult(message), nil
	}

	number := values["trackingNumber"].(string)

	var (
		tracking    client.Tracking
		trackingErr error
		scope       = "account"
	)
	if guest {
		scope = "public"
		tracking, trackingErr = t.parcel.PublicTracking(ctx, number)
	} else {
		tracking, trackingErr = t.parcel.Tracking(ctx, number, caller.AccessToken)
	}
	if trackingErr != nil {
		return trackingFailure(trackingErr), nil
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
			Scope:          scope,
		},
	}, nil
}
