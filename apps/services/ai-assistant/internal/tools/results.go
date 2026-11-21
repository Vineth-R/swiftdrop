package tools

import (
	"errors"

	"github.com/Vineth-R/swiftdrop/apps/services/ai-assistant/internal/client"
	"github.com/Vineth-R/swiftdrop/apps/services/ai-assistant/internal/contract"
)

func invalidResult(message string) contract.ToolResult {
	return contract.ToolResult{Status: contract.StatusInvalid, Message: message}
}

func deniedResult(message string) contract.ToolResult {
	return contract.ToolResult{Status: contract.StatusDenied, Message: message}
}

func unavailableResult(message string) contract.ToolResult {
	return contract.ToolResult{Status: contract.StatusUnavailable, Message: message}
}

// pricingFailure maps a Pricing client error to a safe tool result. An
// unserviceable route or unpriced weight is INVALID; every other failure is
// UNAVAILABLE. A price is never fabricated.
func pricingFailure(err error) contract.ToolResult {
	var upstream *client.Error
	if errors.As(err, &upstream) {
		switch upstream.Kind {
		case client.KindInvalid:
			switch upstream.Code {
			case "ROUTE_NOT_SERVICEABLE":
				return invalidResult("No delivery quote is available for that route.")
			case "WEIGHT_NOT_PRICED":
				return invalidResult("No delivery quote is available for that weight.")
			default:
				return invalidResult("A delivery quote is not available for those details.")
			}
		case client.KindNotFound:
			return invalidResult("No delivery quote is available for that route.")
		}
	}
	return unavailableResult("Pricing is temporarily unavailable. Please try again later.")
}

// trackingFailure maps a public tracking error to a safe tool result.
func trackingFailure(err error) contract.ToolResult {
	switch {
	case client.IsKind(err, client.KindNotFound):
		return invalidResult("That tracking number was not found.")
	case client.IsKind(err, client.KindInvalid):
		return invalidResult("That tracking number is not valid.")
	default:
		return unavailableResult("Parcel tracking is temporarily unavailable. Please try again later.")
	}
}

// privateFailure maps an authenticated Parcel read error to a safe result. The
// upstream 404 for another customer's parcel is intentionally indistinguishable
// from a missing parcel.
func privateFailure(err error) contract.ToolResult {
	switch {
	case client.IsKind(err, client.KindUnauthorized):
		return deniedResult("You do not have access to that parcel.")
	case client.IsKind(err, client.KindNotFound):
		return invalidResult("That parcel was not found for this account.")
	case client.IsKind(err, client.KindInvalid):
		return invalidResult("The parcel request was not valid.")
	default:
		return unavailableResult("Parcel details are temporarily unavailable. Please try again later.")
	}
}
