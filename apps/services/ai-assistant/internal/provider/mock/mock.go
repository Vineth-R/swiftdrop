// Package mock provides a deterministic, thread-safe scripted Provider for
// tests and local development. It never contacts a model vendor.
package mock

import (
	"context"
	"sync"

	"github.com/Vineth-R/swiftdrop/apps/services/ai-assistant/internal/contract"
)

// DefaultResponse is returned once the script is exhausted.
const DefaultResponse = "Mock assistant response."

// Step is one scripted generation outcome. When Err is non-nil it is returned
// instead of Response.
type Step struct {
	Response contract.GenerateResponse
	Err      error
}

// Provider is a scripted contract.Provider. It is safe for concurrent use.
type Provider struct {
	mu       sync.Mutex
	steps    []Step
	requests []contract.GenerateRequest
	calls    int
}

// New returns a provider that yields responses in order, then DefaultResponse.
func New(responses ...contract.GenerateResponse) *Provider {
	steps := make([]Step, 0, len(responses))
	for _, response := range responses {
		steps = append(steps, Step{Response: response})
	}
	return &Provider{steps: steps}
}

// NewSteps returns a provider driven by explicit steps, which allows errors to
// be interleaved with responses.
func NewSteps(steps ...Step) *Provider {
	queued := make([]Step, len(steps))
	copy(queued, steps)
	return &Provider{steps: queued}
}

// Generate records a copy of the request and returns the next scripted step.
// Once the script is exhausted it returns DefaultResponse.
func (p *Provider) Generate(_ context.Context, request contract.GenerateRequest) (contract.GenerateResponse, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.requests = append(p.requests, cloneRequest(request))
	p.calls++

	if len(p.steps) == 0 {
		return contract.GenerateResponse{Text: DefaultResponse}, nil
	}

	step := p.steps[0]
	p.steps = p.steps[1:]
	if step.Err != nil {
		return contract.GenerateResponse{}, step.Err
	}
	return cloneResponse(step.Response), nil
}

// Fail queues an error for the next Generate call. It is equivalent to a Step
// with only Err set.
func (p *Provider) Fail(err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.steps = append(p.steps, Step{Err: err})
}

// Requests returns a copy of every request recorded so far.
func (p *Provider) Requests() []contract.GenerateRequest {
	p.mu.Lock()
	defer p.mu.Unlock()

	requests := make([]contract.GenerateRequest, len(p.requests))
	for i, request := range p.requests {
		requests[i] = cloneRequest(request)
	}
	return requests
}

// CallCount returns the number of Generate calls made so far.
func (p *Provider) CallCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls
}

func cloneRequest(request contract.GenerateRequest) contract.GenerateRequest {
	return contract.GenerateRequest{
		SystemInstruction: request.SystemInstruction,
		Contents:          cloneContents(request.Contents),
		Tools:             cloneTools(request.Tools),
		Temperature:       request.Temperature,
		MaxOutputTokens:   request.MaxOutputTokens,
	}
}

func cloneContents(contents []contract.Content) []contract.Content {
	if contents == nil {
		return nil
	}

	cloned := make([]contract.Content, len(contents))
	for i, content := range contents {
		cloned[i] = contract.Content{
			Role:  content.Role,
			Parts: cloneParts(content.Parts),
		}
	}
	return cloned
}

func cloneParts(parts []contract.Part) []contract.Part {
	if parts == nil {
		return nil
	}

	cloned := make([]contract.Part, len(parts))
	for i, part := range parts {
		cloned[i] = contract.Part{Text: part.Text}
		if part.FunctionCall != nil {
			cloned[i].FunctionCall = &contract.FunctionCall{
				ID:               part.FunctionCall.ID,
				Name:             part.FunctionCall.Name,
				Arguments:        cloneMap(part.FunctionCall.Arguments),
				ThoughtSignature: append([]byte(nil), part.FunctionCall.ThoughtSignature...),
			}
		}
		if part.FunctionResponse != nil {
			cloned[i].FunctionResponse = &contract.FunctionResponse{
				ID:       part.FunctionResponse.ID,
				Name:     part.FunctionResponse.Name,
				Response: cloneMap(part.FunctionResponse.Response),
			}
		}
	}
	return cloned
}

func cloneTools(tools []contract.ToolDeclaration) []contract.ToolDeclaration {
	if tools == nil {
		return nil
	}

	cloned := make([]contract.ToolDeclaration, len(tools))
	for i, tool := range tools {
		cloned[i] = contract.ToolDeclaration{
			Name:        tool.Name,
			Description: tool.Description,
		}
		if tool.Parameters.Properties != nil {
			cloned[i].Parameters.Properties = make(map[string]contract.Parameter, len(tool.Parameters.Properties))
			for name, parameter := range tool.Parameters.Properties {
				field := contract.Parameter{
					Type:        parameter.Type,
					Description: parameter.Description,
				}
				if parameter.Enum != nil {
					field.Enum = append([]string(nil), parameter.Enum...)
				}
				cloned[i].Parameters.Properties[name] = field
			}
		}
		if tool.Parameters.Required != nil {
			cloned[i].Parameters.Required = append([]string(nil), tool.Parameters.Required...)
		}
	}
	return cloned
}

func cloneMap(values map[string]any) map[string]any {
	if values == nil {
		return nil
	}

	cloned := make(map[string]any, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

func cloneResponse(response contract.GenerateResponse) contract.GenerateResponse {
	cloned := contract.GenerateResponse{
		Text:  response.Text,
		Usage: response.Usage,
	}
	if response.ToolCalls != nil {
		cloned.ToolCalls = make([]contract.FunctionCall, len(response.ToolCalls))
		for i, call := range response.ToolCalls {
			cloned.ToolCalls[i] = contract.FunctionCall{
				ID:               call.ID,
				Name:             call.Name,
				Arguments:        cloneMap(call.Arguments),
				ThoughtSignature: append([]byte(nil), call.ThoughtSignature...),
			}
		}
	}
	return cloned
}
