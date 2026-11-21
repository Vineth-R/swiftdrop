// Package gemini adapts the official Google Gen AI SDK to the provider-neutral
// contract. Gemini types never leave this package.
package gemini

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"google.golang.org/genai"

	"github.com/Vineth-R/swiftdrop/apps/services/ai-assistant/internal/contract"
)

// Config is the Gemini provider configuration. APIKey and Model are required.
type Config struct {
	APIKey     string
	Model      string
	HTTPClient *http.Client
	// Timeout creates a defensive HTTP client timeout when HTTPClient is nil.
	// Orchestration still bounds every call with its own context.
	Timeout time.Duration
}

type generateFunc func(ctx context.Context, model string, contents []*genai.Content, config *genai.GenerateContentConfig) (*genai.GenerateContentResponse, error)

// Provider implements contract.Provider with the Gemini GenerateContent API.
type Provider struct {
	model    string
	generate generateFunc
}

// New creates a Gemini provider. The API key is passed only to the SDK and is
// never logged, wrapped into an error, or stored in a printable field.
func New(ctx context.Context, cfg Config) (*Provider, error) {
	apiKey := strings.TrimSpace(cfg.APIKey)
	if apiKey == "" {
		return nil, errors.New("gemini: APIKey is required")
	}
	model := strings.TrimSpace(cfg.Model)
	if model == "" {
		return nil, errors.New("gemini: Model is required")
	}

	httpClient := cfg.HTTPClient
	if httpClient == nil && cfg.Timeout > 0 {
		httpClient = &http.Client{Timeout: cfg.Timeout}
	}
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:     apiKey,
		Backend:    genai.BackendGeminiAPI,
		HTTPClient: httpClient,
	})
	if err != nil {
		return nil, fmt.Errorf("gemini: create client: %w", err)
	}

	return &Provider{
		model: model,
		generate: func(ctx context.Context, model string, contents []*genai.Content, config *genai.GenerateContentConfig) (*genai.GenerateContentResponse, error) {
			return client.Models.GenerateContent(ctx, model, contents, config)
		},
	}, nil
}

// Close is a no-op. The SDK client returned by genai.NewClient exposes no Close
// method and does not own the caller-supplied HTTP client.
func (p *Provider) Close() error {
	return nil
}

// Generate performs exactly one provider turn. Tool execution is left to the
// orchestration layer.
func (p *Provider) Generate(ctx context.Context, request contract.GenerateRequest) (contract.GenerateResponse, error) {
	if p == nil || p.generate == nil {
		return contract.GenerateResponse{}, contract.ErrUnavailable
	}

	contents, err := toContents(request.Contents)
	if err != nil {
		return contract.GenerateResponse{}, err
	}
	config, err := toConfig(request)
	if err != nil {
		return contract.GenerateResponse{}, err
	}

	response, err := p.generate(ctx, p.model, contents, config)
	if err != nil {
		return contract.GenerateResponse{}, classifyError(err)
	}
	if response == nil || len(response.Candidates) == 0 {
		return contract.GenerateResponse{}, contract.ErrInvalidResponse
	}

	return parseResponse(response)
}

// parseResponse reads the candidate parts directly instead of using
// FunctionCalls(), because the function-call part also carries the opaque
// ThoughtSignature that Gemini 3.x requires on the next request.
func parseResponse(response *genai.GenerateContentResponse) (contract.GenerateResponse, error) {
	if response == nil || len(response.Candidates) == 0 {
		return contract.GenerateResponse{}, contract.ErrInvalidResponse
	}
	candidate := response.Candidates[0]
	if candidate == nil || candidate.Content == nil {
		return contract.GenerateResponse{}, contract.ErrInvalidResponse
	}

	var text strings.Builder
	result := contract.GenerateResponse{}
	for _, part := range candidate.Content.Parts {
		if part == nil {
			continue
		}
		if part.FunctionCall != nil {
			result.ToolCalls = append(result.ToolCalls, contract.FunctionCall{
				ID:               part.FunctionCall.ID,
				Name:             part.FunctionCall.Name,
				Arguments:        part.FunctionCall.Args,
				ThoughtSignature: append([]byte(nil), part.ThoughtSignature...),
			})
			continue
		}
		if part.Text != "" && !part.Thought {
			text.WriteString(part.Text)
		}
	}

	result.Text = text.String()
	if strings.TrimSpace(result.Text) == "" && len(result.ToolCalls) == 0 {
		return contract.GenerateResponse{}, contract.ErrInvalidResponse
	}
	if response.UsageMetadata != nil {
		result.Usage = contract.TokenUsage{
			PromptTokens: response.UsageMetadata.PromptTokenCount,
			OutputTokens: response.UsageMetadata.CandidatesTokenCount,
			TotalTokens:  response.UsageMetadata.TotalTokenCount,
		}
	}
	return result, nil
}

func toContents(contents []contract.Content) ([]*genai.Content, error) {
	if len(contents) == 0 {
		return nil, nil
	}

	converted := make([]*genai.Content, 0, len(contents))
	for _, content := range contents {
		role, err := contentRole(content)
		if err != nil {
			return nil, err
		}
		converted = append(converted, &genai.Content{
			Role:  role,
			Parts: toParts(content.Parts),
		})
	}
	return converted, nil
}

