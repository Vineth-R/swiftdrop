package httpx

import (
	"errors"
	"net"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// CrossOriginMiddleware is a defence-in-depth CSRF check for state-changing
// requests. It does not replace SameSite cookies: it rejects the requests that
// a browser marks as cross-site and, when the gateway forwarded the original
// host, requires the Origin header to name that same host and scheme.
//
// Non-browser clients (API keys, service-to-service calls) send neither header
// and are unaffected. The Origin comparison only runs when the request came
// through the public gateway, signalled by X-Real-IP (Nginx always sets it).
// A direct local service call (the Next.js development proxy) is not mistaken
// for the gateway, while the browser's Sec-Fetch-Site signal still applies.
func CrossOriginMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if isSafeMethod(c.Request.Method) {
			c.Next()
			return
		}

		if strings.EqualFold(c.GetHeader("Sec-Fetch-Site"), "cross-site") {
			Error(c, http.StatusForbidden, "CSRF_ORIGIN_MISMATCH", "cross-site state-changing requests are not allowed")
			return
		}

		origin := strings.TrimSpace(c.GetHeader("Origin"))
		forwardedHost := firstHeaderValue(c.GetHeader("X-Forwarded-Host"))
		if origin == "" || forwardedHost == "" || strings.TrimSpace(c.GetHeader("X-Real-IP")) == "" {
			c.Next()
			return
		}
		if origin == "null" || !originMatches(origin, forwardedHost, firstHeaderValue(c.GetHeader("X-Forwarded-Proto")), c.Request.TLS != nil) {
			Error(c, http.StatusForbidden, "CSRF_ORIGIN_MISMATCH", "cross-site state-changing requests are not allowed")
			return
		}
		c.Next()
	}
}

func isSafeMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	default:
		return false
	}
}

func firstHeaderValue(value string) string {
	if index := strings.IndexByte(value, ','); index >= 0 {
		value = value[:index]
	}
	return strings.TrimSpace(value)
}

// originMatches compares an Origin URL with the forwarded host and scheme. The
// host comparison is case-insensitive; the scheme falls back to the request TLS
// state when the proxy did not supply one.
//
// Two loopback hosts are accepted even when their ports differ. That is the
// local development path (the Next.js dev server on :3000 proxying to the
// gateway on :8080); a remote attacker cannot make a victim's browser send a
// loopback Origin to a public site, so this does not weaken production.
func originMatches(origin, host, scheme string, tls bool) bool {
	parsed, err := parseOrigin(origin)
	if err != nil {
		return false
	}
	if loopbackHost(parsed.host) && loopbackHost(host) {
		return true
	}
	if scheme == "" {
		if tls {
			scheme = "https"
		} else {
			scheme = "http"
		}
	}
	return strings.EqualFold(parsed.scheme, scheme) && strings.EqualFold(parsed.host, host)
}

func loopbackHost(host string) bool {
	name := host
	if parsedHost, _, err := net.SplitHostPort(host); err == nil {
		name = parsedHost
	}
	name = strings.Trim(strings.ToLower(name), "[]")
	switch name {
	case "localhost", "127.0.0.1", "::1":
		return true
	default:
		return false
	}
}

type originParts struct {
	scheme string
	host   string
}

func parseOrigin(origin string) (originParts, error) {
	scheme, rest, found := strings.Cut(origin, "://")
	if !found || scheme == "" || rest == "" {
		return originParts{}, errors.New("invalid origin")
	}
	if strings.ContainsAny(rest, "/?#") {
		return originParts{}, errors.New("invalid origin")
	}
	return originParts{scheme: scheme, host: rest}, nil
}
