package middleware

import (
	"net"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/Vineth-R/swiftdrop/packages/go-common/httpx"
)

// OriginGuard rejects browser requests whose Origin does not match the public
// web origin. Requests without an Origin header (CLI clients, older browsers and
// same-origin form posts that omit it) are allowed; the guest cookie is still
// HttpOnly and SameSite=Lax. This is a server-side defence in depth on top of
// the browser's CORS preflight and cookie policy.
func OriginGuard(allowedOrigin string) gin.HandlerFunc {
	allowed, ok := normalizeOrigin(allowedOrigin)
	return func(c *gin.Context) {
		origin := strings.TrimSpace(c.GetHeader("Origin"))
		if origin == "" {
			c.Next()
			return
		}
		presented, valid := normalizeOrigin(origin)
		if !ok || !valid || presented != allowed {
			httpx.Error(c, 403, "FORBIDDEN", "request origin is not allowed")
			return
		}
		c.Next()
	}
}

// normalizeOrigin returns a comparable scheme://host[:port] value. Default
// ports are removed so http://example.com and http://example.com:80 match.
func normalizeOrigin(raw string) (string, bool) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Host == "" || parsed.User != nil {
		return "", false
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", false
	}
	if parsed.Path != "" && parsed.Path != "/" {
		return "", false
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", false
	}
	host := strings.ToLower(parsed.Hostname())
	port := parsed.Port()
	if (scheme == "http" && port == "80") || (scheme == "https" && port == "443") {
		port = ""
	}
	if port == "" {
		return scheme + "://" + host, true
	}
	return scheme + "://" + net.JoinHostPort(host, port), true
}
