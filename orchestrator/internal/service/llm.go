package service

import (
	"context"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/xiaot623/gogo/orchestrator/internal/adapter/llm"
	"github.com/xiaot623/gogo/orchestrator/internal/domain"
)

// ProxyChatCompletion handles non-streaming chat completion proxying.
func (s *Service) ProxyChatCompletion(ctx context.Context, runID string, req *llm.ChatCompletionRequest) (*llm.ChatCompletionResponse, error) {
	requestID := "llm_" + uuid.New().String()[:8]
	startTime := time.Now()

	// Record llm_call_started event
	if runID != "" {
		if err := s.recordEvent(ctx, runID, domain.EventTypeLLMCallStarted, domain.LLMCallStartedPayload{
			RequestID: requestID,
			Model:     req.Model,
			Stream:    req.Stream,
		}); err != nil {
			log.Printf("WARN: failed to record llm_call_started event: %v", err)
		}
	}

	resp, err := s.llmClient.CreateChatCompletion(ctx, req)
	if err != nil {
		latencyMs := time.Since(startTime).Milliseconds()
		// Record llm_call_done with error
		if runID != "" {
			s.recordEvent(ctx, runID, domain.EventTypeLLMCallDone, domain.LLMCallDonePayload{
				RequestID: requestID,
				Model:     req.Model,
				LatencyMs: latencyMs,
				Error:     err.Error(),
			})
		}
		return nil, err
	}

	latencyMs := time.Since(startTime).Milliseconds()

	// Record llm_call_done event
	if runID != "" {
		payload := domain.LLMCallDonePayload{
			RequestID: requestID,
			Model:     resp.Model,
			LatencyMs: latencyMs,
		}
		if resp.Usage != nil {
			payload.PromptTokens = resp.Usage.PromptTokens
			payload.CompletionTokens = resp.Usage.CompletionTokens
			payload.TotalTokens = resp.Usage.TotalTokens
		}
		if err := s.recordEvent(ctx, runID, domain.EventTypeLLMCallDone, payload); err != nil {
			log.Printf("WARN: failed to record llm_call_done event: %v", err)
		}
	}

	return resp, nil
}

// ProxyChatCompletionStream handles streaming chat completion proxying.
func (s *Service) ProxyChatCompletionStream(ctx context.Context, runID string, req *llm.ChatCompletionRequest, callback llm.StreamCallback) error {
	requestID := "llm_" + uuid.New().String()[:8]
	startTime := time.Now()

	// Record llm_call_started event
	if runID != "" {
		if err := s.recordEvent(ctx, runID, domain.EventTypeLLMCallStarted, domain.LLMCallStartedPayload{
			RequestID: requestID,
			Model:     req.Model,
			Stream:    req.Stream,
		}); err != nil {
			log.Printf("WARN: failed to record llm_call_started event: %v", err)
		}
	}

	var responseModel string

	// Wrap callback to capture model
	wrapperCallback := func(chunk *llm.StreamChunk) error {
		if responseModel == "" && chunk.Model != "" {
			responseModel = chunk.Model
		}
		return callback(chunk)
	}

	usage, err := s.llmClient.CreateChatCompletionStream(ctx, req, wrapperCallback)

	latencyMs := time.Since(startTime).Milliseconds()

	// Record llm_call_done event
	if runID != "" {
		payload := domain.LLMCallDonePayload{
			RequestID: requestID,
			Model:     responseModel,
			LatencyMs: latencyMs,
		}
		if usage != nil {
			payload.PromptTokens = usage.PromptTokens
			payload.CompletionTokens = usage.CompletionTokens
			payload.TotalTokens = usage.TotalTokens
		}
		if err != nil {
			payload.Error = err.Error()
		}
		if recordErr := s.recordEvent(ctx, runID, domain.EventTypeLLMCallDone, payload); recordErr != nil {
			log.Printf("WARN: failed to record llm_call_done event: %v", recordErr)
		}
	}

	return err
}

// ListModels retrieves the list of available models.
func (s *Service) ListModels(ctx context.Context) ([]llm.Model, error) {
	return s.llmClient.ListModels(ctx)
}
