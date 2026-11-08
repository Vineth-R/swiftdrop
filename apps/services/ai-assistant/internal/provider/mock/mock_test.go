package mock

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/Vineth-R/swiftdrop/apps/services/ai-assistant/internal/contract"
)

func TestProviderReturnsScriptedResponsesInOrder(t *testing.T) {
	t.Parallel()

	first := contract.GenerateResponse{Text: "first"}
	second := contract.GenerateResponse{Text: "second"}
	provider := New(first, second)

	got, err := provider.Generate(context.Background(), contract.GenerateRequest{})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if got.Text != "first" {
		t.Fatalf("first response = %q, want first", got.Text)
	}

	got, err = provider.Generate(context.Background(), contract.GenerateRequest{})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if got.Text != "second" {
		t.Fatalf("second response = %q, want second", got.Text)
	}

	got, err = provider.Generate(context.Background(), contract.GenerateRequest{})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if got.Text != DefaultResponse {
		t.Fatalf("exhausted response = %q, want %q", got.Text, DefaultResponse)
	}

	if provider.CallCount() != 3 {
		t.Fatalf("CallCount() = %d, want 3", provider.CallCount())
	}
}

func TestProviderErrorInjection(t *testing.T) {
	t.Parallel()

	scriptErr := errors.New("scripted failure")
	provider := NewSteps(
		Step{Response: contract.GenerateResponse{Text: "before"}},
		Step{Err: scriptErr},
		Step{Response: contract.GenerateResponse{Text: "after"}},
	)

	got, err := provider.Generate(context.Background(), contract.GenerateRequest{})
	if err != nil || got.Text != "before" {
		t.Fatalf("Generate() = (%#v, %v), want (before, nil)", got, err)
	}

	if _, err := provider.Generate(context.Background(), contract.GenerateRequest{}); !errors.Is(err, scriptErr) {
		t.Fatalf("Generate() error = %v, want scripted error", err)
	}

	got, err = provider.Generate(context.Background(), contract.GenerateRequest{})
	if err != nil || got.Text != "after" {
		t.Fatalf("Generate() = (%#v, %v), want (after, nil)", got, err)
	}
}

func TestProviderFailQueuesNextCall(t *testing.T) {
	t.Parallel()

	failure := errors.New("injected failure")
	provider := New()
	provider.Fail(failure)

	if _, err := provider.Generate(context.Background(), contract.GenerateRequest{}); !errors.Is(err, failure) {
		t.Fatalf("Generate() error = %v, want injected failure", err)
	}

	got, err := provider.Generate(context.Background(), contract.GenerateRequest{})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if got.Text != DefaultResponse {
		t.Fatalf("response = %q, want %q", got.Text, DefaultResponse)
	}
}

