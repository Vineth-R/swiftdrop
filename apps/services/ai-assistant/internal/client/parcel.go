package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
)

// TrackingEvent is one public timeline entry.
type TrackingEvent struct {
	Status string `json:"status"`
	At     string `json:"at"`
}

// Tracking is the public parcel snapshot. It models only the published
// allowlist so private upstream fields cannot reach the caller.
type Tracking struct {
	TrackingNumber string          `json:"trackingNumber"`
	Status         string          `json:"status"`
	PickupCity     string          `json:"pickupCity"`
	DropoffCity    string          `json:"dropoffCity"`
	LastUpdatedAt  string          `json:"lastUpdatedAt"`
	Timeline       []TrackingEvent `json:"timeline"`
}

// HistoryItem is one owned-parcel history row. failureReason and actorRole are
// intentionally not modelled so they cannot leak to the model.
type HistoryItem struct {
	PreviousStatus *string `json:"previousStatus"`
	Status         string  `json:"status"`
	ChangedAt      string  `json:"changedAt"`
	Note           *string `json:"note"`
}

// ParcelListItem is one owned parcel summary entry.
type ParcelListItem struct {
	ID             string      `json:"id"`
	TrackingNumber string      `json:"trackingNumber"`
	Status         string      `json:"status"`
	RecipientName  string      `json:"recipientName"`
	DropoffCity    string      `json:"dropoffCity"`
	Total          json.Number `json:"total"`
	Currency       string      `json:"currency"`
	CreatedAt      string      `json:"createdAt"`
}

// Pagination is the upstream list metadata.
type Pagination struct {
	Page       int `json:"page"`
	PageSize   int `json:"pageSize"`
	TotalItems int `json:"totalItems"`
	TotalPages int `json:"totalPages"`
}

// ParcelList is the owned-parcel list response.
type ParcelList struct {
	Items      []ParcelListItem
	Pagination Pagination
}

// AdminParcelListItem is one operational parcel row. Customer and merchant IDs
// are deliberately not modelled so they cannot reach the model.
type AdminParcelListItem struct {
	ID             string  `json:"id"`
	TrackingNumber string  `json:"trackingNumber"`
	Status         string  `json:"status"`
	PickupCity     string  `json:"pickupCity"`
	DropoffCity    string  `json:"dropoffCity"`
	CourierID      *string `json:"courierId"`
	CreatedAt      string  `json:"createdAt"`
}

// AdminParcelList is the operational parcel search response.
type AdminParcelList struct {
	Items      []AdminParcelListItem
	Pagination Pagination
}

// AdminParcelFilter is the approved operational parcel search filter. Dates are
// calendar dates in UTC; the client turns them into the RFC3339 bounds the
// Parcel API expects.
type AdminParcelFilter struct {
	Status         string
	TrackingNumber string
	FromDate       string
	ToDate         string
}

// ParcelDetail is the authorized parcel detail allowlist. Sender and recipient
// contacts, addresses, phone numbers and proof data are deliberately not
// modelled so they cannot reach the model.
type ParcelDetail struct {
	ID             string      `json:"id"`
	TrackingNumber string      `json:"trackingNumber"`
	Status         string      `json:"status"`
	PickupCity     string      `json:"pickupCity"`
	DropoffCity    string      `json:"dropoffCity"`
	WeightKg       json.Number `json:"weightKg"`
	SizeCategory   string      `json:"sizeCategory"`
	Total          json.Number `json:"total"`
	Currency       string      `json:"currency"`
	CourierName    *string     `json:"courierName"`
	FailureReason  *string     `json:"failureReason"`
	CreatedAt      string      `json:"createdAt"`
	UpdatedAt      string      `json:"updatedAt"`
	DeliveredAt    *string     `json:"deliveredAt"`
}

