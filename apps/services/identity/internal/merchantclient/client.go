// Package merchantclient reads a merchant's current application status from
// the Merchant service's internal API. Identity uses it during merchant login
// and refresh so a failed deactivation cannot permit a stale session.
package merchantclient

import (
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

var ErrUnavailable = errors.New("merchant service is unavailable")

type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

type approvalResponse struct {
	Data struct {
		Status string `json:"status"`
		// Reason is the administrator's text for a REJECTED or SUSPENDED
		// application. It is empty for the other statuses.
		Reason string `json:"reason"`
	} `json:"data"`
}

func New(baseURL, token string, timeout time.Duration) (*Client, error) {
	trimmed := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, errors.New("MERCHANT_BASE_URL must be an absolute http or https URL")
	}
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	return &Client{baseURL: trimmed, token: strings.TrimSpace(token), http: &http.Client{Timeout: timeout}}, nil
}

// Approval returns the merchant status of an owner and, for a REJECTED or
// SUSPENDED application, the administrator's reason. An owner with no merchant
// record has an empty status. The reason is personal to the owner: the caller
// shows it only after a correct password and never logs it.
func (c *Client) Approval(ctx context.Context, userID string) (status, reason string, err error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/internal/merchants/owners/"+url.PathEscape(userID)+"/approval", nil)
	if err != nil {
		return "", "", ErrUnavailable
	}
	request.Header.Set("X-Internal-Token", c.token)
	request.Header.Set(requestid.Header, requestid.New())
	response, err := c.http.Do(request)
	if err != nil {
		return "", "", ErrUnavailable
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotFound {
		return "", "", nil
	}
	if response.StatusCode != http.StatusOK {
		return "", "", ErrUnavailable
	}
	var body approvalResponse
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		return "", "", fmt.Errorf("decode merchant approval: %w", ErrUnavailable)
	}
	return body.Data.Status, body.Data.Reason, nil
}
