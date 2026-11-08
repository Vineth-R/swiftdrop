package servicehttp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Vineth-R/swiftdrop/apps/services/ai-assistant/internal/middleware"
	"github.com/Vineth-R/swiftdrop/apps/services/ai-assistant/internal/service"
	"github.com/Vineth-R/swiftdrop/packages/go-common/auth"
	"github.com/Vineth-R/swiftdrop/packages/go-common/httpx"
	"github.com/Vineth-R/swiftdrop/packages/go-common/requestid"
)

// ChatService is the orchestration seam used by the HTTP handler.
type ChatService interface {
	Chat(ctx context.Context, input service.ChatInput) (service.ChatOutput, error)
}

// Options configures the ai-assistant router.
type Options struct {
	Service           ChatService
	Limiter           *middleware.Limiter
	Verifier          auth.Verifier
	CookieName        string
	GuestCookieName   string
	GuestCookieSecure bool
	GuestSessionTTL   time.Duration
	FrontendOrigin    string
	TrustedProxies    []string
	ProviderStatus    string
}

type handler struct {
	service ChatService
	logger  *slog.Logger
}

type chatRequest struct {
	Message        string  `json:"message"`
	ConversationID *string `json:"conversationId"`
}

// New builds the ai-assistant Gin router. The gateway strips the /api/v1
// prefix, so the chat route is registered as /ai/chat.
func New(logger *slog.Logger, options Options) (*gin.Engine, error) {
	if options.Service == nil {
		return nil, errors.New("chat service is required")
	}
	router := httpx.NewRouter(logger)
	httpx.RegisterHealth(router)
	if err := router.SetTrustedProxies(options.TrustedProxies); err != nil {
		return nil, err
	}
	router.RemoteIPHeaders = []string{"X-Real-IP"}

	status := options.ProviderStatus
	if status == "" {
		status = "unconfigured"
	}
	router.GET("/health", func(c *gin.Context) {
		httpx.JSON(c, http.StatusOK, gin.H{"status": "ok"})
	})
	router.GET("/ready", func(c *gin.Context) {
		httpx.JSON(c, http.StatusOK, gin.H{"status": "ready", "checks": gin.H{"provider": status}})
	})

	h := &handler{service: options.Service, logger: logger}
	group := router.Group("/ai")
	group.Use(middleware.OptionalAuth(options.Verifier, options.CookieName))
	group.Use(middleware.OriginGuard(options.FrontendOrigin))
	group.Use(middleware.GuestSession(options.GuestCookieName, options.GuestCookieSecure, options.GuestSessionTTL))
	if options.Limiter != nil {
		group.Use(options.Limiter.Middleware())
	}
	group.POST("/chat", h.chat)
	return router, nil
}

func (h *handler) chat(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	var request chatRequest
	if err := decodeChatBody(c, &request); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			httpx.Error(c, http.StatusRequestEntityTooLarge, "PAYLOAD_TOO_LARGE", "request is too large")
			return
		}
		httpx.Error(c, http.StatusBadRequest, "BAD_REQUEST", "request must contain a single valid JSON object")
		return
	}

	conversationID := ""
	if request.ConversationID != nil {
		conversationID = *request.ConversationID
		if conversationID == "" {
			httpx.ErrorWithDetails(c, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "one or more fields are invalid",
				[]httpx.ErrorItem{{Field: "conversationId", Issue: "must be a valid conversation ID when provided"}})
			return
		}
	}

	output, err := h.service.Chat(c.Request.Context(), service.ChatInput{
		Message:        request.Message,
		ConversationID: conversationID,
		Caller:         middleware.CallerFrom(c),
	})
	if err != nil {
		h.failure(c, err)
		return
	}

	httpx.Success(c, http.StatusOK, gin.H{
		"conversationId": output.ConversationID,
		"reply":          output.Reply,
		"toolCalls":      output.ToolCalls,
	})
}

func (h *handler) failure(c *gin.Context, err error) {
	var invalid *service.ValidationError
	switch {
	case errors.As(err, &invalid):
		httpx.ErrorWithDetails(c, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "one or more fields are invalid",
			[]httpx.ErrorItem{{Field: invalid.Field, Issue: invalid.Issue}})
	case errors.Is(err, service.ErrConversationNotFound):
		httpx.Error(c, http.StatusNotFound, "CONVERSATION_NOT_FOUND", "conversation not found")
	case errors.Is(err, service.ErrAIUnavailable):
		httpx.Error(c, http.StatusServiceUnavailable, "AI_UNAVAILABLE", "The assistant is temporarily unavailable. Please try again later.")
	case errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded):
		// The caller is gone; there is no useful response to write.
		c.Abort()
	default:
		h.logger.ErrorContext(c.Request.Context(), "ai chat failed",
			"request_id", requestid.FromGin(c),
			"error", err,
		)
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "the assistant could not complete the request")
	}
}

func decodeChatBody(c *gin.Context, value any) error {
	mediaType, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || !strings.EqualFold(mediaType, "application/json") {
		return errors.New("request must be JSON")
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1<<20)
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("request must contain one JSON object")
	}
	return nil
}
