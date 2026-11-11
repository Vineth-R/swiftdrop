// Package identityclient calls the Identity service's authenticated internal
// APIs for merchant account provisioning and lifecycle transitions.
package identityclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Vineth-R/swiftdrop/packages/go-common/requestid"
)

var (
	ErrUnavailable        = errors.New("identity service is unavailable")
	ErrEmailTaken         = errors.New("email is already registered")
	ErrTransitionConflict = errors.New("transition id conflict")
	ErrTransitionStale    = errors.New("transition sequence is stale")
	ErrNotFound           = errors.New("identity user not found")
)

type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

type OwnerInput struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	FullName string `json:"fullName"`
	Phone    string `json:"phone"`
}

type OwnerResult struct {
	UserID  string `json:"userId"`
	Created bool   `json:"created"`
}

type LifecycleInput struct {
	TransitionID string `json:"transitionId"`
	Sequence     int64  `json:"sequence"`
	Operation    string `json:"operation"`
}

type LifecycleResult struct {
	UserID          string `json:"userId"`
	TransitionID    string `json:"transitionId"`
	Sequence        int64  `json:"sequence"`
	Operation       string `json:"operation"`
	AlreadyApplied  bool   `json:"alreadyApplied"`
	SessionVersion  int    `json:"sessionVersion"`
	IsActive        bool   `json:"isActive"`
	RevokedSessions int64  `json:"revokedSessions"`
}

type Contact struct {
	FullName string `json:"fullName"`
	Email    string `json:"email"`
	Phone    string `json:"phone"`
}

type SessionState struct {
	Role           string `json:"role"`
	IsActive       bool   `json:"isActive"`
	SessionVersion int    `json:"sessionVersion"`
}

func New(baseURL, token string, timeout time.Duration) (*Client, error) {
	trimmed := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, errors.New("IDENTITY_BASE_URL must be an absolute http or https URL")
	}
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	return &Client{baseURL: trimmed, token: strings.TrimSpace(token), http: &http.Client{Timeout: timeout}}, nil
}

func (c *Client) CreateMerchantOwner(ctx context.Context, input OwnerInput) (OwnerResult, error) {
	var result OwnerResult
	err := c.do(ctx, http.MethodPost, "/internal/merchant-owners", input, &result)
	switch {
	case errors.Is(err, ErrEmailTaken), errors.Is(err, ErrUnavailable):
		return OwnerResult{}, err
	case err != nil:
		return OwnerResult{}, err
	}
	return result, nil
}

func (c *Client) ApplyLifecycle(ctx context.Context, userID string, input LifecycleInput) (LifecycleResult, error) {
	var result LifecycleResult
	err := c.do(ctx, http.MethodPost, "/internal/merchant-owners/"+url.PathEscape(userID)+"/lifecycle", input, &result)
	return result, err
}

func (c *Client) DeleteMerchantOwner(ctx context.Context, userID string) error {
	return c.do(ctx, http.MethodDelete, "/internal/merchant-owners/"+url.PathEscape(userID), nil, nil)
}

func (c *Client) Contact(ctx context.Context, userID string) (Contact, error) {
	var contact Contact
	err := c.do(ctx, http.MethodGet, "/internal/users/"+url.PathEscape(userID)+"/contact", nil, &contact)
	return contact, err
}

func (c *Client) SessionState(ctx context.Context, userID string) (SessionState, error) {
	var state SessionState
	err := c.do(ctx, http.MethodGet, "/internal/users/"+url.PathEscape(userID)+"/session-state", nil, &state)
	return state, err
}

// outboundRequestID forwards the inbound request ID when one is on the context
// and generates a fresh one only for a call with no inbound request.
func outboundRequestID(ctx context.Context) string {
	if id := requestid.FromContext(ctx); id != "" {
		return id
	}
	return requestid.New()
}

func (c *Client) do(ctx context.Context, method, path string, body, target any) error {
	var payload []byte
	var err error
	if body != nil {
		payload, err = json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode identity request: %w", err)
		}
	}
	request, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bytes.NewReader(payload))
	if err != nil {
		return ErrUnavailable
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	request.Header.Set("X-Internal-Token", c.token)
	request.Header.Set(requestid.Header, outboundRequestID(ctx))
	response, err := c.http.Do(request)
	if err != nil {
		return ErrUnavailable
	}
	defer response.Body.Close()
	switch response.StatusCode {
	case http.StatusOK, http.StatusCreated, http.StatusNoContent:
		if target != nil {
			var envelope struct {
				Data json.RawMessage `json:"data"`
			}
			if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
				return fmt.Errorf("decode identity response: %w", ErrUnavailable)
			}
			if len(envelope.Data) == 0 {
				return fmt.Errorf("decode identity response: %w", ErrUnavailable)
			}
			if err := json.Unmarshal(envelope.Data, target); err != nil {
				return fmt.Errorf("decode identity response: %w", ErrUnavailable)
			}
		}
		return nil
	case http.StatusConflict:
		var envelope struct {
			Error struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		_ = json.NewDecoder(response.Body).Decode(&envelope)
		switch envelope.Error.Code {
		case "EMAIL_ALREADY_EXISTS":
			return ErrEmailTaken
		case "TRANSITION_ID_CONFLICT":
			return ErrTransitionConflict
		case "STALE_TRANSITION":
			return ErrTransitionStale
		default:
			return ErrTransitionConflict
		}
	case http.StatusNotFound:
		return ErrNotFound
	default:
		return ErrUnavailable
	}
}
