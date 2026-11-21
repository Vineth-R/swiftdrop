package gemini

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"google.golang.org/genai"

	"github.com/Vineth-R/swiftdrop/apps/services/ai-assistant/internal/contract"
)

type capturedRequest struct {
	model    string
	contents []*genai.Content
	config   *genai.GenerateContentConfig
}

func newTestProvider(t *testing.T, generate generateFunc) *Provider {
	t.Helper()

	provider, err := New(context.Background(), Config{APIKey: "test-api-key", Model: "gemini-test"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	provider.generate = generate
	return provider
}

func captureProvider(t *testing.T, response *genai.GenerateContentResponse, err error) (*Provider, *capturedRequest) {
	t.Helper()

	captured := &capturedRequest{}
	provider := newTestProvider(t, func(_ context.Context, model string, contents []*genai.Content, config *genai.GenerateContentConfig) (*genai.GenerateContentResponse, error) {
		captured.model = model
		captured.contents = contents
		captured.config = config
		return response, err
	})
	return provider, captured
}

func textResponse(parts ...*genai.Part) *genai.GenerateContentResponse {
	return &genai.GenerateContentResponse{
		Candidates: []*genai.Candidate{
			{Content: &genai.Content{Role: genai.RoleModel, Parts: parts}},
		},
	}
}

func TestNewValidatesConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cfg  Config
	}{
		{name: "missing API key", cfg: Config{Model: "gemini-test"}},
		{name: "blank API key", cfg: Config{APIKey: "   ", Model: "gemini-test"}},
		{name: "missing model", cfg: Config{APIKey: "test-api-key"}},
		{name: "blank model", cfg: Config{APIKey: "test-api-key", Model: "  "}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if _, err := New(context.Background(), test.cfg); err == nil {
				t.Fatal("New() error = nil, want validation error")
			}
		})
	}

	provider, err := New(context.Background(), Config{APIKey: "test-api-key", Model: "gemini-test"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if err := provider.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}

func TestGenerateRequestConstruction(t *testing.T) {
	t.Parallel()

	provider, captured := captureProvider(t, textResponse(&genai.Part{Text: "ok"}), nil)

	request := contract.GenerateRequest{
		SystemInstruction: "Answer only about SwiftDrop.",
		Contents: []contract.Content{
			{Role: "user", Parts: []contract.Part{{Text: "Where is my parcel?"}}},
			{Role: "", Parts: []contract.Part{{
				FunctionCall: &contract.FunctionCall{
					ID:               "call-1",
					Name:             "get_public_parcel_status",
					Arguments:        map[string]any{"trackingNumber": "SDK12345678"},
					ThoughtSignature: []byte("opaque-signature"),
				},
			}}},
			{Role: "", Parts: []contract.Part{{
				FunctionResponse: &contract.FunctionResponse{
					ID:       "call-1",
					Name:     "get_public_parcel_status",
					Response: map[string]any{"status": "IN_TRANSIT"},
				},
			}}},
			{Role: "model", Parts: []contract.Part{{Text: "It is in transit."}}},
		},
		Tools: []contract.ToolDeclaration{
			{
				Name:        "calculate_delivery_fee",
				Description: "Estimate a delivery fee.",
				Parameters: contract.Parameters{
					Properties: map[string]contract.Parameter{
						"origin":       {Type: "string", Description: "Origin city."},
						"weightKg":     {Type: "number", Description: "Weight in kilograms."},
						"sizeCategory": {Type: "string", Description: "Size band.", Enum: []string{"SMALL", "MEDIUM", "LARGE", "OVERSIZED"}},
						"insured":      {Type: "boolean", Description: "Whether insurance applies."},
						"parcels":      {Type: "integer", Description: "Number of parcels."},
					},
					Required: []string{"origin", "weightKg", "sizeCategory"},
				},
			},
		},
		Temperature:     0.2,
		MaxOutputTokens: 512,
	}

	if _, err := provider.Generate(context.Background(), request); err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	if captured.model != "gemini-test" {
		t.Fatalf("model = %q, want gemini-test", captured.model)
	}

	if len(captured.contents) != 4 {
		t.Fatalf("contents length = %d, want 4", len(captured.contents))
	}

	first := captured.contents[0]
	if first.Role != genai.RoleUser {
		t.Fatalf("content[0].Role = %q, want %q", first.Role, genai.RoleUser)
	}
	if len(first.Parts) != 1 || first.Parts[0].Text != "Where is my parcel?" {
		t.Fatalf("content[0] parts = %#v", first.Parts)
	}

	callContent := captured.contents[1]
	if callContent.Role != genai.RoleModel {
		t.Fatalf("content[1].Role = %q, want %q", callContent.Role, genai.RoleModel)
	}
	callPart := callContent.Parts[0].FunctionCall
	if callPart == nil || callPart.ID != "call-1" || callPart.Name != "get_public_parcel_status" {
		t.Fatalf("content[1] function call = %#v", callPart)
	}
	if callPart.Args["trackingNumber"] != "SDK12345678" {
		t.Fatalf("content[1] arguments = %#v", callPart.Args)
	}
	if string(callContent.Parts[0].ThoughtSignature) != "opaque-signature" {
		t.Fatalf("content[1] thought signature = %q", callContent.Parts[0].ThoughtSignature)
	}

	responseContent := captured.contents[2]
	if responseContent.Role != genai.RoleUser {
		t.Fatalf("content[2].Role = %q, want %q", responseContent.Role, genai.RoleUser)
	}
	responsePart := responseContent.Parts[0].FunctionResponse
	if responsePart == nil || responsePart.ID != "call-1" || responsePart.Name != "get_public_parcel_status" {
		t.Fatalf("content[2] function response = %#v", responsePart)
	}
	if responsePart.Response["status"] != "IN_TRANSIT" {
		t.Fatalf("content[2] response = %#v", responsePart.Response)
	}

	if captured.contents[3].Role != genai.RoleModel {
		t.Fatalf("content[3].Role = %q, want %q", captured.contents[3].Role, genai.RoleModel)
	}

	config := captured.config
	if config == nil {
		t.Fatal("config = nil")
	}
	if config.SystemInstruction == nil || len(config.SystemInstruction.Parts) != 1 {
		t.Fatalf("SystemInstruction = %#v", config.SystemInstruction)
	}
	if config.SystemInstruction.Parts[0].Text != "Answer only about SwiftDrop." {
		t.Fatalf("SystemInstruction text = %q", config.SystemInstruction.Parts[0].Text)
	}
	if config.SystemInstruction.Role != genai.RoleUser {
		t.Fatalf("SystemInstruction role = %q, want %q", config.SystemInstruction.Role, genai.RoleUser)
	}
	if config.Temperature == nil || *config.Temperature != 0.2 {
		t.Fatalf("Temperature = %#v, want 0.2", config.Temperature)
	}
	if config.MaxOutputTokens != 512 {
		t.Fatalf("MaxOutputTokens = %d, want 512", config.MaxOutputTokens)
	}

	if len(config.Tools) != 1 || len(config.Tools[0].FunctionDeclarations) != 1 {
		t.Fatalf("Tools = %#v", config.Tools)
	}
	declaration := config.Tools[0].FunctionDeclarations[0]
	if declaration.Name != "calculate_delivery_fee" || declaration.Description != "Estimate a delivery fee." {
		t.Fatalf("FunctionDeclaration = %#v", declaration)
	}
	parameters := declaration.Parameters
	if parameters == nil || parameters.Type != genai.TypeObject {
		t.Fatalf("Parameters = %#v", parameters)
	}
	if parameters.Properties["origin"].Type != genai.TypeString {
		t.Fatalf("origin type = %q", parameters.Properties["origin"].Type)
	}
	if parameters.Properties["weightKg"].Type != genai.TypeNumber {
		t.Fatalf("weightKg type = %q", parameters.Properties["weightKg"].Type)
	}
	if parameters.Properties["insured"].Type != genai.TypeBoolean {
		t.Fatalf("insured type = %q", parameters.Properties["insured"].Type)
	}
	if parameters.Properties["parcels"].Type != genai.TypeInteger {
		t.Fatalf("parcels type = %q", parameters.Properties["parcels"].Type)
	}
	if len(parameters.Properties["sizeCategory"].Enum) != 4 {
		t.Fatalf("sizeCategory enum = %#v", parameters.Properties["sizeCategory"].Enum)
	}
	if len(parameters.Required) != 3 {
		t.Fatalf("Required = %#v", parameters.Required)
	}
}

func TestGenerateOmitsEmptyOptionalConfig(t *testing.T) {
	t.Parallel()

	provider, captured := captureProvider(t, textResponse(&genai.Part{Text: "ok"}), nil)

	request := contract.GenerateRequest{
		Contents: []contract.Content{{Role: "user", Parts: []contract.Part{{Text: "hello"}}}},
	}

	if _, err := provider.Generate(context.Background(), request); err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	if captured.config.SystemInstruction != nil {
		t.Fatalf("SystemInstruction = %#v, want nil", captured.config.SystemInstruction)
	}
	if captured.config.Tools != nil {
		t.Fatalf("Tools = %#v, want nil", captured.config.Tools)
	}
	if captured.config.Temperature == nil {
		t.Fatal("Temperature = nil, want a pointer even when zero")
	}
}

func TestGenerateParsesFunctionCalls(t *testing.T) {
	t.Parallel()

	response := &genai.GenerateContentResponse{
		Candidates: []*genai.Candidate{
			{Content: &genai.Content{Role: genai.RoleModel, Parts: []*genai.Part{
				{
					FunctionCall: &genai.FunctionCall{
						ID:   "call-1",
						Name: "calculate_delivery_fee",
						Args: map[string]any{"origin": "Colombo", "destination": "Kandy"},
					},
					ThoughtSignature: []byte("signature-1"),
				},
				{FunctionCall: &genai.FunctionCall{
					ID:   "call-2",
					Name: "get_public_parcel_status",
					Args: map[string]any{"trackingNumber": "SDK12345678"},
				}},
			}}},
		},
	}

	provider, _ := captureProvider(t, response, nil)

	result, err := provider.Generate(context.Background(), contract.GenerateRequest{})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	if len(result.ToolCalls) != 2 {
		t.Fatalf("ToolCalls length = %d, want 2", len(result.ToolCalls))
	}
	if result.ToolCalls[0].ID != "call-1" || result.ToolCalls[0].Name != "calculate_delivery_fee" {
		t.Fatalf("ToolCalls[0] = %#v", result.ToolCalls[0])
	}
	if result.ToolCalls[0].Arguments["origin"] != "Colombo" {
		t.Fatalf("ToolCalls[0].Arguments = %#v", result.ToolCalls[0].Arguments)
	}
	if string(result.ToolCalls[0].ThoughtSignature) != "signature-1" {
		t.Fatalf("ToolCalls[0].ThoughtSignature = %q", result.ToolCalls[0].ThoughtSignature)
	}
	if result.ToolCalls[1].ID != "call-2" || result.ToolCalls[1].Name != "get_public_parcel_status" {
		t.Fatalf("ToolCalls[1] = %#v", result.ToolCalls[1])
	}
}

func TestGenerateSkipsThoughtTextAndPreservesSignature(t *testing.T) {
	t.Parallel()

	response := textResponse(
		&genai.Part{Text: "internal reasoning", Thought: true},
		&genai.Part{Text: "Visible answer. "},
		&genai.Part{
			FunctionCall:     &genai.FunctionCall{Name: "get_public_parcel_status"},
			ThoughtSignature: []byte("sig"),
		},
	)
	provider, _ := captureProvider(t, response, nil)

	result, err := provider.Generate(context.Background(), contract.GenerateRequest{})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if result.Text != "Visible answer. " {
		t.Fatalf("Text = %q, want visible text only", result.Text)
	}
	if len(result.ToolCalls) != 1 || string(result.ToolCalls[0].ThoughtSignature) != "sig" {
		t.Fatalf("ToolCalls = %#v", result.ToolCalls)
	}
}

func TestGenerateParsesTextAndUsage(t *testing.T) {
	t.Parallel()

	response := &genai.GenerateContentResponse{
		Candidates: []*genai.Candidate{
			{Content: &genai.Content{Role: genai.RoleModel, Parts: []*genai.Part{
				{Text: "Your parcel "},
				{Text: "is in transit."},
			}}},
		},
		UsageMetadata: &genai.GenerateContentResponseUsageMetadata{
			PromptTokenCount:     12,
			CandidatesTokenCount: 7,
			TotalTokenCount:      19,
		},
	}

	provider, _ := captureProvider(t, response, nil)

	result, err := provider.Generate(context.Background(), contract.GenerateRequest{})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	if result.Text != "Your parcel is in transit." {
		t.Fatalf("Text = %q", result.Text)
	}
	if len(result.ToolCalls) != 0 {
		t.Fatalf("ToolCalls = %#v, want none", result.ToolCalls)
	}
	if result.Usage.PromptTokens != 12 || result.Usage.OutputTokens != 7 || result.Usage.TotalTokens != 19 {
		t.Fatalf("Usage = %#v", result.Usage)
	}
}

func TestGenerateInvalidResponses(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		response *genai.GenerateContentResponse
	}{
		{name: "nil response", response: nil},
		{name: "no candidates", response: &genai.GenerateContentResponse{}},
		{
			name:     "nil content",
			response: &genai.GenerateContentResponse{Candidates: []*genai.Candidate{{}}},
		},
		{
			name: "no parts",
			response: &genai.GenerateContentResponse{
				Candidates: []*genai.Candidate{{Content: &genai.Content{Role: genai.RoleModel}}},
			},
		},
		{
			name:     "empty part",
			response: textResponse(&genai.Part{}),
		},
		{
			name:     "whitespace text",
			response: textResponse(&genai.Part{Text: "   "}),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			provider, _ := captureProvider(t, test.response, nil)
			_, err := provider.Generate(context.Background(), contract.GenerateRequest{})
			if !errors.Is(err, contract.ErrInvalidResponse) {
				t.Fatalf("Generate() error = %v, want ErrInvalidResponse", err)
			}
		})
	}
}

