package tools

import (
	"context"
	"encoding/json"

	"github.com/Vineth-R/swiftdrop/apps/services/ai-assistant/internal/contract"
	"github.com/Vineth-R/swiftdrop/packages/go-common/validation"
)

const (
	toolNameCourierDeliveries = "get_my_courier_deliveries"
	toolNameCourierDelivery   = "get_my_courier_delivery"
	maxCourierRows            = 20
)

// --- get_my_courier_deliveries ----------------------------------------------

type courierDeliveriesTool struct {
	parcel ParcelClient
}

func (t *courierDeliveriesTool) Definition() contract.ToolDeclaration {
	return contract.ToolDeclaration{
		Name: toolNameCourierDeliveries,
		Description: "List the signed-in courier's own deliveries. The courier identity comes from the verified session; " +
			"never accept or invent a courier ID. The result contains the courier's currently assigned active parcels plus " +
			"deliveries completed or failed on the selected day (default today in Asia/Colombo). Use the status filter for " +
			"pending, completed or failed questions. Only the first 20 records on the first page are returned.",
		Parameters: contract.Parameters{
			Properties: map[string]contract.Parameter{
				"status": {
					Type:        "string",
					Description: "Optional parcel status filter.",
					Enum:        parcelStatuses,
				},
				"date": {
					Type:        "string",
					Description: "Optional completion day in YYYY-MM-DD; defaults to today in Asia/Colombo.",
				},
			},
			Required: nil,
		},
	}
}

func (t *courierDeliveriesTool) AvailableTo(caller contract.CallerContext) bool {
	return hasVerifiedIdentity(caller) && isCourierRole(caller.Role)
}

type courierDeliveryRow struct {
	ID             string      `json:"id"`
	TrackingNumber string      `json:"trackingNumber"`
	Status         string      `json:"status"`
	DropoffCity    string      `json:"dropoffCity"`
	RecipientName  string      `json:"recipientName"`
	WeightKg       json.Number `json:"weightKg"`
	AssignedAt     string      `json:"assignedAt"`
	Active         bool        `json:"active"`
}

type courierDeliveriesResult struct {
	Scope      string               `json:"scope"`
	Date       string               `json:"date"`
	Deliveries []courierDeliveryRow `json:"deliveries"`
	Count      int                  `json:"count"`
	TotalItems int                  `json:"totalItems"`
	HasMore    bool                 `json:"hasMore"`
}

func (t *courierDeliveriesTool) Execute(ctx context.Context, args map[string]any, caller contract.CallerContext) (contract.ToolResult, error) {
	if denial, ok := requireCourierAccess(caller); !ok {
		return denial, nil
	}

	values, message := validateArguments(args, []argSpec{
		{name: "status", kind: argString, required: false, enum: parcelStatuses},
		{name: "date", kind: argString, required: false, date: true},
	})
	if message != "" {
		return invalidResult(message), nil
	}

	status, _ := values["status"].(string)
	date, _ := values["date"].(string)

	list, err := t.parcel.CourierDeliveries(ctx, status, date, caller.AccessToken)
	if err != nil {
		return privateFailure(err), nil
	}

	rows := make([]courierDeliveryRow, 0, min(len(list.Items), maxCourierRows))
	for _, item := range list.Items {
		if len(rows) >= maxCourierRows {
			break
		}
		rows = append(rows, courierDeliveryRow{
			ID:             item.ID,
			TrackingNumber: item.TrackingNumber,
			Status:         item.Status,
			DropoffCity:    item.DropoffCity,
			RecipientName:  item.RecipientName,
			WeightKg:       item.WeightKg,
			AssignedAt:     item.AssignedAt,
			Active:         isActiveStatus(item.Status),
		})
	}
	total := list.Pagination.TotalItems
	if total < len(rows) {
		total = len(rows)
	}

	dateScope := date
	if dateScope == "" {
		dateScope = "today (Asia/Colombo)"
	}

	return contract.ToolResult{
		Status: contract.StatusOK,
		Data: courierDeliveriesResult{
			Scope:      "courier",
			Date:       dateScope,
			Deliveries: rows,
			Count:      len(rows),
			TotalItems: total,
			HasMore:    total > len(rows),
		},
	}, nil
}

// --- get_my_courier_delivery ------------------------------------------------

type courierDeliveryTool struct {
	parcel ParcelClient
}

func (t *courierDeliveryTool) Definition() contract.ToolDeclaration {
	return contract.ToolDeclaration{
		Name: toolNameCourierDelivery,
		Description: "Read one delivery assigned to the signed-in courier, including status history and any failure reason. " +
			"The Parcel service verifies the current assignment from the verified session, so another courier's delivery is " +
			"never returned. Find the parcel ID from the courier delivery list first.",
		Parameters: contract.Parameters{
			Properties: map[string]contract.Parameter{
				"parcelId": {
					Type:        "string",
					Description: "The parcel UUID from the courier's own delivery list.",
				},
			},
			Required: []string{"parcelId"},
		},
	}
}

func (t *courierDeliveryTool) AvailableTo(caller contract.CallerContext) bool {
	return hasVerifiedIdentity(caller) && isCourierRole(caller.Role)
}

type courierHistoryRow struct {
	Status         string  `json:"status"`
	PreviousStatus *string `json:"previousStatus,omitempty"`
	Note           *string `json:"note,omitempty"`
	FailureReason  *string `json:"failureReason,omitempty"`
	ChangedAt      string  `json:"changedAt"`
}

type courierDeliveryResult struct {
	Scope          string              `json:"scope"`
	ID             string              `json:"id"`
	TrackingNumber string              `json:"trackingNumber"`
	Status         string              `json:"status"`
	DropoffCity    string              `json:"dropoffCity"`
	RecipientName  string              `json:"recipientName"`
	WeightKg       json.Number         `json:"weightKg"`
	SizeCategory   string              `json:"sizeCategory"`
	AssignedAt     string              `json:"assignedAt"`
	History        []courierHistoryRow `json:"history"`
}

func (t *courierDeliveryTool) Execute(ctx context.Context, args map[string]any, caller contract.CallerContext) (contract.ToolResult, error) {
	if denial, ok := requireCourierAccess(caller); !ok {
		return denial, nil
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

	detail, err := t.parcel.CourierDelivery(ctx, parcelID, caller.AccessToken)
	if err != nil {
		return privateFailure(err), nil
	}

	history := make([]courierHistoryRow, 0, len(detail.History))
	for _, item := range detail.History {
		history = append(history, courierHistoryRow{
			Status:         item.Status,
			PreviousStatus: item.PreviousStatus,
			Note:           item.Note,
			FailureReason:  item.FailureReason,
			ChangedAt:      item.ChangedAt,
		})
	}

	return contract.ToolResult{
		Status: contract.StatusOK,
		Data: courierDeliveryResult{
			Scope:          "courier",
			ID:             detail.ID,
			TrackingNumber: detail.TrackingNumber,
			Status:         detail.Status,
			DropoffCity:    detail.DropoffCity,
			RecipientName:  detail.RecipientName,
			WeightKg:       detail.WeightKg,
			SizeCategory:   detail.SizeCategory,
			AssignedAt:     detail.AssignedAt,
			History:        history,
		},
	}, nil
}
