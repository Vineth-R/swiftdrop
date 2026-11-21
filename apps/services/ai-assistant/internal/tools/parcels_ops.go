package tools

import (
	"context"
	"encoding/json"
	"regexp"
	"sort"

	"github.com/Vineth-R/swiftdrop/apps/services/ai-assistant/internal/client"
	"github.com/Vineth-R/swiftdrop/apps/services/ai-assistant/internal/contract"
	"github.com/Vineth-R/swiftdrop/packages/go-common/validation"
)

const (
	toolNameParcelDetails      = "get_parcel_details"
	toolNameSearchParcels      = "search_parcels"
	toolNameUnassignedParcels  = "list_unassigned_parcels"
	toolNameCourierWorkload    = "get_courier_workload"
	maxOperationalRows         = 20
	maxWorkloadRows            = 50
	operationalPageDescription = "Only the first page of at most 20 matching records is returned."
)

var trackingSearchPattern = regexp.MustCompile(`^[A-Za-z0-9]{1,32}$`)

// --- get_parcel_details -----------------------------------------------------

type parcelDetailsTool struct {
	parcel ParcelClient
}

func (t *parcelDetailsTool) Definition() contract.ToolDeclaration {
	return contract.ToolDeclaration{
		Name: toolNameParcelDetails,
		Description: "Read one parcel's detail by parcel ID, including its current status, route cities, fee, assigned courier name " +
			"and failure reason when the parcel failed. Customers may read only their own parcels; dispatchers and administrators " +
			"may read parcels operationally. Find the parcel ID from an authorized parcel list first. Never accept a user, customer " +
			"or owner ID.",
		Parameters: contract.Parameters{
			Properties: map[string]contract.Parameter{
				"parcelId": {
					Type:        "string",
					Description: "The parcel UUID from an authorized parcel list.",
				},
			},
			Required: []string{"parcelId"},
		},
	}
}

func (t *parcelDetailsTool) AvailableTo(caller contract.CallerContext) bool {
	return hasVerifiedIdentity(caller) &&
		(isCustomerRole(caller.Role) || isStaffRole(caller.Role))
}

type parcelDetailResult struct {
	ID             string      `json:"id"`
	TrackingNumber string      `json:"trackingNumber"`
	Status         string      `json:"status"`
	PickupCity     string      `json:"pickupCity"`
	DropoffCity    string      `json:"dropoffCity"`
	WeightKg       json.Number `json:"weightKg"`
	SizeCategory   string      `json:"sizeCategory"`
	Total          json.Number `json:"total"`
	Currency       string      `json:"currency"`
	CourierName    *string     `json:"courierName,omitempty"`
	FailureReason  *string     `json:"failureReason,omitempty"`
	CreatedAt      string      `json:"createdAt"`
	UpdatedAt      string      `json:"updatedAt"`
	DeliveredAt    *string     `json:"deliveredAt,omitempty"`
}

