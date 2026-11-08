package servicehttp

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Vineth-R/swiftdrop/apps/services/ai-assistant/internal/middleware"
	"github.com/Vineth-R/swiftdrop/apps/services/ai-assistant/internal/service"
	"github.com/Vineth-R/swiftdrop/packages/go-common/auth"
	"github.com/Vineth-R/swiftdrop/packages/go-common/logging"
)

func init() {
	gin.SetMode(gin.TestMode)
}

type fakeChatService struct {
	output service.ChatOutput
	err    error
	input  service.ChatInput
	calls  int
}

func (f *fakeChatService) Chat(_ context.Context, input service.ChatInput) (service.ChatOutput, error) {
	f.calls++
	f.input = input
	return f.output, f.err
}

func testLogger(t *testing.T) *slog.Logger {
	t.Helper()
	logger, err := logging.New("ai-assistant-test", "development", "error")
	if err != nil {
		t.Fatalf("logging.New: %v", err)
	}
	return logger
}

func testRouter(t *testing.T, chat ChatService, limiter *middleware.Limiter, verifier auth.Verifier) *gin.Engine {
	t.Helper()
	router, err := New(testLogger(t), Options{
		Service:        chat,
		Limiter:        limiter,
		Verifier:       verifier,
		CookieName:     "sd_access",
		ProviderStatus: "ok",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return router
}

func newChatRequest(body string) *http.Request {
	request := httptest.NewRequest(http.MethodPost, "/ai/chat", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	return request
}

func TestHealthAndReady(t *testing.T) {
	router := testRouter(t, &fakeChatService{}, nil, nil)
	for _, path := range []string{"/health", "/ready", "/healthz", "/readyz"} {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		if recorder.Code != 200 {
			t.Fatalf("%s status = %d", path, recorder.Code)
		}
	}
}

func TestChatAcceptsUppercaseContentType(t *testing.T) {
	chat := &fakeChatService{output: service.ChatOutput{ConversationID: "cv00000000000000000000000000000000", Reply: "ok"}}
	router := testRouter(t, chat, nil, nil)
	request := newChatRequest(`{"message":"hi"}`)
	request.Header.Set("Content-Type", "Application/JSON")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
	}
	if recorder.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", recorder.Header().Get("Cache-Control"))
	}
}

func TestChatSuccessEnvelope(t *testing.T) {
	chat := &fakeChatService{output: service.ChatOutput{
		ConversationID: "cv00000000000000000000000000000000",
		Reply:          "Your parcel is in transit.",
		ToolCalls:      []service.ToolCallSummary{{Name: "get_public_parcel_status", Status: "OK"}},
	}}
	router := testRouter(t, chat, nil, nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, newChatRequest(`{"message":"Where is my parcel?","conversationId":null}`))

	if recorder.Code != 200 {
		t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
	}
	var payload struct {
		Data struct {
			ConversationID string                    `json:"conversationId"`
			Reply          string                    `json:"reply"`
			ToolCalls      []service.ToolCallSummary `json:"toolCalls"`
		} `json:"data"`
		Meta struct {
			RequestID string `json:"requestId"`
			Timestamp string `json:"timestamp"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if payload.Data.ConversationID != chat.output.ConversationID || payload.Data.Reply != chat.output.Reply {
		t.Fatalf("payload = %+v", payload.Data)
	}
	if len(payload.Data.ToolCalls) != 1 || payload.Data.ToolCalls[0].Status != "OK" {
		t.Fatalf("toolCalls = %+v", payload.Data.ToolCalls)
	}
	if payload.Meta.RequestID == "" || payload.Meta.Timestamp == "" {
		t.Fatalf("meta = %+v", payload.Meta)
	}
}

func TestChatRejectsMalformedInput(t *testing.T) {
	chat := &fakeChatService{output: service.ChatOutput{ConversationID: "cv00000000000000000000000000000000", Reply: "ok"}}
	router := testRouter(t, chat, nil, nil)

	cases := map[string]*http.Request{
		"malformed":   newChatRequest(`{"message":`),
		"unknown":     newChatRequest(`{"message":"hi","extra":true}`),
		"trailing":    newChatRequest(`{"message":"hi"}{"message":"hi"}`),
		"wrong type":  newChatRequest(`{"message":42}`),
		"wrong media": httptest.NewRequest(http.MethodPost, "/ai/chat", strings.NewReader(`{"message":"hi"}`)),
		"too large":   newChatRequest(`{"message":"` + strings.Repeat("a", 1<<20) + `"}`),
	}
	for name, request := range cases {
		t.Run(name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)
			if recorder.Code != http.StatusBadRequest && recorder.Code != http.StatusRequestEntityTooLarge {
				t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
			}
		})
	}
	if chat.calls != 0 {
		t.Fatalf("service calls = %d, want 0", chat.calls)
	}
}

func TestChatValidationError(t *testing.T) {
	chat := &fakeChatService{err: &service.ValidationError{Field: "message", Issue: "must not be empty"}}
	router := testRouter(t, chat, nil, nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, newChatRequest(`{"message":""}`))

	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), `"field":"message"`) {
		t.Fatalf("body = %s", recorder.Body.String())
	}
}

func TestChatRejectsEmptyConversationID(t *testing.T) {
	router := testRouter(t, &fakeChatService{}, nil, nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, newChatRequest(`{"message":"hi","conversationId":""}`))
	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", recorder.Code)
	}
}

func TestChatErrorMapping(t *testing.T) {
	cases := map[error]int{
		service.ErrConversationNotFound: http.StatusNotFound,
		service.ErrAIUnavailable:        http.StatusServiceUnavailable,
		errors.New("boom"):              http.StatusInternalServerError,
	}
	for err, status := range cases {
		chat := &fakeChatService{err: err}
		router := testRouter(t, chat, nil, nil)
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, newChatRequest(`{"message":"hi"}`))
		if recorder.Code != status {
			t.Fatalf("err %v: status = %d, want %d", err, recorder.Code, status)
		}
	}
}

func TestChatRateLimit(t *testing.T) {
	chat := &fakeChatService{output: service.ChatOutput{ConversationID: "cv00000000000000000000000000000000", Reply: "ok"}}
	router := testRouter(t, chat, middleware.NewLimiter(1, time.Minute), nil)

	first := httptest.NewRecorder()
	router.ServeHTTP(first, newChatRequest(`{"message":"hi"}`))
	if first.Code != 200 {
		t.Fatalf("first status = %d", first.Code)
	}

	second := httptest.NewRecorder()
	router.ServeHTTP(second, newChatRequest(`{"message":"hi"}`))
	if second.Code != http.StatusTooManyRequests {
		t.Fatalf("second status = %d, want 429", second.Code)
	}
	if second.Header().Get("Retry-After") == "" {
		t.Fatal("missing Retry-After")
	}
	if !strings.Contains(second.Body.String(), `"code":"RATE_LIMITED"`) {
		t.Fatalf("body = %s", second.Body.String())
	}
}
