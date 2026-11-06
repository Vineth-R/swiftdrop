package delivery

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/Vineth-R/swiftdrop/packages/go-common/webhookurl"
)

// RequestTimeout is how long a merchant has to answer (API v1.1, section
// 10.2). It covers connecting, sending and reading the response.
const RequestTimeout = 10 * time.Second

// maxResponseDrain caps how much of a merchant's reply is read. The body is
// never stored or logged; it is only read so the connection can be reused.
const maxResponseDrain = 4 << 10

// Short, fixed failure descriptions stored in webhook_delivery_attempts
// .error_message (max 255 chars). They never include the URL, the secret or
// anything the merchant's server sent back.
const (
	errURLNotAllowed  = "webhook url is not allowed"
	errAddrNotAllowed = "address is not allowed"
	errTimeout        = "request timed out"
	errTLS            = "tls certificate not trusted"
	errConnect        = "could not connect"
	errRequest        = "could not build request"
)

// Target is where and how to send: the merchant's URL and signing secret.
type Target struct {
	URL    string
	Secret string
}

// Result is the outcome of one attempt, ready to be recorded.
type Result struct {
	// StatusCode is the HTTP status, or 0 when no response arrived (timeout,
	// connection failure). It maps to http_status NULL in that case.
	StatusCode int
	// Success is true only for a 2xx response within the timeout.
	Success bool
	// Duration is how long the attempt took.
	Duration time.Duration
	// ErrorMessage is empty on success, otherwise a short fixed description.
	ErrorMessage string
}

// Sender performs one signed POST. The zero value is not usable: use
// NewSender.
type Sender struct {
	// Client sends the request. NewSender returns a hardened client; tests
	// replace it to reach a local fake merchant.
	Client *http.Client
	// CheckURL is run before every request. It defaults to webhookurl.Validate.
	CheckURL func(string) error
	// Now supplies the signing timestamp. It defaults to time.Now.
	Now func() time.Time
}

// NewSender returns a Sender with the production safety settings:
//   - a 10 second overall timeout;
//   - webhookurl.DialControl on every connection, so addresses are checked
//     after DNS resolution (blocks DNS rebinding and redirects to internal
//     hosts);
//   - no HTTP proxy, because DialControl must see the merchant's real address;
//   - redirects are not followed: a 3xx counts as a failed delivery;
//   - TLS 1.2 or newer.
func NewSender() *Sender {
	dialer := &net.Dialer{
		Timeout: 5 * time.Second,
		Control: webhookurl.DialControl,
	}
	transport := &http.Transport{
		Proxy:               nil,
		DialContext:         dialer.DialContext,
		TLSClientConfig:     &tls.Config{MinVersion: tls.VersionTLS12},
		TLSHandshakeTimeout: 5 * time.Second,
		ForceAttemptHTTP2:   true,
		MaxIdleConns:        20,
		IdleConnTimeout:     30 * time.Second,
	}
	return &Sender{
		Client: &http.Client{
			Timeout:   RequestTimeout,
			Transport: transport,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		CheckURL: webhookurl.Validate,
		Now:      time.Now,
	}
}

// Send signs and POSTs the event to the target and reports the outcome. It
// never returns an error: every failure is a Result with Success false, so the
// caller can always record the attempt.
func (s *Sender) Send(ctx context.Context, target Target, event Event) Result {
	start := time.Now()
	fail := func(status int, msg string) Result {
		return Result{StatusCode: status, Duration: time.Since(start), ErrorMessage: msg}
	}

	if err := s.CheckURL(target.URL); err != nil {
		return fail(0, errURLNotAllowed)
	}

	body, err := event.Body()
	if err != nil {
		return fail(0, errRequest)
	}

	timestamp := s.Now().Unix()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target.URL, bytes.NewReader(body))
	if err != nil {
		return fail(0, errRequest)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "SwiftDrop-Webhooks/1")
	req.Header.Set("X-SwiftDrop-Event", event.Type)
	req.Header.Set("X-SwiftDrop-Event-Id", event.ID)
	req.Header.Set("X-SwiftDrop-Timestamp", strconv.FormatInt(timestamp, 10))
	req.Header.Set("X-SwiftDrop-Signature", Sign(target.Secret, timestamp, body))

	resp, err := s.Client.Do(req)
	if err != nil {
		return fail(0, classify(err))
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxResponseDrain))

	result := Result{StatusCode: resp.StatusCode, Duration: time.Since(start)}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		result.Success = true
		return result
	}
	result.ErrorMessage = fmt.Sprintf("unexpected http status %d", resp.StatusCode)
	return result
}

// classify turns a transport error into one of the fixed descriptions above.
// The raw error is never stored because it can contain the merchant's URL.
func classify(err error) string {
	var certErr *tls.CertificateVerificationError
	switch {
	case errors.Is(err, webhookurl.ErrBlockedIP):
		return errAddrNotAllowed
	case errors.Is(err, context.DeadlineExceeded) || isTimeout(err):
		return errTimeout
	case errors.As(err, &certErr):
		return errTLS
	default:
		return errConnect
	}
}

func isTimeout(err error) bool {
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}