func contentRole(content contract.Content) (string, error) {
	if content.Role != "" {
		switch content.Role {
		case "user":
			return genai.RoleUser, nil
		case "model":
			return genai.RoleModel, nil
		default:
			return "", fmt.Errorf("gemini: unsupported content role %q", content.Role)
		}
	}

	for _, part := range content.Parts {
		if part.FunctionCall != nil {
			return genai.RoleModel, nil
		}
		if part.FunctionResponse != nil {
			return genai.RoleUser, nil
		}
	}
	return genai.RoleUser, nil
}

func toParts(parts []contract.Part) []*genai.Part {
	if len(parts) == 0 {
		return nil
	}

	converted := make([]*genai.Part, 0, len(parts))
	for _, part := range parts {
		switch {
		case part.FunctionCall != nil:
			converted = append(converted, &genai.Part{
				FunctionCall: &genai.FunctionCall{
					ID:   part.FunctionCall.ID,
					Name: part.FunctionCall.Name,
					Args: part.FunctionCall.Arguments,
				},
				ThoughtSignature: append([]byte(nil), part.FunctionCall.ThoughtSignature...),
			})
		case part.FunctionResponse != nil:
			converted = append(converted, &genai.Part{
				FunctionResponse: &genai.FunctionResponse{
					ID:       part.FunctionResponse.ID,
					Name:     part.FunctionResponse.Name,
					Response: part.FunctionResponse.Response,
				},
			})
		case part.Text != "":
			converted = append(converted, &genai.Part{Text: part.Text})
		}
	}
	return converted
}

func toConfig(request contract.GenerateRequest) (*genai.GenerateContentConfig, error) {
	config := &genai.GenerateContentConfig{
		Temperature:     genai.Ptr(request.Temperature),
		MaxOutputTokens: request.MaxOutputTokens,
	}

	if strings.TrimSpace(request.SystemInstruction) != "" {
		config.SystemInstruction = genai.NewContentFromText(request.SystemInstruction, genai.RoleUser)
	}

	tools, err := toTools(request.Tools)
	if err != nil {
		return nil, err
	}
	if len(tools) > 0 {
		config.Tools = tools
	}

	return config, nil
}

func toTools(declarations []contract.ToolDeclaration) ([]*genai.Tool, error) {
	if len(declarations) == 0 {
		return nil, nil
	}

	functions := make([]*genai.FunctionDeclaration, 0, len(declarations))
	for _, declaration := range declarations {
		parameters, err := toSchema(declaration.Parameters)
		if err != nil {
			return nil, fmt.Errorf("gemini: tool %q: %w", declaration.Name, err)
		}
		functions = append(functions, &genai.FunctionDeclaration{
			Name:        declaration.Name,
			Description: declaration.Description,
			Parameters:  parameters,
		})
	}

	return []*genai.Tool{{FunctionDeclarations: functions}}, nil
}

func toSchema(parameters contract.Parameters) (*genai.Schema, error) {
	schema := &genai.Schema{Type: genai.TypeObject}

	if len(parameters.Properties) > 0 {
		schema.Properties = make(map[string]*genai.Schema, len(parameters.Properties))
		for name, parameter := range parameters.Properties {
			fieldType, err := parameterType(parameter.Type)
			if err != nil {
				return nil, fmt.Errorf("parameter %q: %w", name, err)
			}
			field := &genai.Schema{
				Type:        fieldType,
				Description: parameter.Description,
			}
			if len(parameter.Enum) > 0 {
				field.Enum = append([]string(nil), parameter.Enum...)
			}
			schema.Properties[name] = field
		}
	}

	if len(parameters.Required) > 0 {
		schema.Required = append([]string(nil), parameters.Required...)
	}

	return schema, nil
}

func parameterType(value string) (genai.Type, error) {
	switch strings.TrimSpace(value) {
	case "string":
		return genai.TypeString, nil
	case "number":
		return genai.TypeNumber, nil
	case "integer":
		return genai.TypeInteger, nil
	case "boolean":
		return genai.TypeBoolean, nil
	default:
		return "", fmt.Errorf("unsupported parameter type %q", value)
	}
}

func classifyError(err error) error {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return contract.ErrTimeout
	case errors.Is(err, context.Canceled):
		return fmt.Errorf("gemini: request canceled: %w", context.Canceled)
	}

	var apiErr genai.APIError
	if errors.As(err, &apiErr) {
		return contract.ErrUnavailable
	}
	var apiErrPtr *genai.APIError
	if errors.As(err, &apiErrPtr) {
		return contract.ErrUnavailable
	}

	var syntaxErr *json.SyntaxError
	if errors.As(err, &syntaxErr) {
		return contract.ErrInvalidResponse
	}
	var typeErr *json.UnmarshalTypeError
	if errors.As(err, &typeErr) {
		return contract.ErrInvalidResponse
	}
	if errors.Is(err, io.ErrUnexpectedEOF) {
		return contract.ErrInvalidResponse
	}

	return contract.ErrUnavailable
}
