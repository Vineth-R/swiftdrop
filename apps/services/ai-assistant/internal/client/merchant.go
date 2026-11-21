package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// MerchantOrderItem is one merchant-owned order summary. Merchant identifiers,
// contacts and addresses are not modelled.
type MerchantOrderItem struct {
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
}

// MerchantOrderList is the paginated merchant order response.
type MerchantOrderList struct {
	Items      []MerchantOrderItem
	Pagination Pagination
}

// MerchantOrderFilter is the approved merchant order filter. The upstream API
// has no tracking-number filter; TrackingNumber is matched by the tool against
// the returned page.
type MerchantOrderFilter struct {
	Status          string
	ExternalOrderID string
	PageSize        int
}

// WebhookDeliveryAttempt is one merchant-owned webhook delivery attempt. The
// payload, endpoint URL and signing secret are not modelled.
type WebhookDeliveryAttempt struct {
	EventID        string  `json:"eventId"`
	TrackingNumber string  `json:"trackingNumber"`
	AttemptNumber  int     `json:"attemptNumber"`
	HTTPStatus     *int    `json:"httpStatus"`
	Success        bool    `json:"success"`
	NextRetryAt    *string `json:"nextRetryAt"`
	DeadLettered   bool    `json:"deadLettered"`
	CreatedAt      string  `json:"createdAt"`
}

// WebhookDeliveryList is the paginated webhook delivery response.
type WebhookDeliveryList struct {
	Items      []WebhookDeliveryAttempt
	Pagination Pagination
}

// MerchantClient calls the Merchant service's merchant-scoped read endpoints.
// Every request forwards the caller's own token; the Merchant service resolves
// the merchant from the verified session.
type MerchantClient struct {
	http *httpClient
}

// NewMerchantClient validates cfg and returns a Merchant client.
func NewMerchantClient(cfg Config) (*MerchantClient, error) {
	httpClient, err := newHTTPClient(cfg)
	if err != nil {
		return nil, err
	}
	return &MerchantClient{http: httpClient}, nil
}

// Orders returns one page of the authenticated merchant's own orders.
func (c *MerchantClient) Orders(ctx context.Context, filter MerchantOrderFilter, accessToken string) (MerchantOrderList, error) {
	query := url.Values{}
	query.Set("page", "1")
	pageSize := filter.PageSize
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	query.Set("pageSize", strconv.Itoa(pageSize))
	query.Set("sort", "createdAt:desc")
	if value := strings.TrimSpace(filter.Status); value != "" {
		query.Set("status", value)
	}
	if value := strings.TrimSpace(filter.ExternalOrderID); value != "" {
		query.Set("externalOrderId", value)
	}

	var envelope struct {
		Data []MerchantOrderItem `json:"data"`
		Meta struct {
			Pagination Pagination `json:"pagination"`
		} `json:"meta"`
	}
	if err := c.http.do(ctx, http.MethodGet, "/merchant/orders", query, nil, strings.TrimSpace(accessToken), &envelope); err != nil {
		return MerchantOrderList{}, err
	}
	for _, item := range envelope.Data {
		if !validNumber(item.Total) {
			return MerchantOrderList{}, &Error{Kind: KindUnavailable}
		}
	}
	return MerchantOrderList{Items: envelope.Data, Pagination: envelope.Meta.Pagination}, nil
}

// WebhookDeliveries returns the authenticated merchant's own delivery attempts.
// A merchant without a webhook receives a marker error so the tool can explain
// the configuration state instead of reporting an outage.
func (c *MerchantClient) WebhookDeliveries(ctx context.Context, success *bool, accessToken string) (WebhookDeliveryList, error) {
	query := url.Values{}
	query.Set("page", "1")
	query.Set("pageSize", "20")
	if success != nil {
		if *success {
			query.Set("success", "true")
		} else {
			query.Set("success", "false")
		}
	}

	var envelope struct {
		Data []WebhookDeliveryAttempt `json:"data"`
		Meta struct {
			Pagination Pagination `json:"pagination"`
		} `json:"meta"`
	}
	if err := c.http.do(ctx, http.MethodGet, "/merchants/me/webhook/deliveries", query, nil, strings.TrimSpace(accessToken), &envelope); err != nil {
		return WebhookDeliveryList{}, err
	}
	return WebhookDeliveryList{Items: envelope.Data, Pagination: envelope.Meta.Pagination}, nil
}
