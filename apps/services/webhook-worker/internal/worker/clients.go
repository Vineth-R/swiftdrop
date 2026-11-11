package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/Vineth-R/swiftdrop/packages/go-common/requestid"
)

// maxResponseBytes caps what the worker reads from an internal API. The signing
// secret response is tiny; anything larger is a fault.
const maxResponseBytes = 64 << 10

// ErrNotConfigured means the merchant has no webhook, so there is nothing to
// deliver. It is a normal outcome, not a failure.
var ErrNotConfigured = errors.New("merchant has no webhook configured")

// Webhook is what the Merchant service returns to the worker (internal GET).
type Webhook struct {
	URL           string   `json:"url"`
	SigningSecret string   `json:"signingSecret"`
	EventTypes    []string `json:"eventTypes"`
	IsActive      bool     `json:"isActive"`
}

// Attempt is one delivery report for the merchant's delivery log. The JSON
// names match contracts/openapi/merchant-internal.yaml.
type Attempt struct {
	EventID        string     `json:"eventId"`
	ParcelID       string     `json:"parcelId"`
	TrackingNumber string     `json:"trackingNumber"`
	AttemptNumber  int        `json:"attemptNumber"`
	HTTPStatus     *int       `json:"httpStatus,omitempty"`
	Success        bool       `json:"success"`
	ErrorMessage   string     `json:"errorMessage,omitempty"`
	NextRetryAt    *time.Time `json:"nextRetryAt,omitempty"`
	DeadLettered   bool       `json:"deadLettered"`
	DurationMs     int64      `json:"durationMs"`
}

// internalClient is the small shared HTTP layer: token and request ID on every
// call, no redirects, bounded response size, and errors that never carry a
// response body (it could hold a secret).
type internalClient struct {
	baseURL string
	token   string
	http    *http.Client
}

func newInternalClient(baseURL, token string, timeout time.Duration) internalClient {
	return internalClient{
		baseURL: baseURL,
		token:   token,
		http: &http.Client{
			Timeout: timeout,
			// An internal call that redirects is a misconfiguration; following
			// it could send the internal token to another host.
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}
}

// statusError reports an unexpected HTTP status without echoing the body.
type statusError struct {
	op     string
	status int
}

func (e *statusError) Error() string { return fmt.Sprintf("%s: unexpected status %d", e.op, e.status) }

// do sends one request and decodes a JSON {"data": ...} response into out
// (when out is not nil). okStatuses lists the accepted statuses.
func (c internalClient) do(ctx context.Context, op, method, path string, body, out any, okStatuses ...int) (int, error) {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return 0, fmt.Errorf("%s: encode request: %w", op, err)
		}
		reader = bytes.NewReader(raw)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return 0, fmt.Errorf("%s: build request: %w", op, err)
	}
	req.Header.Set("X-Internal-Token", c.token)
	req.Header.Set("X-Request-Id", requestid.New())
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		// The URL error text can include the full URL; keep only the cause kind.
		var urlErr *url.Error
		if errors.As(err, &urlErr) {
			err = urlErr.Err
		}
		return 0, fmt.Errorf("%s: request failed: %w", op, err)
	}
	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return resp.StatusCode, fmt.Errorf("%s: read response: %w", op, err)
	}

	accepted := false
	for _, status := range okStatuses {
		if resp.StatusCode == status {
			accepted = true
			break
		}
	}
	if !accepted {
		return resp.StatusCode, &statusError{op: op, status: resp.StatusCode}
	}

	// Only a 2xx carries {"data": ...}; an accepted 404 is an error body.
	if out != nil && resp.StatusCode/100 == 2 {
		var envelope struct {
			Data json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(raw, &envelope); err != nil || len(envelope.Data) == 0 {
			return resp.StatusCode, fmt.Errorf("%s: unexpected response shape", op)
		}
		if err := json.Unmarshal(envelope.Data, out); err != nil {
			return resp.StatusCode, fmt.Errorf("%s: decode response: %w", op, err)
		}
	}
	return resp.StatusCode, nil
}

// MerchantClient calls the Merchant service's internal webhook endpoints.
type MerchantClient struct{ c internalClient }

func NewMerchantClient(baseURL, token string, timeout time.Duration) *MerchantClient {
	return &MerchantClient{c: newInternalClient(baseURL, token, timeout)}
}

// Webhook returns the merchant's webhook with the plain signing secret.
// ErrNotConfigured means the merchant has none.
func (m *MerchantClient) Webhook(ctx context.Context, merchantID string) (Webhook, error) {
	var out Webhook
	status, err := m.c.do(ctx, "get webhook", http.MethodGet,
		"/internal/merchants/"+url.PathEscape(merchantID)+"/webhook", nil, &out, http.StatusOK, http.StatusNotFound)
	if err != nil {
		return Webhook{}, err
	}
	if status == http.StatusNotFound {
		return Webhook{}, ErrNotConfigured
	}
	return out, nil
}

// RecordAttempt reports one attempt. The endpoint is idempotent: a repeated
// report answers 200 and changes nothing.
func (m *MerchantClient) RecordAttempt(ctx context.Context, merchantID string, attempt Attempt) error {
	_, err := m.c.do(ctx, "record attempt", http.MethodPost,
		"/internal/merchants/"+url.PathEscape(merchantID)+"/webhook/attempts", attempt, nil,
		http.StatusOK, http.StatusCreated)
	return err
}

// ParcelClient calls the Parcel service's internal summary endpoint.
type ParcelClient struct{ c internalClient }

func NewParcelClient(baseURL, token string, timeout time.Duration) *ParcelClient {
	return &ParcelClient{c: newInternalClient(baseURL, token, timeout)}
}

// ExternalOrderID returns the merchant's own order reference for a parcel, or
// nil when the parcel has none (or no longer exists).
func (p *ParcelClient) ExternalOrderID(ctx context.Context, parcelID string) (*string, error) {
	var out struct {
		ExternalOrderID *string `json:"externalOrderId"`
	}
	status, err := p.c.do(ctx, "get parcel summary", http.MethodGet,
		"/internal/parcels/"+url.PathEscape(parcelID)+"/summary", nil, &out, http.StatusOK, http.StatusNotFound)
	if err != nil {
		return nil, err
	}
	if status == http.StatusNotFound {
		return nil, nil
	}
	return out.ExternalOrderID, nil
}
