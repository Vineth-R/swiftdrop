package service

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Vineth-R/swiftdrop/apps/services/ai-assistant/internal/contract"
	"github.com/Vineth-R/swiftdrop/apps/services/ai-assistant/internal/conversation"
	"github.com/Vineth-R/swiftdrop/apps/services/ai-assistant/internal/knowledge"
	"github.com/Vineth-R/swiftdrop/packages/go-common/auth"
	"github.com/Vineth-R/swiftdrop/packages/go-common/requestid"
)

// MaxMessageCharacters is the API v1.1 message limit.
const MaxMessageCharacters = 500

// RetrievalLimit is the maximum number of knowledge sections injected into the
// system instruction.
const RetrievalLimit = 3

var (
	// ErrAIUnavailable means the provider failed, timed out or is not configured.
	ErrAIUnavailable = errors.New("ai assistant unavailable")
	// ErrConversationNotFound covers unknown, expired and cross-owner IDs.
	ErrConversationNotFound = errors.New("conversation not found")
)

// ValidationError is a safe client-facing validation failure.
type ValidationError struct {
	Field string
	Issue string
}

func (e *ValidationError) Error() string {
	return e.Field + ": " + e.Issue
}

// ChatInput is one validated chat request.
type ChatInput struct {
	Message        string
	ConversationID string
	Caller         contract.CallerContext
}

// ToolCallSummary is the public, non-sensitive record of one executed tool.
type ToolCallSummary struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

// ChatOutput is the successful assistant response.
type ChatOutput struct {
	ConversationID string
	Reply          string
	ToolCalls      []ToolCallSummary
}

// Options configures the orchestrator.
type Options struct {
	Provider        contract.Provider
	Tools           contract.ToolRegistry
	Conversations   *conversation.Store
	Knowledge       *knowledge.Base
	Logger          *slog.Logger
	Model           string
	Temperature     float32
	MaxOutputTokens int32
	RequestTimeout  time.Duration
	ToolTimeout     time.Duration
	TotalTimeout    time.Duration
	MaxToolRounds   int
	MaxToolCalls    int
	MaxHistory      int
	Now             func() time.Time
}

// Orchestrator owns the controlled provider/tool loop.
type Orchestrator struct {
	provider        contract.Provider
	tools           contract.ToolRegistry
	conversations   *conversation.Store
	knowledge       *knowledge.Base
	logger          *slog.Logger
	model           string
	temperature     float32
	maxOutputTokens int32
	requestTimeout  time.Duration
	toolTimeout     time.Duration
	totalTimeout    time.Duration
	maxToolRounds   int
	maxToolCalls    int
	maxHistory      int
	now             func() time.Time
}

// New validates the orchestrator dependencies.
func New(options Options) (*Orchestrator, error) {
	if options.Tools == nil {
		return nil, errors.New("tool registry is required")
	}
	if options.Conversations == nil {
		return nil, errors.New("conversation store is required")
	}
	if options.RequestTimeout <= 0 || options.ToolTimeout <= 0 {
		return nil, errors.New("provider and tool timeouts must be positive")
	}
	if options.MaxToolRounds < 1 || options.MaxToolCalls < 1 || options.MaxHistory < 1 {
		return nil, errors.New("orchestration limits must be positive")
	}
	logger := options.Logger
	if logger == nil {
		logger = slog.Default()
	}
	now := options.Now
	if now == nil {
		now = time.Now
	}
	totalTimeout := options.TotalTimeout
	if totalTimeout <= 0 {
		totalTimeout = 25 * time.Second
	}
	return &Orchestrator{
		provider:        options.Provider,
		tools:           options.Tools,
		conversations:   options.Conversations,
		knowledge:       options.Knowledge,
		logger:          logger,
		model:           options.Model,
		temperature:     options.Temperature,
		maxOutputTokens: options.MaxOutputTokens,
		requestTimeout:  options.RequestTimeout,
		toolTimeout:     options.ToolTimeout,
		totalTimeout:    totalTimeout,
		maxToolRounds:   options.MaxToolRounds,
		maxToolCalls:    options.MaxToolCalls,
		maxHistory:      options.MaxHistory,
		now:             now,
	}, nil
}

