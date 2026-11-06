// Package webhookurl decides whether a merchant-supplied webhook URL is safe
// to call. SwiftDrop's webhook worker POSTs to a URL the merchant controls, so
// without these checks a merchant could point it at the platform's own
// internal services (Postgres, RabbitMQ, the internal APIs) or at a cloud
// metadata address (server-side request forgery, SSRF).
//
// The rules (API v1.1, section 8.13 and 10):
//   - the scheme must be https;
//   - the host must be a public address: no localhost, private, loopback,
//     link-local or other reserved ranges, and no single-label names such as
//     "postgres" that only resolve inside the Docker network;
//   - the URL carries no credentials.
//
// Validate is a cheap check on the text and is used when a merchant saves a
// URL. ValidateResolved additionally resolves the name. DialControl repeats
// the IP check at connection time, so a name that later resolves to an
// internal address (DNS rebinding) is still refused.
package webhookurl

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"strings"
	"syscall"
)

// MaxLength is the longest URL accepted. It matches common browser and proxy
// limits and keeps stored values bounded.
const MaxLength = 2048

// Sentinel errors let callers map a failure to a stable API error code without
// parsing messages. Messages never contain the submitted URL.
var (
	ErrEmpty         = errors.New("webhook url is required")
	ErrTooLong       = errors.New("webhook url is too long")
	ErrInvalid       = errors.New("webhook url is not a valid absolute url")
	ErrNotHTTPS      = errors.New("webhook url must use https")
	ErrCredentials   = errors.New("webhook url must not contain credentials")
	ErrBlockedHost   = errors.New("webhook url host is not allowed")
	ErrBlockedIP     = errors.New("webhook url resolves to a non-public address")
	ErrNoAddresses   = errors.New("webhook url host did not resolve")
	blockedHostNames = []string{"localhost"}
	// Suffixes that only exist on private networks.
	blockedSuffixes = []string{".localhost", ".local", ".internal", ".localdomain", ".home.arpa"}
)

// blockedPrefixes lists ranges that netip's own helpers do not already cover
// (IsPrivate, IsLoopback, IsLinkLocalUnicast, IsMulticast, IsUnspecified).
var blockedPrefixes = mustPrefixes(
	"0.0.0.0/8",       // "this network"
	"100.64.0.0/10",   // carrier-grade NAT
	"192.0.0.0/24",    // IETF protocol assignments
	"192.0.2.0/24",    // documentation (TEST-NET-1)
	"198.18.0.0/15",   // benchmarking
	"198.51.100.0/24", // documentation (TEST-NET-2)
	"203.0.113.0/24",  // documentation (TEST-NET-3)
	"240.0.0.0/4",     // reserved, includes 255.255.255.255
	"64:ff9b::/96",    // NAT64 (can wrap an internal IPv4 address)
	"2001:db8::/32",   // documentation
)

func mustPrefixes(cidrs ...string) []netip.Prefix {
	out := make([]netip.Prefix, 0, len(cidrs))
	for _, c := range cidrs {
		out = append(out, netip.MustParsePrefix(c))
	}
	return out
}

// Validate checks the URL text only. It does not touch the network, so it is
// safe to call on every save and in unit tests.
func Validate(raw string) error {
	_, err := parse(raw)
	return err
}

// parse validates the text and returns the parsed URL.
func parse(raw string) (*url.URL, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, ErrEmpty
	}
	if len(raw) > MaxLength {
		return nil, ErrTooLong
	}
	u, err := url.Parse(raw)
	if err != nil || !u.IsAbs() || u.Opaque != "" {
		return nil, ErrInvalid
	}
	if !strings.EqualFold(u.Scheme, "https") {
		return nil, ErrNotHTTPS
	}
	if u.User != nil {
		return nil, ErrCredentials
	}
	host := u.Hostname()
	if host == "" {
		return nil, ErrInvalid
	}
	if err := checkHost(host); err != nil {
		return nil, err
	}
	return u, nil
}

// checkHost applies the rules that need no DNS lookup.
func checkHost(host string) error {
	// Strip an IPv6 zone ("fe80::1%eth0") before parsing; zones are never
	// legitimate in a public webhook URL.
	if strings.Contains(host, "%") {
		return ErrBlockedHost
	}
	if addr, err := netip.ParseAddr(host); err == nil {
		if IsBlockedIP(addr) {
			return ErrBlockedIP
		}
		return nil
	}
	name := strings.ToLower(strings.TrimSuffix(host, "."))
	for _, b := range blockedHostNames {
		if name == b {
			return ErrBlockedHost
		}
	}
	for _, s := range blockedSuffixes {
		if strings.HasSuffix(name, s) {
			return ErrBlockedHost
		}
	}
	// A single label ("postgres", "rabbitmq", "merchant") resolves only on
	// the Docker network, so it can never be a real merchant server.
	if !strings.Contains(name, ".") {
		return ErrBlockedHost
	}
	return nil
}

// IsBlockedIP reports whether addr is not a public internet address.
func IsBlockedIP(addr netip.Addr) bool {
	// Treat ::ffff:10.0.0.1 exactly like 10.0.0.1.
	addr = addr.Unmap()
	if !addr.IsValid() ||
		addr.IsLoopback() ||
		addr.IsPrivate() ||
		addr.IsLinkLocalUnicast() || // includes 169.254.169.254 (cloud metadata)
		addr.IsLinkLocalMulticast() ||
		addr.IsInterfaceLocalMulticast() ||
		addr.IsMulticast() ||
		addr.IsUnspecified() {
		return true
	}
	for _, p := range blockedPrefixes {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}

// Resolver is the part of net.Resolver that ValidateResolved needs, so tests
// can supply fixed answers.
type Resolver interface {
	LookupNetIP(ctx context.Context, network, host string) ([]netip.Addr, error)
}

// ValidateResolved runs Validate and then resolves the host. Every address
// the name resolves to must be public: one internal answer is enough to refuse.
// A nil resolver uses net.DefaultResolver.
func ValidateResolved(ctx context.Context, raw string, r Resolver) error {
	u, err := parse(raw)
	if err != nil {
		return err
	}
	host := u.Hostname()
	if _, perr := netip.ParseAddr(host); perr == nil {
		return nil // an IP literal was already checked by parse
	}
	if r == nil {
		r = net.DefaultResolver
	}
	addrs, err := r.LookupNetIP(ctx, "ip", host)
	if err != nil || len(addrs) == 0 {
		return fmt.Errorf("%w", ErrNoAddresses)
	}
	for _, a := range addrs {
		if IsBlockedIP(a) {
			return ErrBlockedIP
		}
	}
	return nil
}

// DialControl is a net.Dialer.Control function. It runs after DNS resolution,
// just before the socket connects, and refuses any non-public address. Set it
// on the HTTP client the webhook worker uses:
//
//	dialer := &net.Dialer{Control: webhookurl.DialControl}
//
// This closes the gap between "checked when saved" and "called later": a
// merchant cannot save a harmless name and then repoint its DNS at an
// internal address.
func DialControl(_, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return ErrBlockedIP
	}
	addr, err := netip.ParseAddr(host)
	if err != nil || IsBlockedIP(addr) {
		return ErrBlockedIP
	}
	return nil
}
