package client

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

// Zone is one active public zone. Only the fields exposed by the public
// directory are modelled; anything else in the upstream payload is ignored.
type Zone struct {
	ID        string   `json:"id"`
	Code      string   `json:"code"`
	Name      string   `json:"name"`
	Districts []string `json:"districts"`
}

// EstimateInput is the exact public estimate request body.
type EstimateInput struct {
	OriginZoneID      string      `json:"originZoneId"`
	DestinationZoneID string      `json:"destinationZoneId"`
	WeightKg          json.Number `json:"weightKg"`
	SizeCategory      string      `json:"sizeCategory"`
}

// Estimate is the authoritative quote. Amounts stay as json.Number so their
// decimal text is never rounded or reformatted through float64.
type Estimate struct {
	OriginZoneID      string      `json:"originZoneId"`
	DestinationZoneID string      `json:"destinationZoneId"`
	WeightKg          json.Number `json:"weightKg"`
	SizeCategory      string      `json:"sizeCategory"`
	Currency          string      `json:"currency"`
	BaseFee           json.Number `json:"baseFee"`
	WeightCharge      json.Number `json:"weightCharge"`
	SizeSurcharge     json.Number `json:"sizeSurcharge"`
	Total             json.Number `json:"total"`
}

// PricingClient calls the Pricing service's public read endpoints.
type PricingClient struct {
	http *httpClient
}

// NewPricingClient validates cfg and returns a Pricing client.
func NewPricingClient(cfg Config) (*PricingClient, error) {
	httpClient, err := newHTTPClient(cfg)
	if err != nil {
		return nil, err
	}
	return &PricingClient{http: httpClient}, nil
}

// Zones returns the active public zone directory.
func (c *PricingClient) Zones(ctx context.Context) ([]Zone, error) {
	var envelope struct {
		Data []Zone `json:"data"`
	}
	if err := c.http.do(ctx, http.MethodGet, "/zones", nil, nil, "", &envelope); err != nil {
		return nil, err
	}
	return envelope.Data, nil
}

// Estimate returns the authoritative quote for one route, weight and size.
func (c *PricingClient) Estimate(ctx context.Context, input EstimateInput) (Estimate, error) {
	if strings.TrimSpace(input.OriginZoneID) == "" ||
		strings.TrimSpace(input.DestinationZoneID) == "" ||
		!validNumber(input.WeightKg) ||
		strings.TrimSpace(input.SizeCategory) == "" {
		return Estimate{}, &Error{Kind: KindInvalid}
	}

	var envelope struct {
		Data Estimate `json:"data"`
	}
	if err := c.http.do(ctx, http.MethodPost, "/pricing/estimate", nil, input, "", &envelope); err != nil {
		return Estimate{}, err
	}
	if !validEstimate(envelope.Data) {
		return Estimate{}, &Error{Kind: KindUnavailable}
	}
	return envelope.Data, nil
}

// validEstimate rejects a malformed 200 response rather than letting a missing
// amount marshal as zero and fabricating a price.
func validEstimate(estimate Estimate) bool {
	return strings.TrimSpace(estimate.OriginZoneID) != "" &&
		strings.TrimSpace(estimate.DestinationZoneID) != "" &&
		strings.TrimSpace(estimate.Currency) != "" &&
		validNumber(estimate.BaseFee) &&
		validNumber(estimate.WeightCharge) &&
		validNumber(estimate.SizeSurcharge) &&
		validNumber(estimate.Total)
}

func validNumber(number json.Number) bool {
	if number == "" {
		return false
	}
	_, err := number.Float64()
	return err == nil
}
