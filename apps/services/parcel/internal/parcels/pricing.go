package parcels

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/Vineth-R/swiftdrop/packages/go-common/requestid"
)

type PricingClient struct {
	baseURL, token string
	client         *http.Client
}

func NewPricingClient(baseURL, token string, client *http.Client) *PricingClient {
	return &PricingClient{strings.TrimRight(baseURL, "/"), token, client}
}
func (p *PricingClient) Quote(ctx context.Context, in CreateInput) (Fee, error) {
	if p.client == nil || p.baseURL == "" || p.token == "" {
		return Fee{}, ErrUnavailable
	}
	input := struct {
		Origin      string  `json:"originZoneId"`
		Destination string  `json:"destinationZoneId"`
		Weight      Decimal `json:"weightKg"`
		Size        string  `json:"sizeCategory"`
	}{in.Sender.ZoneID, in.Recipient.ZoneID, in.WeightKG, in.SizeCategory}
	body, err := json.Marshal(input)
	if err != nil {
		return Fee{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/internal/pricing/quote", bytes.NewReader(body))
	if err != nil {
		return Fee{}, ErrUnavailable
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Token", p.token)
	if id := requestid.FromContext(ctx); id != "" {
		req.Header.Set(requestid.Header, id)
	}
	response, err := p.client.Do(req)
	if err != nil {
		return Fee{}, ErrUnavailable
	}
	defer response.Body.Close()
	var envelope struct {
		// Pointer fields distinguish an explicit zero-priced quote from missing
		// or null authoritative amounts, which must never become a free parcel.
		Data *struct {
			Currency      *string  `json:"currency"`
			BaseFee       *Decimal `json:"baseFee"`
			WeightCharge  *Decimal `json:"weightCharge"`
			SizeSurcharge *Decimal `json:"sizeSurcharge"`
			Total         *Decimal `json:"total"`
		} `json:"data"`
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	decoder := json.NewDecoder(io.LimitReader(response.Body, 1<<20))
	if err := decoder.Decode(&envelope); err != nil {
		return Fee{}, ErrUnavailable
	}
	if response.StatusCode == 404 && envelope.Error.Code == "ZONE_NOT_FOUND" {
		return Fee{}, invalid("zones", "pickup and destination zones must be active")
	}
	if response.StatusCode == 422 {
		switch envelope.Error.Code {
		case "ROUTE_NOT_SERVICEABLE", "WEIGHT_NOT_PRICED", "VALIDATION_ERROR":
			return Fee{}, &PricingError{envelope.Error.Code}
		}
	}
	if response.StatusCode != 200 {
		return Fee{}, ErrUnavailable
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return Fee{}, ErrUnavailable
	}
	quote := envelope.Data
	if quote == nil || quote.Currency == nil || *quote.Currency != "LKR" || quote.BaseFee == nil || quote.WeightCharge == nil || quote.SizeSurcharge == nil || quote.Total == nil {
		return Fee{}, ErrUnavailable
	}
	return Fee{Currency: *quote.Currency, BaseFee: *quote.BaseFee, WeightCharge: *quote.WeightCharge, SizeSurcharge: *quote.SizeSurcharge, Total: *quote.Total}, nil
}