func TestGenerateClassifiesAPIErrors(t *testing.T) {
	t.Parallel()

	for _, code := range []int{401, 403, 429, 500} {
		code := code
		t.Run(fmt.Sprintf("status %d", code), func(t *testing.T) {
			t.Parallel()

			provider, _ := captureProvider(t, nil, genai.APIError{
				Code:    code,
				Status:  "ERROR",
				Message: "provider-secret-detail",
			})

			_, err := provider.Generate(context.Background(), contract.GenerateRequest{})
			if !errors.Is(err, contract.ErrUnavailable) {
				t.Fatalf("Generate() error = %v, want ErrUnavailable", err)
			}
			if strings.Contains(err.Error(), "provider-secret-detail") {
				t.Fatalf("error leaked provider message: %v", err)
			}
		})
	}
}

func TestGenerateClassifiesPointerAPIError(t *testing.T) {
	t.Parallel()

	provider, _ := captureProvider(t, nil, &genai.APIError{Code: 503})

	_, err := provider.Generate(context.Background(), contract.GenerateRequest{})
	if !errors.Is(err, contract.ErrUnavailable) {
		t.Fatalf("Generate() error = %v, want ErrUnavailable", err)
	}
}

func TestGenerateClassifiesTimeout(t *testing.T) {
	t.Parallel()

	provider, _ := captureProvider(t, nil, fmt.Errorf("upstream: %w", context.DeadlineExceeded))

	_, err := provider.Generate(context.Background(), contract.GenerateRequest{})
	if !errors.Is(err, contract.ErrTimeout) {
		t.Fatalf("Generate() error = %v, want ErrTimeout", err)
	}
}

