package webhookurl

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"strings"
	"testing"
)

func TestValidate(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want error // nil means accepted
	}{
		// Accepted.
		{"public https host", "https://shop.example.com/hooks/swiftdrop", nil},
		{"https with port and query", "https://shop.example.com:8443/hook?x=1", nil},
		{"uppercase scheme", "HTTPS://shop.example.com/hook", nil},
		{"public ipv4 literal", "https://93.184.216.34/hook", nil},
		{"public ipv6 literal", "https://[2606:2800:220:1:248:1893:25c8:1946]/hook", nil},
		{"surrounding spaces trimmed", "  https://shop.example.com/hook  ", nil},

		// Shape errors.
		{"empty", "", ErrEmpty},
		{"only spaces", "   ", ErrEmpty},
		{"too long", "https://shop.example.com/" + strings.Repeat("a", MaxLength), ErrTooLong},
		{"relative url", "/hooks/swiftdrop", ErrInvalid},
		{"no scheme", "shop.example.com/hook", ErrInvalid},
		{"no host", "https:///hook", ErrInvalid},
		{"opaque url", "https:shop.example.com", ErrInvalid},

		// Scheme.
		{"plain http", "http://shop.example.com/hook", ErrNotHTTPS},
		{"ftp", "ftp://shop.example.com/hook", ErrNotHTTPS},
		{"file", "file:///etc/passwd", ErrNotHTTPS},
		{"javascript (opaque url)", "javascript:alert(1)", ErrInvalid},

		// Credentials.
		{"user and password", "https://user:pass@shop.example.com/hook", ErrCredentials},
		{"user only", "https://user@shop.example.com/hook", ErrCredentials},
		// "https://shop.example.com@10.0.0.1/" really targets 10.0.0.1.
		{"userinfo trick", "https://shop.example.com@10.0.0.1/hook", ErrCredentials},

		// Blocked names.
		{"localhost", "https://localhost/hook", ErrBlockedHost},
		{"localhost mixed case", "https://LocalHost/hook", ErrBlockedHost},
		{"localhost trailing dot", "https://localhost./hook", ErrBlockedHost},
		{"subdomain of localhost", "https://api.localhost/hook", ErrBlockedHost},
		{"mdns local", "https://printer.local/hook", ErrBlockedHost},
		{"internal suffix", "https://db.internal/hook", ErrBlockedHost},
		{"docker service name postgres", "https://postgres/hook", ErrBlockedHost},
		{"docker service name merchant", "https://merchant:8080/hook", ErrBlockedHost},
		{"ipv6 zone", "https://[fe80::1%25eth0]/hook", ErrBlockedHost},

		// Blocked IP literals.
		{"ipv4 loopback", "https://127.0.0.1/hook", ErrBlockedIP},
		{"ipv4 loopback range", "https://127.8.9.10/hook", ErrBlockedIP},
		{"ipv6 loopback", "https://[::1]/hook", ErrBlockedIP},
		{"10/8", "https://10.1.2.3/hook", ErrBlockedIP},
		{"172.16/12", "https://172.20.0.5/hook", ErrBlockedIP},
		{"192.168/16", "https://192.168.1.10/hook", ErrBlockedIP},
		{"cloud metadata", "https://169.254.169.254/latest/meta-data", ErrBlockedIP},
		{"link local ipv6", "https://[fe80::1]/hook", ErrBlockedIP},
		{"unique local ipv6", "https://[fd00::1]/hook", ErrBlockedIP},
		{"unspecified", "https://0.0.0.0/hook", ErrBlockedIP},
		{"unspecified ipv6", "https://[::]/hook", ErrBlockedIP},
		{"this network", "https://0.1.2.3/hook", ErrBlockedIP},
		{"cgnat", "https://100.64.0.1/hook", ErrBlockedIP},
		{"multicast", "https://224.0.0.1/hook", ErrBlockedIP},
		{"broadcast", "https://255.255.255.255/hook", ErrBlockedIP},
		{"documentation range", "https://203.0.113.9/hook", ErrBlockedIP},
		{"ipv4 mapped ipv6 private", "https://[::ffff:10.0.0.1]/hook", ErrBlockedIP},
		{"ipv4 mapped ipv6 loopback", "https://[::ffff:127.0.0.1]/hook", ErrBlockedIP},
		{"nat64 wrapping private", "https://[64:ff9b::a00:1]/hook", ErrBlockedIP},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Validate(tc.url)
			if !errors.Is(got, tc.want) && !(tc.want == nil && got == nil) {
				t.Fatalf("Validate(%q) = %v, want %v", tc.url, got, tc.want)
			}
		})
	}
}

