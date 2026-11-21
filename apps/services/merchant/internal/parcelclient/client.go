// Package parcelclient calls the trusted Parcel internal creation endpoint.
package parcelclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Vineth-R/swiftdrop/apps/services/merchant/internal/orders"
	"github.com/Vineth-R/swiftdrop/packages/go-common/requestid"
	"github.com/Vineth-R/swiftdrop/packages/go-common/validation"
)

const maxResponseBytes = 1 << 20

// Client is the Parcel Operations internal client.
type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

// New validates the Parcel base URL and timeout.
func New(baseURL, token string, timeout time.Duration) (*Client, error) {
	trimmed := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, errors.New("PARCEL_BASE_URL must be an absolute http or https URL")
	}
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	return &Client{baseURL: trimmed, token: strings.TrimSpace(token), http: &http.Client{Timeout: timeout}}, nil
}

type createRequest struct {
	MerchantID      string         `json:"merchantId"`
	CreatedByUserID string         `json:"createdByUserId"`
	ExternalOrderID string         `json:"externalOrderId"`
	IdempotencyKey  string         `json:"idempotencyKey"`
	Pickup          orders.Contact `json:"pickup"`
	Recipient       orders.Contact `json:"recipient"`
	WeightKG        orders.Decimal `json:"weightKg"`
	SizeCategory    string         `json:"sizeCategory"`
	Description     *string        `json:"description,omitempty"`
}

type successEnvelope struct {
	Data struct {
		ParcelID       string     `json:"parcelId"`
		TrackingNumber string     `json:"trackingNumber"`
		Status         string     `json:"status"`
		Fee            orders.Fee `json:"fee"`
	} `json:"data"`
}

type errorEnvelope struct {
	Error struct {
		Code    string `json:"code"`
		Details []struct {
			Field string `json:"field"`
			Issue string `json:"issue"`
		} `json:"details"`
	} `json:"error"`
}

// CreateMerchant calls POST /internal/parcels. It never trusts a client-supplied
// merchant identity; the caller supplies the verified values.
func (c *Client) CreateMerchant(ctx context.Context, input orders.ParcelInput) (orders.ParcelResult, error) {
	body, err := json.Marshal(createRequest{
		MerchantID:      input.MerchantID,
		CreatedByUserID: input.CreatedByUserID,
		ExternalOrderID: input.ExternalOrderID,
		IdempotencyKey:  input.IdempotencyKey,
		Pickup:          input.Pickup,
		Recipient:       input.Recipient,
		WeightKG:        input.WeightKG,
		SizeCategory:    input.SizeCategory,
		Description:     input.Description,
	})
	if err != nil {
		return orders.ParcelResult{}, orders.ErrParcelUnavailable
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/internal/parcels", bytes.NewReader(body))
	if err != nil {
		return orders.ParcelResult{}, orders.ErrParcelUnavailable
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Internal-Token", c.token)
	requestID := requestid.FromContext(ctx)
	if requestID == "" {
		requestID = requestid.New()
	}
	request.Header.Set(requestid.Header, requestID)

	response, err := c.http.Do(request)
	if err != nil {
		return orders.ParcelResult{}, orders.ErrParcelUnavailable
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes))
	if err != nil {
		return orders.ParcelResult{}, orders.ErrParcelUnavailable
	}
	if response.StatusCode != http.StatusCreated && response.StatusCode != http.StatusOK {
		return orders.ParcelResult{}, mapError(response.StatusCode, raw)
	}

	var envelope successEnvelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return orders.ParcelResult{}, orders.ErrParcelUnavailable
	}
	if !validation.UUID(envelope.Data.ParcelID) || strings.TrimSpace(envelope.Data.TrackingNumber) == "" {
		return orders.ParcelResult{}, orders.ErrParcelUnavailable
	}
	return orders.ParcelResult{
		ParcelID:       envelope.Data.ParcelID,
		TrackingNumber: envelope.Data.TrackingNumber,
		Status:         envelope.Data.Status,
		Fee:            envelope.Data.Fee,
	}, nil
}

func mapError(status int, raw []byte) error {
	var envelope errorEnvelope
	_ = json.Unmarshal(raw, &envelope)
	code := strings.TrimSpace(envelope.Error.Code)

	switch status {
	case http.StatusConflict:
		return orders.ErrDuplicateOrder
	case http.StatusUnprocessableEntity:
		if code == "" {
			code = "VALIDATION_ERROR"
		}
		issues := make([]validation.Issue, 0, len(envelope.Error.Details))
		for _, item := range envelope.Error.Details {
			issues = append(issues, validation.Issue{Field: item.Field, Issue: item.Issue})
		}
		return &orders.UpstreamInvalid{Code: code, Issues: issues}
	default:
		return fmt.Errorf("%w: parcel returned status %d", orders.ErrParcelUnavailable, status)
	}
}
