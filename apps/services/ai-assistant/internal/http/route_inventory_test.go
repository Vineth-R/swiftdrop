package servicehttp

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/gin-gonic/gin"

	aiservice "github.com/Vineth-R/swiftdrop/apps/services/ai-assistant/internal/service"
	"github.com/Vineth-R/swiftdrop/packages/go-common/routecontract"
)

type routeInventoryChat struct{}

func (routeInventoryChat) Chat(context.Context, aiservice.ChatInput) (aiservice.ChatOutput, error) {
	return aiservice.ChatOutput{}, nil
}

func TestRouteInventory(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router, err := New(
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		Options{Service: routeInventoryChat{}},
	)
	if err != nil {
		t.Fatalf("New() = %v", err)
	}
	routecontract.Verify(t, router, routecontract.Policy{Service: "ai-assistant"})
}
