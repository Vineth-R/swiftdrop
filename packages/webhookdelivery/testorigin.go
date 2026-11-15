package webhookdelivery

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Vineth-R/swiftdrop/packages/go-common/webhookurl"
)

// NormalizeTestOrigin validates the explicitly configured test-only webhook
// origin. It is used only by development and real-stack tests; production
// configuration must leave WEBHOOK_TEST_ORIGIN empty. The origin is an HTTP(S)
// origin without credentials, query, fragment or path.
func NormalizeTestOrigin(raw string) (string, error) {
	raw = strings.TrimRight(strings.TrimSpace(raw), "/")
	if raw == "" {
		return "", errors.New("test webhook origin is required")
	}
	u, err := url.Parse(raw)
	if err != nil || !u.IsAbs() || u.Host == "" || u.User != nil ||
		u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return "", errors.New("test webhook origin must be an absolute HTTP(S) origin")
	}
	if !strings.EqualFold(u.Scheme, "http") && !strings.EqualFold(u.Scheme, "https") {
		return "", errors.New("test webhook origin must use http or https")
	}
	return strings.ToLower(u.Scheme) + "://" + strings.ToLower(u.Host), nil
}

// MatchesTestOrigin reports whether raw is under the explicitly configured test
// origin. It is only a route for the test origin; every other URL still goes
// through the production webhook validation and dial-time SSRF checks.
func MatchesTestOrigin(raw, origin string) bool {
	target, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || !target.IsAbs() || target.User != nil {
		return false
	}
	base, err := url.Parse(origin)
	if err != nil {
		return false
	}
	if !strings.EqualFold(target.Scheme, base.Scheme) ||
		!strings.EqualFold(target.Host, base.Host) {
		return false
	}
	basePath := strings.TrimRight(base.Path, "/")
	return basePath == "" || target.Path == basePath || strings.HasPrefix(target.Path, basePath+"/")
}

// NewTestOnlySender returns the production sender with one explicitly
// configured test origin allowed over HTTP. It must only be called when the
// service is running in development with a test receiver on the Compose
// network; all other destinations keep the normal HTTPS, DNS and dial-time SSRF
// checks.
func NewTestOnlySender(rawOrigin string) (*Sender, error) {
	origin, err := NormalizeTestOrigin(rawOrigin)
	if err != nil {
		return nil, err
	}
	base := NewSender()
	transport, ok := base.client.Transport.(*http.Transport)
	if !ok {
		return nil, errors.New("test webhook sender requires the production transport")
	}
	testOrigin, err := url.Parse(origin)
	if err != nil {
		return nil, err
	}
	baseDialer := &net.Dialer{Timeout: 5 * time.Second, Control: webhookurl.DialControl}
	testDialer := &net.Dialer{Timeout: 5 * time.Second}
	originPort := testOrigin.Port()
	if originPort == "" {
		if strings.EqualFold(testOrigin.Scheme, "https") {
			originPort = "443"
		} else {
			originPort = "80"
		}
	}
	clone := transport.Clone()
	clone.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	clone.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, splitErr := net.SplitHostPort(address)
		if splitErr == nil && strings.EqualFold(host, testOrigin.Hostname()) && port == originPort {
			return testDialer.DialContext(ctx, network, address)
		}
		return baseDialer.DialContext(ctx, network, address)
	}
	// The controlled test receiver uses a self-signed certificate. Only its
	// exact origin skips verification; every other HTTPS target uses the
	// normal verifying TLS dialer.
	clone.DialTLSContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, splitErr := net.SplitHostPort(address)
		if splitErr == nil && strings.EqualFold(host, testOrigin.Hostname()) && port == originPort {
			conn, err := testDialer.DialContext(ctx, network, address)
			if err != nil {
				return nil, err
			}
			tlsConn := tls.Client(conn, &tls.Config{
				MinVersion:         tls.VersionTLS12,
				ServerName:         host,
				InsecureSkipVerify: true,
			})
			if err := tlsConn.HandshakeContext(ctx); err != nil {
				_ = conn.Close()
				return nil, err
			}
			return tlsConn, nil
		}
		dialer := &tls.Dialer{NetDialer: baseDialer, Config: clone.TLSClientConfig}
		return dialer.DialContext(ctx, network, address)
	}
	base.client = &http.Client{
		Timeout:       base.client.Timeout,
		Transport:     clone,
		CheckRedirect: base.client.CheckRedirect,
	}
	base.checkURL = func(raw string) error {
		if MatchesTestOrigin(raw, origin) {
			return nil
		}
		return webhookurl.Validate(raw)
	}
	return base, nil
}
