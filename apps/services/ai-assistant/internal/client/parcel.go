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
func (c *ParcelClient) AdminParcels(ctx context.Context, status, accessToken string) (AdminParcelList, error) {
	query := url.Values{}
	query.Set("page", "1")
	query.Set("pageSize", "20")
	if trimmed := strings.TrimSpace(status); trimmed != "" {
		query.Set("status", trimmed)
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
