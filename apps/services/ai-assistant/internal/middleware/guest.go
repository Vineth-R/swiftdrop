package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Vineth-R/swiftdrop/apps/services/ai-assistant/internal/conversation"
	"github.com/Vineth-R/swiftdrop/packages/go-common/httpx"
)

const (
	defaultGuestCookieName = "sd_ai_guest"
	defaultGuestSessionTTL = 30 * time.Minute
	guestSessionPrefix     = "gs"
)

// GuestSession gives every anonymous caller a server-generated, HttpOnly
// browser session. Anonymous conversations are bound to this identifier, so a
// conversation ID alone never grants access. Authenticated requests clear the
// guest cookie; guest history is never merged into an account and a later
// logout starts a new guest session.
func GuestSession(cookieName string, secure bool, ttl time.Duration) gin.HandlerFunc {
	if cookieName == "" {
		cookieName = defaultGuestCookieName
	}
	if ttl <= 0 {
		ttl = defaultGuestSessionTTL
	}

	return func(c *gin.Context) {
		c.Writer.Header().Add("Vary", "Cookie")

		caller := CallerFrom(c)
		if caller.Authenticated {
			if _, err := c.Cookie(cookieName); err == nil {
				clearGuestCookie(c, cookieName, secure)
			}
			c.Next()
			return
		}

		sessionID := ""
		if cookie, err := c.Cookie(cookieName); err == nil && conversation.ValidGuestSessionID(cookie) {
			sessionID = cookie
		}
		if sessionID == "" {
			generated, err := newGuestSessionID()
			if err != nil {
				httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "the assistant could not start a session")
				return
			}
			sessionID = generated
		}

		setGuestCookie(c, cookieName, sessionID, ttl, secure)
		caller.SessionID = sessionID
		c.Set(callerContextKey, caller)
		c.Next()
	}
}

func newGuestSessionID() (string, error) {
	var buffer [16]byte
	if _, err := rand.Read(buffer[:]); err != nil {
		return "", err
	}
	return guestSessionPrefix + hex.EncodeToString(buffer[:]), nil
}

func setGuestCookie(c *gin.Context, name, value string, ttl time.Duration, secure bool) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		MaxAge:   int(ttl.Seconds()),
		Expires:  time.Now().Add(ttl),
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}

func clearGuestCookie(c *gin.Context, name string, secure bool) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     name,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		Expires:  time.Unix(1, 0),
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}