// UnassignedParcel is one dispatch-planning row. Street addresses and zone IDs
// stay out of the model's view.
type UnassignedParcel struct {
	ID             string      `json:"id"`
	TrackingNumber string      `json:"trackingNumber"`
	Status         string      `json:"status"`
	PickupCity     string      `json:"pickupCity"`
	DropoffCity    string      `json:"dropoffCity"`
	WeightKg       json.Number `json:"weightKg"`
	SizeCategory   string      `json:"sizeCategory"`
	CreatedAt      string      `json:"createdAt"`
}

// UnassignedList is the paginated unassigned-parcel response.
type UnassignedList struct {
	Items      []UnassignedParcel
	Pagination Pagination
}

// CourierWorkload is one courier's operational workload. Phone and zone are not
// modelled.
type CourierWorkload struct {
	CourierID        string         `json:"courierId"`
	FullName         string         `json:"fullName"`
	IsAvailable      bool           `json:"isAvailable"`
	ActiveDeliveries int            `json:"activeDeliveries"`
	ByStatus         map[string]int `json:"byStatus"`
}

// CourierDeliveryItem is one courier delivery row. Addresses, phone numbers and
// proof data are not modelled.
type CourierDeliveryItem struct {
	ID             string      `json:"id"`
	TrackingNumber string      `json:"trackingNumber"`
	Status         string      `json:"status"`
	DropoffCity    string      `json:"dropoffCity"`
	RecipientName  string      `json:"recipientName"`
	WeightKg       json.Number `json:"weightKg"`
	AssignedAt     string      `json:"assignedAt"`
}

// CourierDeliveryList is the paginated courier delivery response.
type CourierDeliveryList struct {
	Items      []CourierDeliveryItem
	Pagination Pagination
}

// CourierHistoryItem is one owned-delivery history row.
type CourierHistoryItem struct {
	PreviousStatus *string `json:"previousStatus"`
	Status         string  `json:"status"`
	Note           *string `json:"note"`
	FailureReason  *string `json:"failureReason"`
	ChangedAt      string  `json:"changedAt"`
}

// CourierDeliveryDetail is the courier-owned delivery detail allowlist. Sender
// and recipient contacts, proof paths and internal metadata are excluded.
type CourierDeliveryDetail struct {
	ID             string               `json:"id"`
	TrackingNumber string               `json:"trackingNumber"`
	Status         string               `json:"status"`
	DropoffCity    string               `json:"dropoffCity"`
	RecipientName  string               `json:"recipientName"`
	WeightKg       json.Number          `json:"weightKg"`
	SizeCategory   string               `json:"sizeCategory"`
	AssignedAt     string               `json:"assignedAt"`
	History        []CourierHistoryItem `json:"history"`
}

// ParcelClient calls the Parcel service's public tracking and owned parcel
// read endpoints.
type ParcelClient struct {
	http *httpClient
}

// NewParcelClient validates cfg and returns a Parcel client.
func NewParcelClient(cfg Config) (*ParcelClient, error) {
	httpClient, err := newHTTPClient(cfg)
	if err != nil {
		return nil, err
	}
	return &ParcelClient{http: httpClient}, nil
}

// Tracking returns the caller's authorized snapshot for one tracking number.
// The Parcel service scopes CUSTOMER lookups to their own parcels and allows
// DISPATCHER/ADMIN operational access; the assistant never calls the anonymous
// public tracking projection.
func (c *ParcelClient) Tracking(ctx context.Context, trackingNumber, accessToken string) (Tracking, error) {
	number := strings.TrimSpace(trackingNumber)
	if number == "" {
		return Tracking{}, &Error{Kind: KindInvalid}
	}

	var envelope struct {
		Data Tracking `json:"data"`
	}
	path := "/parcels/tracking/" + url.PathEscape(number)
	if err := c.http.do(ctx, http.MethodGet, path, nil, nil, strings.TrimSpace(accessToken), &envelope); err != nil {
		return Tracking{}, err
	}
	return envelope.Data, nil
}

