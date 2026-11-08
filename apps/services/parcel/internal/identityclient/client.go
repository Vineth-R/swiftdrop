// Package identityclient communicates with the Identity service's internal and
// operational APIs. It is used by the parcel dispatch operations to check courier
// eligibility and query courier availability.
package identityclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Vineth-R/swiftdrop/packages/go-common/auth"
	"github.com/Vineth-R/swiftdrop/packages/go-common/requestid"
)

// Domain errors mapped to API v1.1 error responses.
var (
	ErrCourierNotEligible = errors.New("courier is not eligible for assignment")
	ErrServiceUnavailable = errors.New("identity service is unavailable")
)

// Eligibility is the courier eligibility payload returned by Identity.
type Eligibility struct {
	IsActive    bool      `json:"isActive"`
	IsAvailable bool      `json:"isAvailable"`
	Role        auth.Role `json:"role"`
}

// Courier contains the basic identity attributes for workload reporting.
type Courier struct {
	ID          string `json:"id"`
	FullName    string `json:"fullName"`
	IsAvailable bool   `json:"isAvailable"`
}

// CourierClient defines the interface required by the dispatch service.
type CourierClient interface {
	CheckEligibility(ctx context.Context, courierID string) error
	ListCouriers(ctx context.Context, authHeader string, cookie *http.Cookie) ([]Courier, error)
}

// Client calls the Identity service.
type Client struct {
	baseURL string
	token   string
	timeout time.Duration
	http    *http.Client
}

// New returns a configured Client.
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
		http: &http.Client{
			Timeout: timeout,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}, nil
}

// WithTransport configures a custom RoundTripper, primarily for testing.
func (c *Client) WithTransport(rt http.RoundTripper) *Client {
	if c != nil && c.http != nil {
		c.http.Transport = rt
	}
	return c
}

// CheckEligibility calls GET /internal/couriers/{courierId}/eligibility.
// It verifies that the courier exists, has role COURIER, isActive=true, and isAvailable=true.
func (c *Client) CheckEligibility(ctx context.Context, courierID string) error {
	if c == nil || c.baseURL == "" {
		return ErrServiceUnavailable
	}

	trimmedID := strings.TrimSpace(courierID)
	if trimmedID == "" {
		return ErrCourierNotEligible
	}

	requestCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	endpoint := c.baseURL + "/internal/couriers/" + url.PathEscape(trimmedID) + "/eligibility"
	request, err := http.NewRequestWithContext(requestCtx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("%w: build request: %v", ErrServiceUnavailable, err)
	}

	request.Header.Set("X-Internal-Token", c.token)
	requestID := requestid.FromContext(ctx)
	if !requestid.Valid(requestID) {
		requestID = requestid.New()
	}
	request.Header.Set(requestid.Header, requestID)

	response, err := c.http.Do(request)
	if err != nil {
		return fmt.Errorf("%w: request failed: %v", ErrServiceUnavailable, err)
	}
	defer response.Body.Close()

	switch response.StatusCode {
	case http.StatusOK:
		var envelope struct {
			Data Eligibility `json:"data"`
		}
		if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
			return fmt.Errorf("%w: parse response: %v", ErrServiceUnavailable, err)
		}
		if !envelope.Data.IsActive || !envelope.Data.IsAvailable || envelope.Data.Role != auth.RoleCourier {
			return ErrCourierNotEligible
		}
		return nil

	case http.StatusNotFound:
		return ErrCourierNotEligible

	case http.StatusUnauthorized, http.StatusForbidden:
		// Authentication/token configuration mismatch is a fail-closed service unavailable
		return fmt.Errorf("%w: identity authentication rejected", ErrServiceUnavailable)

	default:
		return fmt.Errorf("%w: unexpected status %d", ErrServiceUnavailable, response.StatusCode)
	}
}

// ListCouriers calls GET /couriers to retrieve the current couriers for dispatch workload balancing.
func (c *Client) ListCouriers(ctx context.Context, authHeader string, cookie *http.Cookie) ([]Courier, error) {
	if c == nil || c.baseURL == "" {
		return nil, ErrServiceUnavailable
	}

	requestCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	var allCouriers []Courier
	page := 1
	pageSize := 100

	for {
		endpoint := fmt.Sprintf("%s/couriers?page=%d&pageSize=%d", c.baseURL, page, pageSize)
		request, err := http.NewRequestWithContext(requestCtx, http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, fmt.Errorf("%w: build request: %v", ErrServiceUnavailable, err)
		}

		if authHeader != "" {
			request.Header.Set("Authorization", authHeader)
		}
		if cookie != nil {
			request.AddCookie(cookie)
		}

		requestID := requestid.FromContext(ctx)
		if !requestid.Valid(requestID) {
			requestID = requestid.New()
		}
		request.Header.Set(requestid.Header, requestID)

		response, err := c.http.Do(request)
		if err != nil {
			return nil, fmt.Errorf("%w: request failed: %v", ErrServiceUnavailable, err)
		}

		if response.StatusCode != http.StatusOK {
			_ = response.Body.Close()
			return nil, fmt.Errorf("%w: unexpected status %d", ErrServiceUnavailable, response.StatusCode)
		}

		var envelope struct {
			Data []struct {
				ID          string `json:"id"`
				FullName    string `json:"fullName"`
				IsAvailable bool   `json:"isAvailable"`
			} `json:"data"`
			Meta struct {
				Pagination *struct {
					Page       int `json:"page"`
					PageSize   int `json:"pageSize"`
					TotalItems int `json:"totalItems"`
					TotalPages int `json:"totalPages"`
				} `json:"pagination"`
			} `json:"meta"`
		}
		if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
			_ = response.Body.Close()
			return nil, fmt.Errorf("%w: parse response: %v", ErrServiceUnavailable, err)
		}
		_ = response.Body.Close()

		for _, item := range envelope.Data {
			allCouriers = append(allCouriers, Courier{
				ID:          item.ID,
				FullName:    item.FullName,
				IsAvailable: item.IsAvailable,
			})
		}

		if len(envelope.Data) == 0 {
			break
		}
		if envelope.Meta.Pagination != nil {
			if page >= envelope.Meta.Pagination.TotalPages {
				break
			}
		} else if len(envelope.Data) < pageSize {
			break
		}
		page++
	}

	return allCouriers, nil
}
