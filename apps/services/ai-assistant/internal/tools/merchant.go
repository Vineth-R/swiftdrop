package tools

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/Vineth-R/swiftdrop/apps/services/ai-assistant/internal/client"
	"github.com/Vineth-R/swiftdrop/apps/services/ai-assistant/internal/contract"
)

const (
	toolNameMerchantOrders            = "get_my_merchant_orders"
	toolNameMerchantWebhookDeliveries = "get_my_webhook_deliveries"
	maxMerchantOrders                 = 20
	merchantTrackingScanSize          = 100
	merchantTrackingScanLabel         = "most recent 100 orders"
)

// --- get_my_merchant_orders -------------------------------------------------

type merchantOrdersTool struct {
	merchant MerchantClient
}

func (t *merchantOrdersTool) Definition() contract.ToolDeclaration {
	return contract.ToolDeclaration{
		Name: toolNameMerchantOrders,
		Description: "List the signed-in merchant's own orders, or match one exact tracking number. The Merchant service resolves " +
			"the merchant from the verified session, so another merchant's orders are never returned. Use status for pending, " +
			"in-transit or failed questions. When trackingNumber is supplied, only the merchant's " + merchantTrackingScanLabel +
			" are checked and an exact match is required; if there is no match, say so and ask for an order ID or external order ID.",
		Parameters: contract.Parameters{
			Properties: map[string]contract.Parameter{
				"status": {
					Type:        "string",
					Description: "Optional order status filter.",
					Enum:        parcelStatuses,
				},
				"externalOrderId": {
					Type:        "string",
					Description: "Optional exact external order reference.",
				},
				"trackingNumber": {
					Type:        "string",
					Description: "Optional exact SwiftDrop tracking number, for example SDN4T8W2LC6.",
				},
			},
			Required: nil,
		},
	}
}

func (t *merchantOrdersTool) AvailableTo(caller contract.CallerContext) bool {
	return hasVerifiedIdentity(caller) && isMerchantRole(caller.Role)
}

type merchantOrderRow struct {
	OrderID         string      `json:"orderId"`
	ExternalOrderID string      `json:"externalOrderId"`
	TrackingNumber  string      `json:"trackingNumber"`
	Status          string      `json:"status"`
	RecipientName   string      `json:"recipientName"`
	DropoffCity     string      `json:"dropoffCity"`
	Total           json.Number `json:"total"`
	Currency        string      `json:"currency"`
	CreatedAt       string      `json:"createdAt"`
	UpdatedAt       string      `json:"updatedAt"`
	Active          bool        `json:"active"`
}

type merchantOrdersResult struct {
	Scope        string             `json:"scope"`
	Orders       []merchantOrderRow `json:"orders"`
	Count        int                `json:"count"`
	TotalItems   int                `json:"totalItems"`
	HasMore      bool               `json:"hasMore"`
	SearchWindow string             `json:"searchWindow,omitempty"`
	MatchedBy    string             `json:"matchedBy,omitempty"`
}

func (t *merchantOrdersTool) Execute(ctx context.Context, args map[string]any, caller contract.CallerContext) (contract.ToolResult, error) {
	if denial, ok := requireMerchantAccess(caller); !ok {
		return denial, nil
	}

	values, message := validateArguments(args, []argSpec{
		{name: "status", kind: argString, required: false, enum: parcelStatuses},
		{name: "externalOrderId", kind: argString, required: false, maxLen: 100},
		{name: "trackingNumber", kind: argString, required: false, maxLen: 20, pattern: trackingNumberPattern},
	})
	if message != "" {
		return invalidResult(message), nil
	}

	filter := client.MerchantOrderFilter{}
	if value, ok := values["status"].(string); ok {
		filter.Status = value
	}
	if value, ok := values["externalOrderId"].(string); ok {
		filter.ExternalOrderID = value
	}
	trackingNumber, hasTracking := values["trackingNumber"].(string)

	if !hasTracking {
		filter.PageSize = maxMerchantOrders
		list, err := t.merchant.Orders(ctx, filter, caller.AccessToken)
		if err != nil {
			return merchantFailure(err), nil
		}
		rows := make([]merchantOrderRow, 0, min(len(list.Items), maxMerchantOrders))
		for _, item := range list.Items {
			if len(rows) >= maxMerchantOrders {
				break
			}
			rows = append(rows, merchantOrderRowOf(item))
		}
		total := list.Pagination.TotalItems
		if total < len(rows) {
			total = len(rows)
		}
		return contract.ToolResult{
			Status: contract.StatusOK,
			Data: merchantOrdersResult{
				Scope:      "merchant",
				Orders:     rows,
				Count:      len(rows),
				TotalItems: total,
				HasMore:    total > len(rows),
			},
		}, nil
	}

	// The merchant API has no tracking-number filter. Check only the recent
	// page and require an exact match so the assistant never implies a full
	// historical search.
	filter.PageSize = merchantTrackingScanSize
	list, err := t.merchant.Orders(ctx, filter, caller.AccessToken)
	if err != nil {
		return merchantFailure(err), nil
	}
	for _, item := range list.Items {
		if strings.EqualFold(strings.TrimSpace(item.TrackingNumber), trackingNumber) {
			return contract.ToolResult{
				Status: contract.StatusOK,
				Data: merchantOrdersResult{
					Scope:        "merchant",
					Orders:       []merchantOrderRow{merchantOrderRowOf(item)},
					Count:        1,
					TotalItems:   1,
					SearchWindow: merchantTrackingScanLabel,
					MatchedBy:    "trackingNumber",
				},
			}, nil
		}
	}

	return invalidResult("No order with that exact tracking number was found in your " + merchantTrackingScanLabel +
		". If you want a specific order, provide its order ID or external order ID."), nil
}