// History returns the caller's owned parcel history. accessToken is forwarded
// only when non-empty.
func (c *ParcelClient) History(ctx context.Context, parcelID, accessToken string) ([]HistoryItem, error) {
	id := strings.TrimSpace(parcelID)
	if id == "" {
		return nil, &Error{Kind: KindInvalid}
	}

	var envelope struct {
		Data []HistoryItem `json:"data"`
	}
	path := "/parcels/" + url.PathEscape(id) + "/history"
	if err := c.http.do(ctx, http.MethodGet, path, nil, nil, strings.TrimSpace(accessToken), &envelope); err != nil {
		return nil, err
	}
	return envelope.Data, nil
}

// List returns the first page of the caller's owned parcels. A blank status
// omits the filter.
func (c *ParcelClient) List(ctx context.Context, status, accessToken string) (ParcelList, error) {
	query := url.Values{}
	query.Set("page", "1")
	query.Set("pageSize", "20")
	if trimmed := strings.TrimSpace(status); trimmed != "" {
		query.Set("status", trimmed)
	}

	var envelope struct {
		Data []ParcelListItem `json:"data"`
		Meta struct {
			Pagination Pagination `json:"pagination"`
		} `json:"meta"`
	}
	if err := c.http.do(ctx, http.MethodGet, "/parcels", query, nil, strings.TrimSpace(accessToken), &envelope); err != nil {
		return ParcelList{}, err
	}
	for _, item := range envelope.Data {
		if !validNumber(item.Total) {
			return ParcelList{}, &Error{Kind: KindUnavailable}
		}
	}
	return ParcelList{Items: envelope.Data, Pagination: envelope.Meta.Pagination}, nil
}

// AdminParcels returns the first page of the operational parcel search. The
// Parcel API authorizes DISPATCHER and ADMIN callers; the assistant never
// supplies an identity.
func (c *ParcelClient) AdminParcels(ctx context.Context, filter AdminParcelFilter, accessToken string) (AdminParcelList, error) {
	query := url.Values{}
	query.Set("page", "1")
	query.Set("pageSize", "20")
	query.Set("sort", "createdAt:desc")
	if value := strings.TrimSpace(filter.Status); value != "" {
		query.Set("status", value)
	}
	if value := strings.TrimSpace(filter.TrackingNumber); value != "" {
		query.Set("trackingNumber", value)
	}
	if value := strings.TrimSpace(filter.FromDate); value != "" {
		query.Set("from", value+"T00:00:00Z")
	}
	if value := strings.TrimSpace(filter.ToDate); value != "" {
		query.Set("to", value+"T23:59:59Z")
	}

	var envelope struct {
		Data []AdminParcelListItem `json:"data"`
		Meta struct {
			Pagination Pagination `json:"pagination"`
		} `json:"meta"`
	}
	if err := c.http.do(ctx, http.MethodGet, "/admin/parcels", query, nil, strings.TrimSpace(accessToken), &envelope); err != nil {
		return AdminParcelList{}, err
	}
	return AdminParcelList{Items: envelope.Data, Pagination: envelope.Meta.Pagination}, nil
}

