// Package identityclient resolves a user's contact details through the
// identity service's internal API. It is used by the notification worker for
// customer-owned parcel domain events.
package identityclient

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

// Contact is the contact detail set returned by identity. Phone is empty when
// the user has none.
type Contact struct {
	FullName string
	Email    string
	Phone    string
}

// Stable, non-sensitive errors. Driver and response bodies are never included
// so contact data and credentials cannot reach logs.
var (
	ErrNotFound        = errors.New("identity user not found")
	ErrUnauthorized    = errors.New("identity internal authentication failed")
	ErrInvalidResponse = errors.New("identity contact response is invalid")
	ErrUnavailable     = errors.New("identity contact lookup is unavailable")
)

// Client calls GET /internal/users/{userId}/contact.
type Client struct {
	baseURL string
	token   string
	timeout time.Duration
	http    *http.Client
}

// New returns a client. baseURL must be an absolute http or https URL. The
// token is sent as X-Internal-Token; an empty token fails closed at identity.
func New(baseURL, token string, timeout time.Duration) (*Client, error) {
	trimmed := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, errors.New("IDENTITY_BASE_URL must be an absolute http or https URL")
	}
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	return &Client{
		baseURL: trimmed,
		token:   strings.TrimSpace(token),
		timeout: timeout,
		http:    &http.Client{Timeout: timeout},
	}, nil
}

// Contact resolves one user. A 404, 401, 400, 403 or 405 is permanent; a 5xx,
// network error or timeout is retryable.
func (c *Client) Contact(ctx context.Context, userID string) (Contact, error) {
	if c == nil || c.baseURL == "" {
		return Contact{}, ErrUnavailable
	}

	requestCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	endpoint := c.baseURL + "/internal/users/" + url.PathEscape(userID) + "/contact"
	request, err := http.NewRequestWithContext(requestCtx, http.MethodGet, endpoint, nil)
	if err != nil {
		return Contact{}, fmt.Errorf("%w: build request", ErrUnavailable)
	}
	request.Header.Set("X-Internal-Token", c.token)
	requestID := requestid.FromContext(ctx)
	if !requestid.Valid(requestID) {
		requestID = requestid.New()
	}
	request.Header.Set(requestid.Header, requestID)

	response, err := c.http.Do(request)
	if err != nil {
		return Contact{}, fmt.Errorf("%w: request failed", ErrUnavailable)
	}
	defer response.Body.Close()

	switch {
	case response.StatusCode == http.StatusOK:
		var envelope struct {
			Data struct {
				FullName string `json:"fullName"`
				Email    string `json:"email"`
				Phone    string `json:"phone"`
			} `json:"data"`
		}
		if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&envelope); err != nil {
			return Contact{}, fmt.Errorf("%w: decode response", ErrInvalidResponse)
		}
		email := strings.TrimSpace(envelope.Data.Email)
		if email == "" {
			return Contact{}, fmt.Errorf("%w: email is empty", ErrInvalidResponse)
		}
		return Contact{
			FullName: strings.TrimSpace(envelope.Data.FullName),
			Email:    email,
			Phone:    strings.TrimSpace(envelope.Data.Phone),
		}, nil
	case response.StatusCode == http.StatusNotFound:
		return Contact{}, ErrNotFound
	case response.StatusCode == http.StatusUnauthorized,
		response.StatusCode == http.StatusBadRequest,
		response.StatusCode == http.StatusForbidden,
		response.StatusCode == http.StatusMethodNotAllowed:
		return Contact{}, ErrUnauthorized
	case response.StatusCode >= http.StatusInternalServerError:
		return Contact{}, fmt.Errorf("%w: status %d", ErrUnavailable, response.StatusCode)
	default:
		return Contact{}, fmt.Errorf("%w: status %d", ErrInvalidResponse, response.StatusCode)
	}
}
