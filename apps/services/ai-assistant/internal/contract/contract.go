// Package contract holds the shared types exchanged by the AI assistant's
// provider, tool registry, conversation store and orchestration layers. It is
// deliberately provider-neutral: Gemini types must stay inside
// internal/provider/gemini.
package contract

import (
	"context"
	"encoding/json"
	"errors"
)

// Provider errors. Callers map every provider failure to a safe 503 and must
// never expose the underlying error to a client or to the model.
var (
	ErrUnavailable     = errors.New("ai provider unavailable")
	ErrTimeout         = errors.New("ai provider timeout")
	ErrInvalidResponse = errors.New("ai provider returned an invalid response")
)

// CallerContext is the trusted identity of the HTTP caller. It is populated
// only by authentication middleware; no field may be supplied by the model.
type CallerContext struct {
	UserID        string
	Role          string
	Authenticated bool
	AccessToken   string
	SessionID     string
}

// TokenUsage is the provider-reported token accounting for one Generate call.
type TokenUsage struct {
	PromptTokens int32
	OutputTokens int32
	TotalTokens  int32
}

// Part is one content part sent to or returned by the provider.
type Part struct {
	Text             string
	FunctionCall     *FunctionCall
	FunctionResponse *FunctionResponse
}

// Content is one provider turn. Role is "user" or "model".
type Content struct {
	Role  string
	Parts []Part
}

// FunctionCall is a provider request to execute a named tool.
// ThoughtSignature is provider-opaque state that must be echoed back with the
// call on the next turn. Gemini 3.x rejects a missing signature for tool use.
type FunctionCall struct {
	ID               string
	Name             string
	Arguments        map[string]any
	ThoughtSignature []byte
}

// FunctionResponse is the sanitized result of a tool execution returned to the
// provider. Response is a JSON-serializable object.
type FunctionResponse struct {
	ID       string
	Name     string
	Response map[string]any
}

// GenerateRequest is one provider generation request. Contents already include
// prior text turns plus any function-call and function-response turns.
type GenerateRequest struct {
	SystemInstruction string
	Contents          []Content
	Tools             []ToolDeclaration
	Temperature       float32
	MaxOutputTokens   int32
}

// GenerateResponse is one provider generation result.
type GenerateResponse struct {
	Text      string
	ToolCalls []FunctionCall
	Usage     TokenUsage
}

// Provider is the single seam between orchestration and the model vendor.
type Provider interface {
	Generate(ctx context.Context, request GenerateRequest) (GenerateResponse, error)
}

// Parameter is a strict tool argument description. Only string, number,
// integer, boolean and enum parameters are supported by the MVP.
type Parameter struct {
	Type        string
	Description string
	Enum        []string
}

// Parameters is a flat object schema for a tool. Nested objects and arrays are
// intentionally unsupported so argument validation stays strict and simple.
type Parameters struct {
	Properties map[string]Parameter
	Required   []string
}

// ToolDeclaration is the provider-facing description of an approved tool.
type ToolDeclaration struct {
	Name        string
	Description string
	Parameters  Parameters
}

// ToolStatus is the safe, public outcome of one tool execution.
type ToolStatus string

const (
	StatusOK          ToolStatus = "OK"
	StatusInvalid     ToolStatus = "INVALID"
	StatusDenied      ToolStatus = "DENIED"
	StatusUnavailable ToolStatus = "UNAVAILABLE"
)

// ToolResult is a tool outcome. Message is safe to send back to the model; it
// must not contain credentials, upstream bodies or internal details.
type ToolResult struct {
	Status  ToolStatus
	Data    any
	Message string
}

// Tool executes one approved, read-only business operation.
type Tool interface {
	Definition() ToolDeclaration
	Execute(ctx context.Context, args map[string]any, caller CallerContext) (ToolResult, error)
}

// ToolRegistry is the allowlist boundary. Unknown names are never executed.
type ToolRegistry interface {
	Get(name string) (Tool, bool)
	List() []ToolDeclaration
	Execute(ctx context.Context, name string, args map[string]any, caller CallerContext) (ToolResult, error)
}

// CanonicalToolKey returns a stable key for duplicate-call protection. JSON
// encoding sorts object keys, so argument insertion order cannot change it.
func CanonicalToolKey(name string, args map[string]any) string {
	raw, err := json.Marshal(args)
	if err != nil {
		return name
	}
	return name + "|" + string(raw)
}
