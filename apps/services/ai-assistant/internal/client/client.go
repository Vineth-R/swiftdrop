// Package client contains the small, read-only HTTP clients the AI assistant
// uses for the approved Pricing and Parcel tools. Each client validates its
// origin, bounds every response body, refuses redirects and converts failures
// into a typed error that never carries the upstream response body.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Vineth-R/swiftdrop/packages/go-common/requestid"
)

// maxResponseBytes bounds every upstream response. A larger body is treated as
// an unavailable upstream rather than being decoded.
const maxResponseBytes = 512 * 1024

// Config configures one upstream API client.
type Config struct {
	// BaseURL is the upstream origin, for example http://pricing:8083. It must
	// not contain user information, a query, a fragment or a path other than
	// "/".
	BaseURL string
	// Timeout bounds one request. A non-positive value defaults to 3s.
	Timeout time.Duration
}

type httpClient struct {
	baseURL string
	timeout time.Duration
	http    *http.Client
}

func newHTTPClient(cfg Config) (*httpClient, error) {
	baseURL, err := normalizeBaseURL(cfg.BaseURL)
	if err != nil {
		return nil, err
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	// Never use HTTP_PROXY/HTTPS_PROXY for internal service calls: private
	// tool requests carry the caller's Bearer token.
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	return &httpClient{
		baseURL: baseURL,
		timeout: timeout,
		http: &http.Client{
			Timeout:   timeout,
			Transport: transport,
			// Never follow an upstream redirect: a redirect could point at an
			// arbitrary host. The 3xx response surfaces as a safe failure.
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}, nil
}

func normalizeBaseURL(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", errors.New("base URL is required")
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return "", errors.New("base URL must be an absolute http or https origin")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", errors.New("base URL must be an absolute http or https origin")
	}
	if parsed.Host == "" || parsed.Opaque != "" {
		return "", errors.New("base URL must be an absolute http or https origin")
	}
	if parsed.User != nil {
		return "", errors.New("base URL must not contain user information")
	}
	if parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" {
		return "", errors.New("base URL must not contain a query or fragment")
	}
	if parsed.Path != "" && parsed.Path != "/" {
		return "", errors.New("base URL must not contain a path")
	}
	return strings.TrimRight(parsed.String(), "/"), nil
}

// do performs one request and decodes a JSON response into out. Request IDs
// already present on the context are forwarded; the model can never supply
// one. No retry is attempted because orchestration must not multiply calls.
func (c *httpClient) do(ctx context.Context, method, path string, query url.Values, payload any, bearer string, out any) error {
	if c == nil {
		return &Error{Kind: KindUnavailable}
	}

	requestCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	endpoint := c.baseURL + path
	if len(query) > 0 {
		if encoded := query.Encode(); encoded != "" {
			endpoint += "?" + encoded
		}
	}

	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return &Error{Kind: KindInvalid}
		}
		body = bytes.NewReader(encoded)
	}

	request, err := http.NewRequestWithContext(requestCtx, method, endpoint, body)
	if err != nil {
		return &Error{Kind: KindUnavailable}
	}
	if payload != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if bearer != "" {
		request.Header.Set("Authorization", "Bearer "+bearer)
	}
	if id := requestid.FromContext(ctx); requestid.Valid(id) {
		request.Header.Set(requestid.Header, id)
	}

	response, err := c.http.Do(request)
	if err != nil {
		return &Error{Kind: KindUnavailable}
	}
	defer response.Body.Close()

	data, readErr := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if readErr != nil {
		return &Error{Kind: KindUnavailable, Status: response.StatusCode}
	}
	if len(data) > maxResponseBytes {
		return &Error{Kind: KindUnavailable, Status: response.StatusCode}
	}

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return &Error{
			Kind:   kindForStatus(response.StatusCode),
			Status: response.StatusCode,
			Code:   upstreamCode(data),
		}
	}

	if out == nil {
		return nil
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(out); err != nil {
		return &Error{Kind: KindUnavailable, Status: response.StatusCode}
	}
	return nil
}

func kindForStatus(status int) Kind {
	switch {
	case status == http.StatusUnauthorized, status == http.StatusForbidden:
		return KindUnauthorized
	case status == http.StatusNotFound:
		return KindNotFound
	case status == http.StatusBadRequest,
		status == http.StatusRequestEntityTooLarge,
		status == http.StatusUnprocessableEntity:
		return KindInvalid
	default:
		// 429, every 5xx and any unexpected status fail closed as unavailable.
		return KindUnavailable
	}
}

func upstreamCode(data []byte) string {
	var envelope struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return ""
	}
	return strings.TrimSpace(envelope.Error.Code)
}
