package servicehttp

import (
	"io"
	"log/slog"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/Vineth-R/swiftdrop/packages/go-common/routecontract"
)

func TestRouteInventory(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router, err := New(slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("New() = %v", err)
	}
	routecontract.Verify(t, router, routecontract.Policy{Service: "notification-worker"})
}
