// Package merchantclient resolves a merchant's notification contact details
// through the merchant service's internal API. It is used by the notification
// worker for merchant-owned parcel domain events.
package merchantclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Vineth-R/swiftdrop/packages/go-common/requestid"
)

// Merchant is the approved merchant contact set returned by the merchant
// service. The owner user ID is carried for auditability; the notification
// recipient is ContactEmail.
type Merchant struct {
	ID           string
	OwnerUserID  string
	BusinessName string
	ContactEmail string
	Status       string
}

// Stable, non-sensitive errors. Response bodies are never included so contact
// data and credentials cannot reach logs.
var (
	ErrNotFound        = errors.New("merchant not found")
	ErrUnauthorized    = errors.New("merchant internal authentication failed")
	ErrInvalidResponse = errors.New("merchant contact response is invalid")
	ErrUnavailable     = errors.New("merchant contact lookup is unavailable")
)

// Client calls GET /internal/merchants/{merchantId}/contact.
type Client struct {
	baseURL string
	token   string
	timeout time.Duration
	http    *http.Client
}

// New returns a client. baseURL must be an absolute http or https URL. The
// token is sent as X-Internal-Token; an empty token fails closed at the
// merchant service.
func New(baseURL, token string, timeout time.Duration) (*Client, error) {
	trimmed := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, errors.New("MERCHANT_BASE_URL must be an absolute http or https URL")
	}
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	return &Client{
		baseURL: trimmed,
		token:   strings.TrimSpace(token),
		timeout: timeout,
		http: &http.Client{
			Timeout: timeout,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}, nil
}

// Merchant resolves one merchant. A 404, 401, 403, 400 or 405 is permanent; a
// 5xx, network error or timeout is retryable.
func (c *Client) Merchant(ctx context.Context, merchantID string) (Merchant, error) {
	if c == nil || c.baseURL == "" {
		return Merchant{}, ErrUnavailable
	}

	requestCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	endpoint := c.baseURL + "/internal/merchants/" + url.PathEscape(merchantID) + "/contact"
	request, err := http.NewRequestWithContext(requestCtx, http.MethodGet, endpoint, nil)
	if err != nil {
		return Merchant{}, fmt.Errorf("%w: build request", ErrUnavailable)
	}
	request.Header.Set("X-Internal-Token", c.token)
	requestID := requestid.FromContext(ctx)
	if !requestid.Valid(requestID) {
		requestID = requestid.New()
	}
	request.Header.Set(requestid.Header, requestID)

	response, err := c.http.Do(request)
	if err != nil {
		return Merchant{}, fmt.Errorf("%w: request failed", ErrUnavailable)
	}
	defer response.Body.Close()

	switch {
	case response.StatusCode == http.StatusOK:
		var envelope struct {
			Data struct {
				MerchantID   string `json:"merchantId"`
				OwnerUserID  string `json:"ownerUserId"`
				BusinessName string `json:"businessName"`
				ContactEmail string `json:"contactEmail"`
				Status       string `json:"status"`
			} `json:"data"`
		}
		if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&envelope); err != nil {
			return Merchant{}, fmt.Errorf("%w: decode response", ErrInvalidResponse)
		}
		return Merchant{
			ID:           strings.TrimSpace(envelope.Data.MerchantID),
			OwnerUserID:  strings.TrimSpace(envelope.Data.OwnerUserID),
			BusinessName: strings.TrimSpace(envelope.Data.BusinessName),
			ContactEmail: strings.TrimSpace(envelope.Data.ContactEmail),
			Status:       strings.TrimSpace(envelope.Data.Status),
		}, nil
	case response.StatusCode == http.StatusNotFound:
		return Merchant{}, ErrNotFound
	case response.StatusCode == http.StatusUnauthorized,
		response.StatusCode == http.StatusForbidden:
		return Merchant{}, ErrUnauthorized
	case response.StatusCode == http.StatusBadRequest,
		response.StatusCode == http.StatusMethodNotAllowed:
		return Merchant{}, fmt.Errorf("%w: status %d", ErrInvalidResponse, response.StatusCode)
	case response.StatusCode >= http.StatusInternalServerError:
		return Merchant{}, fmt.Errorf("%w: status %d", ErrUnavailable, response.StatusCode)
	default:
		return Merchant{}, fmt.Errorf("%w: status %d", ErrInvalidResponse, response.StatusCode)
	}
}
