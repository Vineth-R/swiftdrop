package pricingclient

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Vineth-R/swiftdrop/packages/go-common/requestid"
)

var ErrUnavailable = errors.New("pricing service is unavailable")

type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

type Zone struct {
	ID       string `json:"id"`
	Code     string `json:"code"`
	Name     string `json:"name"`
	IsActive bool   `json:"isActive"`
}

func New(baseURL, token string, timeout time.Duration) (*Client, error) {
	trimmed := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, errors.New("PRICING_BASE_URL must be an absolute http or https URL")
	}
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	return &Client{baseURL: trimmed, token: strings.TrimSpace(token), http: &http.Client{Timeout: timeout}}, nil
}

func (c *Client) Zone(ctx context.Context, zoneID string) (Zone, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/internal/zones/"+url.PathEscape(zoneID), nil)
	if err != nil {
		return Zone{}, ErrUnavailable
	}
	request.Header.Set("X-Internal-Token", c.token)
	request.Header.Set(requestid.Header, requestid.New())
	response, err := c.http.Do(request)
	if err != nil {
		return Zone{}, ErrUnavailable
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return Zone{}, ErrUnavailable
	}
	var envelope struct {
		Data Zone `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		return Zone{}, ErrUnavailable
	}
	return envelope.Data, nil
}