func TestGenerateClassifiesCancellation(t *testing.T) {
	t.Parallel()

	provider, _ := captureProvider(t, nil, fmt.Errorf("upstream: %w", context.Canceled))

	_, err := provider.Generate(context.Background(), contract.GenerateRequest{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Generate() error = %v, want context.Canceled", err)
	}
	if errors.Is(err, contract.ErrTimeout) {
		t.Fatalf("Generate() error = %v, want cancellation not timeout", err)
	}
}

func TestGenerateRejectsUnknownToolParameterType(t *testing.T) {
	t.Parallel()

	called := false
	provider := newTestProvider(t, func(context.Context, string, []*genai.Content, *genai.GenerateContentConfig) (*genai.GenerateContentResponse, error) {
		called = true
		return textResponse(&genai.Part{Text: "ok"}), nil
	})

	request := contract.GenerateRequest{
		Tools: []contract.ToolDeclaration{
			{
				Name: "calculate_delivery_fee",
				Parameters: contract.Parameters{
					Properties: map[string]contract.Parameter{
						"stops": {Type: "array"},
					},
				},
			},
		},
	}

	_, err := provider.Generate(context.Background(), request)
	if err == nil {
		t.Fatal("Generate() error = nil, want unsupported parameter type error")
	}
	if called {
		t.Fatal("provider SDK was called despite an invalid tool schema")
	}
}

func TestGenerateRejectsUnknownContentRole(t *testing.T) {
	t.Parallel()

	provider := newTestProvider(t, func(context.Context, string, []*genai.Content, *genai.GenerateContentConfig) (*genai.GenerateContentResponse, error) {
		t.Fatal("provider SDK should not be called")
		return nil, nil
	})

	request := contract.GenerateRequest{
		Contents: []contract.Content{{Role: "system", Parts: []contract.Part{{Text: "hello"}}}},
	}

	if _, err := provider.Generate(context.Background(), request); err == nil {
		t.Fatal("Generate() error = nil, want unsupported role error")
	}
}