func (t *parcelDetailsTool) Execute(ctx context.Context, args map[string]any, caller contract.CallerContext) (contract.ToolResult, error) {
	if denial, ok := requireParcelAccess(caller, "Please sign in to view parcel details."); !ok {
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

	detail, err := t.parcel.Detail(ctx, parcelID, caller.AccessToken)
	if err != nil {
		return privateFailure(err), nil
	}

	return contract.ToolResult{
		Status: contract.StatusOK,
		Data: parcelDetailResult{
			ID:             detail.ID,
			TrackingNumber: detail.TrackingNumber,
			Status:         detail.Status,
			PickupCity:     detail.PickupCity,
			DropoffCity:    detail.DropoffCity,
			WeightKg:       detail.WeightKg,
			SizeCategory:   detail.SizeCategory,
			Total:          detail.Total,
			Currency:       detail.Currency,
			CourierName:    detail.CourierName,
			FailureReason:  detail.FailureReason,
			CreatedAt:      detail.CreatedAt,
			UpdatedAt:      detail.UpdatedAt,
			DeliveredAt:    detail.DeliveredAt,
		},
	}, nil
}

// --- search_parcels ---------------------------------------------------------

type searchParcelsTool struct {
	parcel ParcelClient
}

func (t *searchParcelsTool) Definition() contract.ToolDeclaration {
	return contract.ToolDeclaration{
		Name: toolNameSearchParcels,
		Description: "Search operational parcels across customers by status, tracking number or creation date range. " +
			"Dispatchers and administrators only. " + operationalPageDescription +
			" Use it for questions such as which deliveries failed or which parcels match a tracking number.",
		Parameters: contract.Parameters{
			Properties: map[string]contract.Parameter{
				"status": {
					Type:        "string",
					Description: "Optional parcel status filter.",
					Enum:        parcelStatuses,
				},
				"trackingNumber": {
					Type:        "string",
					Description: "Optional tracking number or prefix, for example SDK7M2Q9XP4.",
				},
				"from": {
					Type:        "string",
					Description: "Optional inclusive creation start date in YYYY-MM-DD (UTC).",
				},
				"to": {
					Type:        "string",
					Description: "Optional inclusive creation end date in YYYY-MM-DD (UTC).",
				},
			},
			Required: nil,
		},
	}
}

func (t *searchParcelsTool) AvailableTo(caller contract.CallerContext) bool {
	return hasVerifiedIdentity(caller) && isStaffRole(caller.Role)
}

type operationalParcelRow struct {
	ID             string  `json:"id"`
	TrackingNumber string  `json:"trackingNumber"`
	Status         string  `json:"status"`
	PickupCity     string  `json:"pickupCity"`
	DropoffCity    string  `json:"dropoffCity"`
	CourierID      *string `json:"courierId,omitempty"`
	CreatedAt      string  `json:"createdAt"`
	Active         bool    `json:"active"`
}

type operationalParcelResult struct {
	Scope       string                 `json:"scope"`
	Parcels     []operationalParcelRow `json:"parcels"`
	Count       int                    `json:"count"`
	TotalItems  int                    `json:"totalItems"`
	HasMore     bool                   `json:"hasMore"`
	PeriodNote  string                 `json:"periodNote"`
	StatusNote  string                 `json:"statusNote,omitempty"`
	TrackingKey string                 `json:"trackingFilter,omitempty"`
}

func (t *searchParcelsTool) Execute(ctx context.Context, args map[string]any, caller contract.CallerContext) (contract.ToolResult, error) {
	if denial, ok := requireStaffAccess(caller); !ok {
		return denial, nil
	}

	values, message := validateArguments(args, []argSpec{
		{name: "status", kind: argString, required: false, enum: parcelStatuses},
		{name: "trackingNumber", kind: argString, required: false, maxLen: 32, pattern: trackingSearchPattern},
		{name: "from", kind: argString, required: false, date: true},
		{name: "to", kind: argString, required: false, date: true},
	})
	if message != "" {
		return invalidResult(message), nil
	}

	filter := client.AdminParcelFilter{}
	if value, ok := values["status"].(string); ok {
		filter.Status = value
	}
	if value, ok := values["trackingNumber"].(string); ok {
		filter.TrackingNumber = value
	}
	if value, ok := values["from"].(string); ok {
		filter.FromDate = value
	}
	if value, ok := values["to"].(string); ok {
		filter.ToDate = value
	}
	if filter.FromDate != "" && filter.ToDate != "" && filter.FromDate > filter.ToDate {
		return invalidResult("from must be on or before to."), nil
	}

	list, err := t.parcel.AdminParcels(ctx, filter, caller.AccessToken)
	if err != nil {
		return privateFailure(err), nil
	}

	rows := make([]operationalParcelRow, 0, min(len(list.Items), maxOperationalRows))
	for _, item := range list.Items {
		if len(rows) >= maxOperationalRows {
			break
		}
		rows = append(rows, operationalParcelRow{
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
	total := list.Pagination.TotalItems
	if total < len(rows) {
		total = len(rows)
	}

	period := "All creation dates."
	if filter.FromDate != "" || filter.ToDate != "" {
		period = "Creation dates " + dateRangeLabel(filter.FromDate, filter.ToDate) + " (UTC)."
	}

	return contract.ToolResult{
		Status: contract.StatusOK,
		Data: operationalParcelResult{
			Scope:       "operations",
			Parcels:     rows,
			Count:       len(rows),
			TotalItems:  total,
			HasMore:     total > len(rows),
			PeriodNote:  period,
			StatusNote:  filter.Status,
			TrackingKey: filter.TrackingNumber,
		},
	}, nil
}

// --- list_unassigned_parcels ------------------------------------------------

type unassignedParcelsTool struct {
	parcel ParcelClient
}

func (t *unassignedParcelsTool) Definition() contract.ToolDeclaration {
	return contract.ToolDeclaration{
		Name: toolNameUnassignedParcels,
		Description: "List parcels that are waiting for courier assignment. Dispatchers and administrators only. " +
			"Read-only: never assign, reassign or claim a parcel. " + operationalPageDescription,
		Parameters: contract.Parameters{},
	}
}

func (t *unassignedParcelsTool) AvailableTo(caller contract.CallerContext) bool {
	return hasVerifiedIdentity(caller) && isStaffRole(caller.Role)
}

type unassignedParcelRow struct {
	ID             string      `json:"id"`
	TrackingNumber string      `json:"trackingNumber"`
	Status         string      `json:"status"`
	PickupCity     string      `json:"pickupCity"`
	DropoffCity    string      `json:"dropoffCity"`
	WeightKg       json.Number `json:"weightKg"`
	SizeCategory   string      `json:"sizeCategory"`
	CreatedAt      string      `json:"createdAt"`
}

type unassignedParcelResult struct {
	Scope      string                `json:"scope"`
	Parcels    []unassignedParcelRow `json:"parcels"`
	Count      int                   `json:"count"`
	TotalItems int                   `json:"totalItems"`
	HasMore    bool                  `json:"hasMore"`
}

func (t *unassignedParcelsTool) Execute(ctx context.Context, args map[string]any, caller contract.CallerContext) (contract.ToolResult, error) {
	if denial, ok := requireStaffAccess(caller); !ok {
		return denial, nil
	}
	if _, message := validateArguments(args, nil); message != "" {
		return invalidResult(message), nil
	}

	list, err := t.parcel.Unassigned(ctx, caller.AccessToken)
	if err != nil {
		return privateFailure(err), nil
	}

	rows := make([]unassignedParcelRow, 0, min(len(list.Items), maxOperationalRows))
	for _, item := range list.Items {
		if len(rows) >= maxOperationalRows {
			break
		}
		rows = append(rows, unassignedParcelRow{
			ID:             item.ID,
			TrackingNumber: item.TrackingNumber,
			Status:         item.Status,
			PickupCity:     item.PickupCity,
			DropoffCity:    item.DropoffCity,
			WeightKg:       item.WeightKg,
			SizeCategory:   item.SizeCategory,
			CreatedAt:      item.CreatedAt,
		})
	}
	total := list.Pagination.TotalItems
	if total < len(rows) {
		total = len(rows)
	}
	return contract.ToolResult{
		Status: contract.StatusOK,
		Data: unassignedParcelResult{
			Scope:      "dispatch",
			Parcels:    rows,
			Count:      len(rows),
			TotalItems: total,
			HasMore:    total > len(rows),
		},
	}, nil
}

// --- get_courier_workload ---------------------------------------------------

type courierWorkloadTool struct {
	parcel ParcelClient
}

func (t *courierWorkloadTool) Definition() contract.ToolDeclaration {
	return contract.ToolDeclaration{
		Name: toolNameCourierWorkload,
		Description: "Read active courier workload, availability and per-status delivery counts. Dispatchers and administrators only. " +
			"Report only the returned values; never estimate or invent availability or workload. Highest workload is the largest " +
			"returned activeDeliveries value.",
		Parameters: contract.Parameters{},
	}
}

func (t *courierWorkloadTool) AvailableTo(caller contract.CallerContext) bool {
	return hasVerifiedIdentity(caller) && isStaffRole(caller.Role)
}

type courierWorkloadRow struct {
	CourierID        string         `json:"courierId"`
	FullName         string         `json:"fullName"`
	IsAvailable      bool           `json:"isAvailable"`
	ActiveDeliveries int            `json:"activeDeliveries"`
	ByStatus         map[string]int `json:"byStatus"`
}

type courierWorkloadResult struct {
	Scope    string               `json:"scope"`
	Couriers []courierWorkloadRow `json:"couriers"`
	Count    int                  `json:"count"`
}

func (t *courierWorkloadTool) Execute(ctx context.Context, args map[string]any, caller contract.CallerContext) (contract.ToolResult, error) {
	if denial, ok := requireStaffAccess(caller); !ok {
		return denial, nil
	}
	if _, message := validateArguments(args, nil); message != "" {
		return invalidResult(message), nil
	}

	items, err := t.parcel.Workload(ctx, caller.AccessToken)
	if err != nil {
		return privateFailure(err), nil
	}

	rows := make([]courierWorkloadRow, 0, min(len(items), maxWorkloadRows))
	for _, item := range items {
		rows = append(rows, courierWorkloadRow{
			CourierID:        item.CourierID,
			FullName:         item.FullName,
			IsAvailable:      item.IsAvailable,
			ActiveDeliveries: item.ActiveDeliveries,
			ByStatus:         item.ByStatus,
		})
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].ActiveDeliveries != rows[j].ActiveDeliveries {
			return rows[i].ActiveDeliveries > rows[j].ActiveDeliveries
		}
		return rows[i].FullName < rows[j].FullName
	})
	if len(rows) > maxWorkloadRows {
		rows = rows[:maxWorkloadRows]
	}

	return contract.ToolResult{
		Status: contract.StatusOK,
		Data: courierWorkloadResult{
			Scope:    "operations",
			Couriers: rows,
			Count:    len(rows),
		},
	}, nil
}

func dateRangeLabel(from, to string) string {
	switch {
	case from != "" && to != "":
		return "from " + from + " to " + to
	case from != "":
		return "from " + from
	case to != "":
		return "up to " + to
	default:
		return "all dates"
	}
}