// Detail returns one authorized parcel detail. The upstream payload carries
// contacts and courier contact data; only the allowlisted fields below are
// modelled, so private fields cannot reach the caller.
func (c *ParcelClient) Detail(ctx context.Context, parcelID, accessToken string) (ParcelDetail, error) {
	id := strings.TrimSpace(parcelID)
	if id == "" {
		return ParcelDetail{}, &Error{Kind: KindInvalid}
	}

	var envelope struct {
		Data struct {
			ID             string `json:"id"`
			TrackingNumber string `json:"trackingNumber"`
			Status         string `json:"status"`
			Sender         struct {
				City string `json:"city"`
			} `json:"sender"`
			Recipient struct {
				City string `json:"city"`
			} `json:"recipient"`
			WeightKG     json.Number `json:"weightKg"`
			SizeCategory string      `json:"sizeCategory"`
			Fee          struct {
				Currency string      `json:"currency"`
				Total    json.Number `json:"total"`
			} `json:"fee"`
			Courier *struct {
				FullName string `json:"fullName"`
			} `json:"courier"`
			FailureReason *string `json:"failureReason"`
			CreatedAt     string  `json:"createdAt"`
			UpdatedAt     string  `json:"updatedAt"`
			DeliveredAt   *string `json:"deliveredAt"`
		} `json:"data"`
	}
	path := "/parcels/" + url.PathEscape(id)
	if err := c.http.do(ctx, http.MethodGet, path, nil, nil, strings.TrimSpace(accessToken), &envelope); err != nil {
		return ParcelDetail{}, err
	}
	if !validNumber(envelope.Data.WeightKG) || !validNumber(envelope.Data.Fee.Total) {
		return ParcelDetail{}, &Error{Kind: KindUnavailable}
	}

	detail := ParcelDetail{
		ID:             envelope.Data.ID,
		TrackingNumber: envelope.Data.TrackingNumber,
		Status:         envelope.Data.Status,
		PickupCity:     envelope.Data.Sender.City,
		DropoffCity:    envelope.Data.Recipient.City,
		WeightKg:       envelope.Data.WeightKG,
		SizeCategory:   envelope.Data.SizeCategory,
		Total:          envelope.Data.Fee.Total,
		Currency:       envelope.Data.Fee.Currency,
		FailureReason:  cleanOptional(envelope.Data.FailureReason),
		CreatedAt:      envelope.Data.CreatedAt,
		UpdatedAt:      envelope.Data.UpdatedAt,
		DeliveredAt:    cleanOptional(envelope.Data.DeliveredAt),
	}
	if envelope.Data.Courier != nil {
		name := strings.TrimSpace(envelope.Data.Courier.FullName)
		if name != "" {
			detail.CourierName = &name
		}
	}
	return detail, nil
}

// Unassigned returns the first page of parcels waiting for a courier. The
// Parcel API authorizes DISPATCHER and ADMIN callers.
func (c *ParcelClient) Unassigned(ctx context.Context, accessToken string) (UnassignedList, error) {
	query := url.Values{}
	query.Set("page", "1")
	query.Set("pageSize", "20")
	query.Set("sort", "createdAt:asc")

	var envelope struct {
		Data []UnassignedParcel `json:"data"`
		Meta struct {
			Pagination Pagination `json:"pagination"`
		} `json:"meta"`
	}
	if err := c.http.do(ctx, http.MethodGet, "/dispatch/unassigned", query, nil, strings.TrimSpace(accessToken), &envelope); err != nil {
		return UnassignedList{}, err
	}
	for _, item := range envelope.Data {
		if !validNumber(item.WeightKg) {
			return UnassignedList{}, &Error{Kind: KindUnavailable}
		}
	}
	return UnassignedList{Items: envelope.Data, Pagination: envelope.Meta.Pagination}, nil
}

// Workload returns the operational courier workload summary.
func (c *ParcelClient) Workload(ctx context.Context, accessToken string) ([]CourierWorkload, error) {
	var envelope struct {
		Data []CourierWorkload `json:"data"`
	}
	if err := c.http.do(ctx, http.MethodGet, "/dispatch/workload", nil, nil, strings.TrimSpace(accessToken), &envelope); err != nil {
		return nil, err
	}
	return envelope.Data, nil
}