func merchantOrderRowOf(item client.MerchantOrderItem) merchantOrderRow {
	return merchantOrderRow{
		OrderID:         item.OrderID,
		ExternalOrderID: item.ExternalOrderID,
		TrackingNumber:  item.TrackingNumber,
		Status:          item.Status,
		RecipientName:   item.RecipientName,
		DropoffCity:     item.DropoffCity,
		Total:           item.Total,
		Currency:        item.Currency,
		CreatedAt:       item.CreatedAt,
		UpdatedAt:       item.UpdatedAt,
		Active:          isActiveStatus(item.Status),
	}
}

// --- get_my_webhook_deliveries ----------------------------------------------

type merchantWebhookDeliveriesTool struct {
	merchant MerchantClient
}

func (t *merchantWebhookDeliveriesTool) Definition() contract.ToolDeclaration {
	return contract.ToolDeclaration{
		Name: toolNameMerchantWebhookDeliveries,
		Description: "List the signed-in merchant's own webhook delivery attempts, including HTTP status, retry timing and " +
			"dead-letter state. The Merchant service scopes the result to the verified merchant. Never expose or ask for the " +
			"webhook signing secret. Use success=false to find failed deliveries.",
		Parameters: contract.Parameters{
			Properties: map[string]contract.Parameter{
				"success": {
					Type:        "boolean",
					Description: "Optional filter: true for successful attempts, false for failed attempts.",
				},
			},
			Required: nil,
		},
	}
}

func (t *merchantWebhookDeliveriesTool) AvailableTo(caller contract.CallerContext) bool {
	return hasVerifiedIdentity(caller) && isMerchantRole(caller.Role)
}

type webhookDeliveryRow struct {
	EventID        string  `json:"eventId"`
	TrackingNumber string  `json:"trackingNumber"`
	AttemptNumber  int     `json:"attemptNumber"`
	HTTPStatus     *int    `json:"httpStatus,omitempty"`
	Success        bool    `json:"success"`
	NextRetryAt    *string `json:"nextRetryAt,omitempty"`
	DeadLettered   bool    `json:"deadLettered"`
	CreatedAt      string  `json:"createdAt"`
}

type webhookDeliveriesResult struct {
	Scope      string               `json:"scope"`
	Deliveries []webhookDeliveryRow `json:"deliveries"`
	Count      int                  `json:"count"`
	TotalItems int                  `json:"totalItems"`
	HasMore    bool                 `json:"hasMore"`
}

func (t *merchantWebhookDeliveriesTool) Execute(ctx context.Context, args map[string]any, caller contract.CallerContext) (contract.ToolResult, error) {
	if denial, ok := requireMerchantAccess(caller); !ok {
		return denial, nil
	}

	values, message := validateArguments(args, []argSpec{
		{name: "success", kind: argBool, required: false},
	})
	if message != "" {
		return invalidResult(message), nil
	}

	var success *bool
	if value, ok := values["success"].(bool); ok {
		success = &value
	}

	list, err := t.merchant.WebhookDeliveries(ctx, success, caller.AccessToken)
	if err != nil {
		if client.IsKind(err, client.KindNotFound) && upstreamCode(err) == "WEBHOOK_NOT_CONFIGURED" {
			return invalidResult("No webhook is configured for this merchant, so there are no delivery attempts to show."), nil
		}
		return merchantFailure(err), nil
	}

	rows := make([]webhookDeliveryRow, 0, min(len(list.Items), maxMerchantOrders))
	for _, item := range list.Items {
		if len(rows) >= maxMerchantOrders {
			break
		}
		rows = append(rows, webhookDeliveryRow{
			EventID:        item.EventID,
			TrackingNumber: item.TrackingNumber,
			AttemptNumber:  item.AttemptNumber,
			HTTPStatus:     item.HTTPStatus,
			Success:        item.Success,
			NextRetryAt:    item.NextRetryAt,
			DeadLettered:   item.DeadLettered,
			CreatedAt:      item.CreatedAt,
		})
	}
	total := list.Pagination.TotalItems
	if total < len(rows) {
		total = len(rows)
	}
	return contract.ToolResult{
		Status: contract.StatusOK,
		Data: webhookDeliveriesResult{
			Scope:      "merchant",
			Deliveries: rows,
			Count:      len(rows),
			TotalItems: total,
			HasMore:    total > len(rows),
		},
	}, nil
}

func upstreamCode(err error) string {
	var upstream *client.Error
	if errors.As(err, &upstream) {
		return upstream.Code
	}
	return ""
}

// merchantFailure maps a merchant-owned read failure to a safe result.
func merchantFailure(err error) contract.ToolResult {
	switch {
	case client.IsKind(err, client.KindUnauthorized):
		return deniedResult("This information is not available for your merchant account.")
	case client.IsKind(err, client.KindNotFound):
		return invalidResult("That record was not found for your merchant account.")
	case client.IsKind(err, client.KindInvalid):
		return invalidResult("The merchant order request was not valid.")
	default:
		return unavailableResult("Merchant order information is temporarily unavailable. Please try again later.")
	}
}