func TestProviderRecordsRequestCopies(t *testing.T) {
	t.Parallel()

	request := contract.GenerateRequest{
		SystemInstruction: "system",
		Contents: []contract.Content{
			{Role: "user", Parts: []contract.Part{{Text: "hello"}}},
			{Role: "model", Parts: []contract.Part{{
				FunctionCall: &contract.FunctionCall{
					ID:        "call-1",
					Name:      "calculate_delivery_fee",
					Arguments: map[string]any{"origin": "Colombo"},
				},
			}}},
		},
		Tools: []contract.ToolDeclaration{
			{
				Name: "calculate_delivery_fee",
				Parameters: contract.Parameters{
					Properties: map[string]contract.Parameter{
						"origin": {Type: "string", Enum: []string{"Colombo"}},
					},
					Required: []string{"origin"},
				},
			},
		},
		Temperature:     0.5,
		MaxOutputTokens: 128,
	}

	provider := New()
	if _, err := provider.Generate(context.Background(), request); err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	request.Contents[0].Parts[0].Text = "mutated"
	request.Contents[1].Parts[0].FunctionCall.Arguments["origin"] = "Kandy"
	request.Tools[0].Parameters.Properties["origin"] = contract.Parameter{Type: "number"}
	request.Tools[0].Parameters.Required[0] = "destination"

	recorded := provider.Requests()
	if len(recorded) != 1 {
		t.Fatalf("Requests() length = %d, want 1", len(recorded))
	}
	if recorded[0].Contents[0].Parts[0].Text != "hello" {
		t.Fatalf("recorded text = %q, want hello", recorded[0].Contents[0].Parts[0].Text)
	}
	if recorded[0].Contents[1].Parts[0].FunctionCall.Arguments["origin"] != "Colombo" {
		t.Fatalf("recorded argument = %#v, want Colombo", recorded[0].Contents[1].Parts[0].FunctionCall.Arguments)
	}
	if recorded[0].Tools[0].Parameters.Properties["origin"].Type != "string" {
		t.Fatalf("recorded parameter = %#v, want string", recorded[0].Tools[0].Parameters.Properties["origin"])
	}
	if recorded[0].Tools[0].Parameters.Required[0] != "origin" {
		t.Fatalf("recorded required = %#v", recorded[0].Tools[0].Parameters.Required)
	}

	recorded[0].Contents[1].Parts[0].FunctionCall.Arguments["origin"] = "Galle"
	second := provider.Requests()
	if second[0].Contents[1].Parts[0].FunctionCall.Arguments["origin"] != "Colombo" {
		t.Fatalf("Requests() returned an aliased map: %#v", second[0].Contents[1].Parts[0].FunctionCall.Arguments)
	}
}

func TestProviderReturnsResponseCopies(t *testing.T) {
	t.Parallel()

	scripted := contract.GenerateResponse{
		Text: "scripted",
		ToolCalls: []contract.FunctionCall{
			{ID: "call-1", Name: "tool", Arguments: map[string]any{"key": "value"}},
		},
	}
	provider := New(scripted, scripted)

	first, err := provider.Generate(context.Background(), contract.GenerateRequest{})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	first.ToolCalls[0].Arguments["key"] = "mutated"

	second, err := provider.Generate(context.Background(), contract.GenerateRequest{})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if second.Text != "scripted" {
		t.Fatalf("second response = %q, want scripted", second.Text)
	}
	if second.ToolCalls[0].Arguments["key"] != "value" {
		t.Fatalf("second response aliased the first: %#v", second.ToolCalls[0].Arguments)
	}
}

func TestProviderPreservesThoughtSignature(t *testing.T) {
	t.Parallel()

	provider := New(contract.GenerateResponse{
		ToolCalls: []contract.FunctionCall{{
			Name:             "get_public_parcel_status",
			ThoughtSignature: []byte("sig"),
		}},
	})
	response, err := provider.Generate(context.Background(), contract.GenerateRequest{})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if len(response.ToolCalls) != 1 || string(response.ToolCalls[0].ThoughtSignature) != "sig" {
		t.Fatalf("ToolCalls = %#v", response.ToolCalls)
	}
}

func TestProviderConcurrentUse(t *testing.T) {
	t.Parallel()

	provider := New()
	const goroutines = 50

	var waitGroup sync.WaitGroup
	waitGroup.Add(goroutines)
	for range goroutines {
		go func() {
			defer waitGroup.Done()
			if _, err := provider.Generate(context.Background(), contract.GenerateRequest{
				Contents: []contract.Content{{Role: "user", Parts: []contract.Part{{Text: "hello"}}}},
			}); err != nil {
				t.Errorf("Generate() error = %v", err)
			}
			_ = provider.Requests()
			_ = provider.CallCount()
		}()
	}
	waitGroup.Wait()

	if provider.CallCount() != goroutines {
		t.Fatalf("CallCount() = %d, want %d", provider.CallCount(), goroutines)
	}
	if len(provider.Requests()) != goroutines {
		t.Fatalf("Requests() length = %d, want %d", len(provider.Requests()), goroutines)
	}
}