// Chat runs one complete assistant turn.
func (o *Orchestrator) Chat(ctx context.Context, input ChatInput) (ChatOutput, error) {
	ctx, cancel := context.WithTimeout(ctx, o.totalTimeout)
	defer cancel()

	message := strings.TrimSpace(input.Message)
	if message == "" {
		return ChatOutput{}, &ValidationError{Field: "message", Issue: "must not be empty"}
	}
	if utf8.RuneCountInString(message) > MaxMessageCharacters {
		return ChatOutput{}, &ValidationError{Field: "message", Issue: "must be at most 500 characters"}
	}

	item, err := o.resolveConversation(input)
	if err != nil {
		return ChatOutput{}, err
	}

	if category, blocked := DetectSecurityThreat(message, input.Caller.Role, input.Caller.Authenticated); blocked {
		o.logger.WarnContext(ctx, "security guard triggered",
			"request_id", requestid.FromContext(ctx),
			"category", category,
			"authenticated", input.Caller.Authenticated,
		)
		output := ChatOutput{ConversationID: item.ID, Reply: SecurityRefusal, ToolCalls: []ToolCallSummary{}}
		o.persist(item.ID, message, SecurityRefusal)
		return output, nil
	}

	if OutOfScope(message) {
		output := ChatOutput{ConversationID: item.ID, Reply: ScopeRefusal, ToolCalls: []ToolCallSummary{}}
		o.persist(item.ID, message, ScopeRefusal)
		return output, nil
	}

	now := o.now().UTC()
	var retrieved []knowledge.Section
	if o.knowledge != nil {
		retrieved = o.knowledge.Retrieve(message, RetrievalLimit)
	}
	systemInstruction := buildSystemInstruction(knowledge.Format(retrieved), input.Caller.Authenticated, now)

	contents := buildContents(item.Messages, message, o.maxHistory)
	output := ChatOutput{ConversationID: item.ID, ToolCalls: []ToolCallSummary{}}

	if o.provider == nil {
		o.logger.WarnContext(ctx, "ai provider is not configured", "conversation_id", item.ID)
		return ChatOutput{}, ErrAIUnavailable
	}

	cache := make(map[string]contract.ToolResult)
	toolCount := 0
	duplicateStreak := 0

	for round := 0; ; round++ {
		response, err := o.generate(ctx, systemInstruction, contents)
		if err != nil {
			o.logError(ctx, "provider generate failed", item.ID, err)
			return ChatOutput{}, classifyProviderError(ctx, err)
		}

		if len(response.ToolCalls) == 0 {
			reply := strings.TrimSpace(response.Text)
			if reply == "" {
				return ChatOutput{}, ErrAIUnavailable
			}
			output.Reply = reply
			o.persist(item.ID, message, reply)
			o.logger.InfoContext(ctx, "ai chat completed",
				"conversation_id", item.ID,
				"authenticated", input.Caller.Authenticated,
				"tool_calls", len(output.ToolCalls),
			)
			return output, nil
		}

		if round >= o.maxToolRounds {
			output.Reply = fallbackReply
			o.persist(item.ID, message, fallbackReply)
			o.logger.WarnContext(ctx, "ai tool round limit reached", "conversation_id", item.ID, "rounds", round)
			return output, nil
		}

		modelParts := make([]contract.Part, 0, len(response.ToolCalls)+1)
		if strings.TrimSpace(response.Text) != "" {
			modelParts = append(modelParts, contract.Part{Text: response.Text})
		}
		responseParts := make([]contract.Part, 0, len(response.ToolCalls))

		for _, requested := range response.ToolCalls {
			call := requested
			modelParts = append(modelParts, contract.Part{FunctionCall: &call})

			if toolCount >= o.maxToolCalls {
				output.Reply = fallbackReply
				o.persist(item.ID, message, fallbackReply)
				o.logger.WarnContext(ctx, "ai tool call limit reached", "conversation_id", item.ID, "calls", toolCount)
				return output, nil
			}

			key := contract.CanonicalToolKey(call.Name, call.Arguments)
			if cached, ok := cache[key]; ok {
				duplicateStreak++
				responseParts = append(responseParts, contract.Part{
					FunctionResponse: functionResponse(call, cached),
				})
				if duplicateStreak >= 2 {
					output.Reply = fallbackReply
					o.persist(item.ID, message, fallbackReply)
					o.logger.WarnContext(ctx, "ai repeated tool call detected", "conversation_id", item.ID, "tool", call.Name)
					return output, nil
				}
				continue
			}
			duplicateStreak = 0

			_, known := o.tools.Get(call.Name)
			result, err := o.executeTool(ctx, call, input.Caller)
			if err != nil {
				return ChatOutput{}, err
			}
			cache[key] = result
			toolCount++
			if known {
				output.ToolCalls = append(output.ToolCalls, ToolCallSummary{
					Name:   call.Name,
					Status: string(result.Status),
				})
			}
			responseParts = append(responseParts, contract.Part{
				FunctionResponse: functionResponse(call, result),
			})
		}

		contents = append(contents,
			contract.Content{Role: "model", Parts: modelParts},
			contract.Content{Role: "user", Parts: responseParts},
		)
	}
}

