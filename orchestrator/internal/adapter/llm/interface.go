// Package llm provides an abstraction for LLM API clients.
package llm

import "context"

// LLMClient defines the interface for LLM API operations.
type LLMClient interface {
	// CreateChatCompletion sends a chat completion request (non-streaming).
	CreateChatCompletion(ctx context.Context, req *ChatCompletionRequest) (*ChatCompletionResponse, error)

	// CreateChatCompletionStream sends a streaming chat completion request.
	// The callback is called for each chunk received.
	CreateChatCompletionStream(ctx context.Context, req *ChatCompletionRequest, callback StreamCallback) (*Usage, error)

	// ListModels retrieves the list of available models.
	ListModels(ctx context.Context) ([]Model, error)
}

// Ensure Client implements LLMClient interface.
var _ LLMClient = (*Client)(nil)
