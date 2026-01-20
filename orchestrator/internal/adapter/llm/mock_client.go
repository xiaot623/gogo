package llm

import (
	"context"
	"fmt"
	"time"
)

// MockClient is a mock implementation of LLMClient for testing.
type MockClient struct{}

// NewMockClient creates a new mock LLM client.
func NewMockClient() *MockClient {
	return &MockClient{}
}

// Ensure MockClient implements LLMClient interface.
var _ LLMClient = (*MockClient)(nil)

// CreateChatCompletion returns a mock response.
func (m *MockClient) CreateChatCompletion(ctx context.Context, req *ChatCompletionRequest) (*ChatCompletionResponse, error) {
	// Generate mock response based on input
	responseContent := m.generateMockResponse(req)

	return &ChatCompletionResponse{
		ID:      fmt.Sprintf("mock-chatcmpl-%d", time.Now().UnixNano()),
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   req.Model,
		Choices: []Choice{
			{
				Index: 0,
				Message: &ChatMessage{
					Role:    "assistant",
					Content: responseContent,
				},
				FinishReason: "stop",
			},
		},
		Usage: &Usage{
			PromptTokens:     m.estimateTokens(req),
			CompletionTokens: len(responseContent) / 4,
			TotalTokens:      m.estimateTokens(req) + len(responseContent)/4,
		},
		SystemFingerprint: "mock-fp",
	}, nil
}

// CreateChatCompletionStream simulates a streaming response.
func (m *MockClient) CreateChatCompletionStream(ctx context.Context, req *ChatCompletionRequest, callback StreamCallback) (*Usage, error) {
	responseContent := m.generateMockResponse(req)
	id := fmt.Sprintf("mock-chatcmpl-%d", time.Now().UnixNano())
	created := time.Now().Unix()

	// Simulate streaming by sending content in chunks
	chunks := m.splitIntoChunks(responseContent, 10)

	for i, chunk := range chunks {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		finishReason := ""
		if i == len(chunks)-1 {
			finishReason = "stop"
		}

		streamChunk := &StreamChunk{
			ID:      id,
			Object:  "chat.completion.chunk",
			Created: created,
			Model:   req.Model,
			Choices: []Choice{
				{
					Index: 0,
					Delta: &ChatMessage{
						Role:    "assistant",
						Content: chunk,
					},
					FinishReason: finishReason,
				},
			},
			SystemFingerprint: "mock-fp",
		}

		if err := callback(streamChunk); err != nil {
			return nil, err
		}
	}

	usage := &Usage{
		PromptTokens:     m.estimateTokens(req),
		CompletionTokens: len(responseContent) / 4,
		TotalTokens:      m.estimateTokens(req) + len(responseContent)/4,
	}

	return usage, nil
}

// ListModels returns a list of mock models.
func (m *MockClient) ListModels(ctx context.Context) ([]Model, error) {
	return []Model{
		{
			ID:      "mock-gpt-4",
			Object:  "model",
			Created: time.Now().Unix(),
			OwnedBy: "mock",
		},
		{
			ID:      "mock-gpt-3.5-turbo",
			Object:  "model",
			Created: time.Now().Unix(),
			OwnedBy: "mock",
		},
	}, nil
}

// generateMockResponse generates a mock response based on the request.
func (m *MockClient) generateMockResponse(req *ChatCompletionRequest) string {
	// If tools are provided, return a mock tool call response as text
	if len(req.Tools) > 0 {
		return fmt.Sprintf("[MOCK] I would call tool '%s' to help with this request.", req.Tools[0].Function.Name)
	}

	// Get the last user message
	var lastUserMessage string
	for i := len(req.Messages) - 1; i >= 0; i-- {
		if req.Messages[i].Role == "user" {
			lastUserMessage = req.Messages[i].Content
			break
		}
	}

	if lastUserMessage == "" {
		return "[MOCK] This is a mock response from the LLM client."
	}

	return fmt.Sprintf("[MOCK] Received your message: %q. This is a mock response.", truncate(lastUserMessage, 100))
}

// estimateTokens provides a rough token count estimate.
func (m *MockClient) estimateTokens(req *ChatCompletionRequest) int {
	total := 0
	for _, msg := range req.Messages {
		total += len(msg.Content) / 4
	}
	return total
}

// splitIntoChunks splits a string into chunks of approximately the given size.
func (m *MockClient) splitIntoChunks(s string, chunkSize int) []string {
	if len(s) == 0 {
		return []string{""}
	}

	var chunks []string
	for i := 0; i < len(s); i += chunkSize {
		end := i + chunkSize
		if end > len(s) {
			end = len(s)
		}
		chunks = append(chunks, s[i:end])
	}
	return chunks
}

// truncate truncates a string to the given length.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