func (o *Orchestrator) resolveConversation(input ChatInput) (conversation.Conversation, error) {
	if input.ConversationID == "" {
		guestSessionID := ""
		if !input.Caller.Authenticated {
			if !conversation.ValidGuestSessionID(input.Caller.SessionID) {
				return conversation.Conversation{}, errors.New("guest session is required for an anonymous conversation")
			}
			guestSessionID = input.Caller.SessionID
		}
		item, err := o.conversations.Create(input.Caller.UserID, guestSessionID, !input.Caller.Authenticated)
		if err != nil {
			return conversation.Conversation{}, fmt.Errorf("create conversation: %w", err)
		}
		return item, nil
	}

	if !conversation.ValidID(input.ConversationID) {
		return conversation.Conversation{}, &ValidationError{Field: "conversationId", Issue: "must be a valid conversation ID"}
	}

	item, err := o.conversations.Load(input.ConversationID)
	if err != nil {
		if errors.Is(err, conversation.ErrNotFound) {
			return conversation.Conversation{}, ErrConversationNotFound
		}
		return conversation.Conversation{}, &ValidationError{Field: "conversationId", Issue: "must be a valid conversation ID"}
	}

	if input.Caller.Authenticated {
		if item.Anonymous || item.OwnerID != input.Caller.UserID {
			return conversation.Conversation{}, ErrConversationNotFound
		}
		return item, nil
	}
	if !item.Anonymous ||
		!conversation.ValidGuestSessionID(input.Caller.SessionID) ||
		!sameGuestSession(item.GuestSessionID, input.Caller.SessionID) {
		return conversation.Conversation{}, ErrConversationNotFound
	}
	return item, nil
}

func sameGuestSession(stored, presented string) bool {
	if len(stored) != len(presented) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(stored), []byte(presented)) == 1
}

func (o *Orchestrator) generate(ctx context.Context, systemInstruction string, contents []contract.Content) (contract.GenerateResponse, error) {
	requestCtx, cancel := context.WithTimeout(ctx, o.requestTimeout)
	defer cancel()

	start := time.Now()
	response, err := o.provider.Generate(requestCtx, contract.GenerateRequest{
		SystemInstruction: systemInstruction,
		Contents:          contents,
		Tools:             o.tools.List(),
		Temperature:       o.temperature,
		MaxOutputTokens:   o.maxOutputTokens,
	})
	duration := time.Since(start)

	if err != nil {
		// Provider errors can echo request content, so only a safe category is
		// logged. The raw error is never written to the log.
		o.logger.ErrorContext(ctx, "ai provider call failed",
			"duration_ms", duration.Milliseconds(),
			"model", o.model,
			"error_category", providerErrorCategory(err),
		)
		return contract.GenerateResponse{}, err
	}

	o.logger.InfoContext(ctx, "ai provider call",
		"duration_ms", duration.Milliseconds(),
		"model", o.model,
		"usage_prompt", response.Usage.PromptTokens,
		"usage_output", response.Usage.OutputTokens,
		"usage_total", response.Usage.TotalTokens,
		"tool_calls", len(response.ToolCalls),
	)
	return response, nil
}