// CourierDeliveries returns the authenticated courier's deliveries. The Parcel
// API derives the courier from the forwarded token and never from an argument.
func (c *ParcelClient) CourierDeliveries(ctx context.Context, status, date, accessToken string) (CourierDeliveryList, error) {
	query := url.Values{}
	query.Set("page", "1")
	query.Set("pageSize", "20")
	if value := strings.TrimSpace(status); value != "" {
		query.Set("status", value)
	}
	if value := strings.TrimSpace(date); value != "" {
		query.Set("date", value)
	}

	var envelope struct {
		Data []CourierDeliveryItem `json:"data"`
		Meta struct {
			Pagination Pagination `json:"pagination"`
		} `json:"meta"`
	}
	if err := c.http.do(ctx, http.MethodGet, "/courier/deliveries", query, nil, strings.TrimSpace(accessToken), &envelope); err != nil {
		return CourierDeliveryList{}, err
	}
	for _, item := range envelope.Data {
		if !validNumber(item.WeightKg) {
			return CourierDeliveryList{}, &Error{Kind: KindUnavailable}
		}
	}
	return CourierDeliveryList{Items: envelope.Data, Pagination: envelope.Meta.Pagination}, nil
}

// CourierDelivery returns one delivery owned by the authenticated courier.
func (c *ParcelClient) CourierDelivery(ctx context.Context, parcelID, accessToken string) (CourierDeliveryDetail, error) {
	id := strings.TrimSpace(parcelID)
	if id == "" {
		return CourierDeliveryDetail{}, &Error{Kind: KindInvalid}
	}

	var envelope struct {
		Data struct {
			ID             string      `json:"id"`
			TrackingNumber string      `json:"trackingNumber"`
			Status         string      `json:"status"`
			DropoffCity    string      `json:"dropoffCity"`
			RecipientName  string      `json:"recipientName"`
			WeightKg       json.Number `json:"weightKg"`
			SizeCategory   string      `json:"sizeCategory"`
			AssignedAt     string      `json:"assignedAt"`
			History        []struct {
				PreviousStatus *string `json:"previousStatus"`
				Status         string  `json:"status"`
				Note           *string `json:"note"`
				FailureReason  *string `json:"failureReason"`
				ChangedAt      string  `json:"changedAt"`
			} `json:"history"`
		} `json:"data"`
	}
	path := "/courier/deliveries/" + url.PathEscape(id)
	if err := c.http.do(ctx, http.MethodGet, path, nil, nil, strings.TrimSpace(accessToken), &envelope); err != nil {
		return CourierDeliveryDetail{}, err
	}
	if !validNumber(envelope.Data.WeightKg) {
		return CourierDeliveryDetail{}, &Error{Kind: KindUnavailable}
	}

	history := make([]CourierHistoryItem, 0, len(envelope.Data.History))
	for _, item := range envelope.Data.History {
		history = append(history, CourierHistoryItem{
			PreviousStatus: cleanOptional(item.PreviousStatus),
			Status:         item.Status,
			Note:           cleanOptional(item.Note),
			FailureReason:  cleanOptional(item.FailureReason),
			ChangedAt:      item.ChangedAt,
		})
	}
	return CourierDeliveryDetail{
		ID:             envelope.Data.ID,
		TrackingNumber: envelope.Data.TrackingNumber,
		Status:         envelope.Data.Status,
		DropoffCity:    envelope.Data.DropoffCity,
		RecipientName:  envelope.Data.RecipientName,
		WeightKg:       envelope.Data.WeightKg,
		SizeCategory:   envelope.Data.SizeCategory,
		AssignedAt:     envelope.Data.AssignedAt,
		History:        history,
	}, nil
}

// PublicTracking reads the anonymous public tracking projection. It is used
// only for unauthenticated callers; signed-in callers always use the
// ownership-scoped Tracking method.
func (c *ParcelClient) PublicTracking(ctx context.Context, trackingNumber string) (Tracking, error) {
	number := strings.TrimSpace(trackingNumber)
	if number == "" {
		return Tracking{}, &Error{Kind: KindInvalid}
	}

	var envelope struct {
		Data Tracking `json:"data"`
	}
	path := "/tracking/" + url.PathEscape(number)
	if err := c.http.do(ctx, http.MethodGet, path, nil, nil, "", &envelope); err != nil {
		return Tracking{}, err
	}
	return envelope.Data, nil
}

func cleanOptional(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}
