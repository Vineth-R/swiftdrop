package httpx

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestInternalTokenFailsClosed(t *testing.T) {
	for _, test := range []struct {
		name, secret, provided string
		status                 int
	}{
		{name: "missing configuration", status: 401}, {name: "wrong secret", secret: "internal", provided: "wrong", status: 401},
		{name: "valid", secret: "internal", provided: "internal", status: 204},
	} {
		t.Run(test.name, func(t *testing.T) {
			router := gin.New()
			router.GET("/internal", InternalToken(test.secret), func(c *gin.Context) { c.Status(204) })
			request := httptest.NewRequest("GET", "/internal", nil)
			request.Header.Set("X-Internal-Token", test.provided)
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)
			if recorder.Code != test.status {
				t.Fatalf("status %d want %d", recorder.Code, test.status)
			}
		})
	}
}