func (o *Orchestrator) executeTool(ctx context.Context, call contract.FunctionCall, caller contract.CallerContext) (contract.ToolResult, error) {
	if strings.TrimSpace(call.Name) == "" {
		return contract.ToolResult{Status: contract.StatusInvalid, Message: "tool is not available"}, nil
	}

	if _, ok := o.tools.Get(call.Name); !ok {
		o.logger.WarnContext(ctx, "ai requested an unknown tool", "tool", call.Name)
		return contract.ToolResult{Status: contract.StatusInvalid, Message: "tool is not available"}, nil
	}

	toolCtx, cancel := context.WithTimeout(ctx, o.toolTimeout)
	defer cancel()

	start := time.Now()
	result, err := o.tools.Execute(toolCtx, call.Name, call.Arguments, caller)
	duration := time.Since(start)

	if err != nil {
		if errors.Is(err, context.Canceled) && ctx.Err() != nil {
			return contract.ToolResult{}, ctx.Err()
		}
		o.logger.ErrorContext(ctx, "ai tool execution failed",
			"tool", call.Name,
			"duration_ms", duration.Milliseconds(),
			"error", err,
		)
		return contract.ToolResult{Status: contract.StatusUnavailable, Message: "That information is temporarily unavailable."}, nil
	}
	if result.Status == "" {
		result.Status = contract.StatusUnavailable
	}
	if result.Status == contract.StatusUnavailable && result.Message == "" {
		result.Message = "That information is temporarily unavailable."
	}

	if isPrivilegedParcelLookup(call.Name, caller.Role) {
		o.logger.InfoContext(ctx, "privileged parcel lookup",
			"audit", true,
			"request_id", requestid.FromContext(ctx),
			"actor_id", caller.UserID,
			"role", caller.Role,
			"tool", call.Name,
			"status", string(result.Status),
			"occurred_at", o.now().UTC().Format(time.RFC3339),
		)
	}

	o.logger.InfoContext(ctx, "ai tool execution",
		"tool", call.Name,
		"status", string(result.Status),
		"duration_ms", duration.Milliseconds(),
	)
	return result, nil
}

func (o *Orchestrator) persist(id, userMessage, assistantMessage string) {
	err := o.conversations.Append(id,
		conversation.Message{Role: conversation.RoleUser, Text: userMessage},
		conversation.Message{Role: conversation.RoleAssistant, Text: assistantMessage},
	)
	if err != nil {
		o.logger.Warn("could not persist conversation turn", "conversation_id", id, "error", err)
	}
}

func (o *Orchestrator) logError(ctx context.Context, message, conversationID string, err error) {
	o.logger.ErrorContext(ctx, message, "conversation_id", conversationID, "error_category", providerErrorCategory(err))
}

func providerErrorCategory(err error) string {
	switch {
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, contract.ErrTimeout):
		return "timeout"
	case errors.Is(err, context.Canceled):
		return "canceled"
	case errors.Is(err, contract.ErrInvalidResponse):
		return "invalid_response"
	case errors.Is(err, contract.ErrUnavailable):
		return "unavailable"
	default:
		return "provider_error"
	}
}

func classifyProviderError(ctx context.Context, err error) error {
	if errors.Is(err, context.Canceled) && ctx.Err() != nil {
		return ctx.Err()
	}
	return ErrAIUnavailable
}

func buildContents(messages []conversation.Message, userMessage string, maxHistory int) []contract.Content {
	if maxHistory < 1 {
		maxHistory = 1
	}
	history := messages
	if len(history) > maxHistory-1 {
		history = history[len(history)-(maxHistory-1):]
	}

	contents := make([]contract.Content, 0, len(history)+1)
	for _, message := range history {
		if strings.TrimSpace(message.Text) == "" {
			continue
		}
		role := "user"
		if message.Role == conversation.RoleAssistant {
			role = "model"
		}
		contents = append(contents, contract.Content{Role: role, Parts: []contract.Part{{Text: message.Text}}})
	}

	// A trimmed history can start with an assistant turn. The Gemini API is
	// happiest when the conversation starts with a user turn.
	for len(contents) > 0 && contents[0].Role == "model" {
		contents = contents[1:]
	}
	return append(contents, contract.Content{Role: "user", Parts: []contract.Part{{Text: userMessage}}})
}

func functionResponse(call contract.FunctionCall, result contract.ToolResult) *contract.FunctionResponse {
	payload := map[string]any{"status": string(result.Status)}
	if result.Message != "" {
		payload["message"] = result.Message
	}
	if result.Data != nil {
		if data := jsonValue(result.Data); data != nil {
			payload["data"] = data
		}
	}
	return &contract.FunctionResponse{
		ID:       call.ID,
		Name:     call.Name,
		Response: payload,
	}
}

func jsonValue(value any) any {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil
	}
	return decoded
}

func isPrivilegedParcelLookup(tool, role string) bool {
	if role != string(auth.RoleDispatcher) && role != string(auth.RoleAdmin) {
		return false
	}
	return tool == "get_my_parcel_history" || tool == "get_my_active_deliveries"
}
