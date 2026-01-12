package llmproxy

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/xiaot623/gogo/orchestrator/config"
	"github.com/xiaot623/gogo/orchestrator/domain"
	"github.com/xiaot623/gogo/orchestrator/store"
)

// Handler handles LLM proxy HTTP requests.
type Handler struct {
	client *Client
	store  store.Store
	config *config.Config
}

// NewHandler creates a new LLM proxy handler.
func NewHandler(cfg *config.Config, store store.Store) *Handler {
	client := NewClient(cfg.LiteLLMURL, cfg.LiteLLMAPIKey, cfg.LLMTimeout)
	return &Handler{
		client: client,
		store:  store,
		config: cfg,
	}
}

// RegisterRoutes registers LLM proxy routes.
func (h *Handler) RegisterRoutes(e *echo.Echo) {
	// OpenAI-compatible endpoints
	e.POST("/v1/chat/completions", h.ChatCompletions)
	e.GET("/v1/models", h.ListModels)
}

// ChatCompletions handles chat completion requests.
// POST /v1/chat/completions
func (h *Handler) ChatCompletions(c echo.Context) error {
	ctx := c.Request().Context()

	// Get run_id from header for trace correlation
	runID := c.Request().Header.Get("x-run-id")

	var req ChatCompletionRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: &APIError{
				Message: "invalid request body",
				Type:    "invalid_request_error",
			},
		})
	}

	// Validate required fields
	if req.Model == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: &APIError{
				Message: "model is required",
				Type:    "invalid_request_error",
				Param:   "model",
			},
		})
	}
	if len(req.Messages) == 0 {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: &APIError{
				Message: "messages is required",
				Type:    "invalid_request_error",
				Param:   "messages",
			},
		})
	}

	// Generate request ID for tracing
	requestID := "llm_" + uuid.New().String()[:8]
	startTime := time.Now()

	// Record llm_call_started event
	if runID != "" {
		if err := h.recordEvent(ctx, runID, domain.EventTypeLLMCallStarted, domain.LLMCallStartedPayload{
			RequestID: requestID,
			Model:     req.Model,
			Stream:    req.Stream,
		}); err != nil {
			log.Printf("WARN: failed to record llm_call_started event: %v", err)
		}
	}

	if req.Stream {
		return h.handleStreamingRequest(c, ctx, &req, runID, requestID, startTime)
	}

	return h.handleNonStreamingRequest(c, ctx, &req, runID, requestID, startTime)
}

// handleNonStreamingRequest handles non-streaming chat completion requests.
func (h *Handler) handleNonStreamingRequest(c echo.Context, ctx context.Context, req *ChatCompletionRequest, runID, requestID string, startTime time.Time) error {
	resp, err := h.client.CreateChatCompletion(ctx, req)
	if err != nil {
		latencyMs := time.Since(startTime).Milliseconds()

		// Record llm_call_done with error
		if runID != "" {
			h.recordEvent(ctx, runID, domain.EventTypeLLMCallDone, domain.LLMCallDonePayload{
				RequestID: requestID,
				Model:     req.Model,
				LatencyMs: latencyMs,
				Error:     err.Error(),
			})
		}

		log.Printf("ERROR: LLM request failed: %v", err)
		return c.JSON(http.StatusBadGateway, ErrorResponse{
			Error: &APIError{
				Message: err.Error(),
				Type:    "upstream_error",
			},
		})
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
		if err := h.recordEvent(ctx, runID, domain.EventTypeLLMCallDone, payload); err != nil {
			log.Printf("WARN: failed to record llm_call_done event: %v", err)
		}
	}

	return c.JSON(http.StatusOK, resp)
}

// handleStreamingRequest handles streaming chat completion requests.
func (h *Handler) handleStreamingRequest(c echo.Context, ctx context.Context, req *ChatCompletionRequest, runID, requestID string, startTime time.Time) error {
	c.Response().Header().Set("Content-Type", "text/event-stream")
	c.Response().Header().Set("Cache-Control", "no-cache")
	c.Response().Header().Set("Connection", "keep-alive")
	c.Response().WriteHeader(http.StatusOK)

	flusher, ok := c.Response().Writer.(http.Flusher)
	if !ok {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: &APIError{
				Message: "streaming not supported",
				Type:    "internal_error",
			},
		})
	}

	var totalPromptTokens, totalCompletionTokens int
	var responseModel string

	usage, err := h.client.CreateChatCompletionStream(ctx, req, func(chunk *StreamChunk) error {
		if responseModel == "" && chunk.Model != "" {
			responseModel = chunk.Model
		}

		// Forward the chunk as SSE
		data, err := json.Marshal(chunk)
		if err != nil {
			return err
		}

		_, writeErr := fmt.Fprintf(c.Response().Writer, "data: %s\n\n", data)
		if writeErr != nil {
			return writeErr
		}
		flusher.Flush()
		return nil
	})

	// Write [DONE] marker
	fmt.Fprintf(c.Response().Writer, "data: [DONE]\n\n")
	flusher.Flush()

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
		} else {
			payload.PromptTokens = totalPromptTokens
			payload.CompletionTokens = totalCompletionTokens
		}
		if err != nil {
			payload.Error = err.Error()
		}
		if recordErr := h.recordEvent(ctx, runID, domain.EventTypeLLMCallDone, payload); recordErr != nil {
			log.Printf("WARN: failed to record llm_call_done event: %v", recordErr)
		}
	}

	if err != nil {
		log.Printf("ERROR: LLM streaming request failed: %v", err)
	}

	return nil
}

// ListModels handles the models list request.
// GET /v1/models
func (h *Handler) ListModels(c echo.Context) error {
	ctx := c.Request().Context()

	models, err := h.client.ListModels(ctx)
	if err != nil {
		log.Printf("ERROR: failed to list models: %v", err)
		return c.JSON(http.StatusBadGateway, ErrorResponse{
			Error: &APIError{
				Message: err.Error(),
				Type:    "upstream_error",
			},
		})
	}

	return c.JSON(http.StatusOK, ModelsResponse{
		Object: "list",
		Data:   models,
	})
}

// recordEvent records an event to the store.
func (h *Handler) recordEvent(ctx context.Context, runID string, eventType domain.EventType, payload interface{}) error {
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	event := &domain.Event{
		EventID: "evt_" + uuid.New().String()[:8],
		RunID:   runID,
		Ts:      time.Now().UnixMilli(),
		Type:    eventType,
		Payload: payloadJSON,
	}

	return h.store.CreateEvent(ctx, event)
}
