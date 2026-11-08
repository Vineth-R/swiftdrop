package tools

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/Vineth-R/swiftdrop/apps/services/ai-assistant/internal/client"
	"github.com/Vineth-R/swiftdrop/apps/services/ai-assistant/internal/contract"
)

const (
	toolNameCalculateDeliveryFee = "calculate_delivery_fee"
	maxLocationLength            = 120
)

var sizeCategories = []string{"SMALL", "MEDIUM", "LARGE", "OVERSIZED"}

type feeTool struct {
	pricing PricingClient
}

func (t *feeTool) Definition() contract.ToolDeclaration {
	return contract.ToolDeclaration{
		Name: toolNameCalculateDeliveryFee,
		Description: "Calculate the authoritative SwiftDrop delivery fee for a route. " +
			"Ask the customer for the pickup location, drop-off location, weight in kilograms and parcel size if any are missing. " +
			"Never estimate, calculate or invent a price yourself; report only the quote this tool returns.",
		Parameters: contract.Parameters{
			Properties: map[string]contract.Parameter{
				"origin": {
					Type:        "string",
					Description: "Pickup zone name, zone code or district, for example Colombo.",
				},
				"destination": {
					Type:        "string",
					Description: "Drop-off zone name, zone code or district, for example Kandy.",
				},
				"weightKg": {
					Type:        "number",
					Description: "Parcel weight in kilograms. Must be greater than 0 and have at most two decimal places.",
				},
				"sizeCategory": {
					Type:        "string",
					Description: "Parcel size category.",
					Enum:        sizeCategories,
				},
			},
			Required: []string{"origin", "destination", "weightKg", "sizeCategory"},
		},
	}
}

type feeResult struct {
	Origin        string      `json:"origin"`
	Destination   string      `json:"destination"`
	WeightKg      json.Number `json:"weightKg"`
	SizeCategory  string      `json:"sizeCategory"`
	Currency      string      `json:"currency"`
	BaseFee       json.Number `json:"baseFee"`
	WeightCharge  json.Number `json:"weightCharge"`
	SizeSurcharge json.Number `json:"sizeSurcharge"`
	Total         json.Number `json:"total"`
}

func (t *feeTool) Execute(ctx context.Context, args map[string]any, _ contract.CallerContext) (contract.ToolResult, error) {
	values, message := validateArguments(args, []argSpec{
		{name: "origin", kind: argString, required: true, maxLen: maxLocationLength},
		{name: "destination", kind: argString, required: true, maxLen: maxLocationLength},
		{name: "weightKg", kind: argNumber, required: true},
		{name: "sizeCategory", kind: argString, required: true, enum: sizeCategories},
	})
	if message != "" {
		return invalidResult(message), nil
	}

	weight, ok := values["weightKg"].(json.Number)
	if !ok || !validWeight(weight) {
		return invalidResult("weightKg must be greater than 0, at most 999999 and have at most two decimal places."), nil
	}
	origin := values["origin"].(string)
	destination := values["destination"].(string)
	sizeCategory := values["sizeCategory"].(string)

	zones, err := t.pricing.Zones(ctx)
	if err != nil {
		return unavailableResult("The zone directory is temporarily unavailable. Please try again later."), nil
	}

	originZone, match := resolveZone(zones, origin)
	switch match {
	case zoneAmbiguous:
		return invalidResult("The pickup location matches more than one zone. Ask for a more specific district or zone."), nil
	case zoneNotFound:
		return invalidResult("The pickup location is not in the serviceable zone directory."), nil
	}

	destinationZone, match := resolveZone(zones, destination)
	switch match {
	case zoneAmbiguous:
		return invalidResult("The drop-off location matches more than one zone. Ask for a more specific district or zone."), nil
	case zoneNotFound:
		return invalidResult("The drop-off location is not in the serviceable zone directory."), nil
	}

	estimate, err := t.pricing.Estimate(ctx, client.EstimateInput{
		OriginZoneID:      originZone.ID,
		DestinationZoneID: destinationZone.ID,
		WeightKg:          weight,
		SizeCategory:      sizeCategory,
	})
	if err != nil {
		return pricingFailure(err), nil
	}

	return contract.ToolResult{
		Status: contract.StatusOK,
		Data: feeResult{
			Origin:        originZone.Name,
			Destination:   destinationZone.Name,
			WeightKg:      weight,
			SizeCategory:  sizeCategory,
			Currency:      estimate.Currency,
			BaseFee:       estimate.BaseFee,
			WeightCharge:  estimate.WeightCharge,
			SizeSurcharge: estimate.SizeSurcharge,
			Total:         estimate.Total,
		},
	}, nil
}

type zoneMatch int

const (
	zoneResolved zoneMatch = iota
	zoneNotFound
	zoneAmbiguous
)

// resolveZone matches a customer-supplied location case-insensitively against
// a zone name, zone code or any district. A location matching more than one
// zone is ambiguous and must be clarified rather than guessed.
func resolveZone(zones []client.Zone, query string) (client.Zone, zoneMatch) {
	query = strings.TrimSpace(query)

	var matches []client.Zone
	for _, zone := range zones {
		if zoneMatches(zone, query) {
			matches = append(matches, zone)
		}
	}

	switch len(matches) {
	case 0:
		return client.Zone{}, zoneNotFound
	case 1:
		return matches[0], zoneResolved
	default:
		return client.Zone{}, zoneAmbiguous
	}
}

func zoneMatches(zone client.Zone, query string) bool {
	if strings.EqualFold(strings.TrimSpace(zone.Name), query) {
		return true
	}
	if strings.EqualFold(strings.TrimSpace(zone.Code), query) {
		return true
	}
	for _, district := range zone.Districts {
		if strings.EqualFold(strings.TrimSpace(district), query) {
			return true
		}
	}
	return false
}
