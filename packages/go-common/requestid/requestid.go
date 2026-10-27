package requestid

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	Header     = "X-Request-ID"
	contextKey = "request_id"
)

type contextKeyType struct{}

var contextKeyValue contextKeyType

func New() string {
	var buffer [16]byte
	if _, err := rand.Read(buffer[:]); err != nil {
		return timeFallback()
	}
	return hex.EncodeToString(buffer[:])
}

func Valid(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 128 {
		return false
	}
	for _, char := range value {
		switch {
		case char >= 'a' && char <= 'z':
		case char >= 'A' && char <= 'Z':
		case char >= '0' && char <= '9':
		case strings.ContainsRune("-._:", char):
		default:
			return false
		}
	}
	return true
}

func Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := strings.TrimSpace(c.GetHeader(Header))
		if !Valid(id) {
			id = New()
		}

		c.Header(Header, id)
		c.Set(contextKey, id)
		c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), contextKeyValue, id))
		c.Next()
	}
}

func FromGin(c *gin.Context) string {
	if value, ok := c.Get(contextKey); ok {
		if id, ok := value.(string); ok {
			return id
		}
	}
	return FromContext(c.Request.Context())
}

func FromContext(ctx context.Context) string {
	if id, ok := ctx.Value(contextKeyValue).(string); ok {
		return id
	}
	return ""
}

func timeFallback() string {
	return "rid-" + time.Now().UTC().Format("20060102T150405.000000000")
}