// Errors must never echo the submitted URL, which may hold a secret token.
func TestErrorsDoNotLeakURL(t *testing.T) {
	const secret = "TOKEN-12345"
	for _, raw := range []string{
		"http://shop.example.com/hook?t=" + secret,
		"https://u:" + secret + "@shop.example.com/hook",
		"https://localhost/hook?t=" + secret,
	} {
		err := Validate(raw)
		if err == nil || strings.Contains(err.Error(), secret) {
			t.Fatalf("Validate(%q) error = %v; must fail without echoing the URL", raw, err)
		}
	}
}

func TestIsBlockedIP(t *testing.T) {
	blocked := []string{"127.0.0.1", "10.0.0.1", "172.31.255.255", "192.168.0.1",
		"169.254.169.254", "0.0.0.0", "100.100.100.100", "::1", "fe80::1", "fc00::1"}
	public := []string{"8.8.8.8", "1.1.1.1", "93.184.216.34", "172.32.0.1", "2606:4700:4700::1111"}
	for _, s := range blocked {
		if !IsBlockedIP(netip.MustParseAddr(s)) {
			t.Errorf("%s should be blocked", s)
		}
	}
	for _, s := range public {
		if IsBlockedIP(netip.MustParseAddr(s)) {
			t.Errorf("%s should be allowed", s)
		}
	}
	if !IsBlockedIP(netip.Addr{}) {
		t.Error("the zero Addr should be blocked")
	}
}

// fakeResolver returns fixed answers, so these tests never use the network.
type fakeResolver struct {
	addrs []netip.Addr
	err   error
}

func (f fakeResolver) LookupNetIP(context.Context, string, string) ([]netip.Addr, error) {
	return f.addrs, f.err
}

func TestValidateResolved(t *testing.T) {
	pub := netip.MustParseAddr("93.184.216.34")
	priv := netip.MustParseAddr("10.0.0.5")
	tests := []struct {
		name string
		url  string
		res  fakeResolver
		want error
	}{
		{"public answer", "https://shop.example.com/h", fakeResolver{addrs: []netip.Addr{pub}}, nil},
		{"private answer", "https://shop.example.com/h", fakeResolver{addrs: []netip.Addr{priv}}, ErrBlockedIP},
		{"one private among public", "https://shop.example.com/h", fakeResolver{addrs: []netip.Addr{pub, priv}}, ErrBlockedIP},
		{"lookup error", "https://shop.example.com/h", fakeResolver{err: errors.New("nxdomain")}, ErrNoAddresses},
		{"no answers", "https://shop.example.com/h", fakeResolver{}, ErrNoAddresses},
		{"syntax error wins", "http://shop.example.com/h", fakeResolver{addrs: []netip.Addr{pub}}, ErrNotHTTPS},
		{"ip literal skips lookup", "https://93.184.216.34/h", fakeResolver{err: errors.New("must not be called")}, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ValidateResolved(context.Background(), tc.url, tc.res)
			if !errors.Is(got, tc.want) && !(tc.want == nil && got == nil) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestDialControl(t *testing.T) {
	for addr, wantErr := range map[string]bool{
		"93.184.216.34:443":     false,
		"[2606:4700::1111]:443": false,
		"127.0.0.1:443":         true,
		"10.0.0.1:443":          true,
		"169.254.169.254:80":    true,
		"[::1]:443":             true,
		"not-an-address":        true,
		"shop.example.com:443":  true, // Control receives IPs; a name here is refused
	} {
		err := DialControl("tcp", addr, nil)
		if (err != nil) != wantErr {
			t.Errorf("DialControl(%q) = %v, wantErr %v", addr, err, wantErr)
		}
	}
}

// DialControl must actually stop a real connection. This dials a throwaway
// listener on 127.0.0.1 and expects the dial itself to fail.
func TestDialControlBlocksRealDial(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skip("cannot listen on loopback:", err)
	}
	defer ln.Close()

	d := &net.Dialer{Control: DialControl}
	conn, err := d.DialContext(context.Background(), "tcp", ln.Addr().String())
	if err == nil {
		conn.Close()
		t.Fatal("dial to loopback succeeded; DialControl did not block it")
	}
	if !errors.Is(err, ErrBlockedIP) {
		t.Fatalf("dial error = %v, want ErrBlockedIP", err)
	}
}
