package llmproxy

import (
	"encoding/json"
	"fmt"
	"net/http"
	"context"

	"github.com/labstack/echo/v4"
	"github.com/xiaot623/gogo/orchestrator/internal/adapter/llm"
	"github.com/xiaot623/gogo/orchestrator/internal/service"
)

// Handler handles LLM proxy HTTP requests.
type Handler struct {
	service *service.Service
}

// NewHandler creates a new LLM proxy handler.
func NewHandler(service *service.Service) *Handler {
	return &Handler{
		service: service,
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

	var req llm.ChatCompletionRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, llm.ErrorResponse{
			Error: &llm.APIError{
				Message: "invalid request body",
				Type:    "invalid_request_error",
			},
		})
	}

	// Validate required fields
	if req.Model == "" {
		return c.JSON(http.StatusBadRequest, llm.ErrorResponse{
			Error: &llm.APIError{
				Message: "model is required",
				Type:    "invalid_request_error",
				Param:   "model",
			},
		})
	}
	if len(req.Messages) == 0 {
		return c.JSON(http.StatusBadRequest, llm.ErrorResponse{
			Error: &llm.APIError{
				Message: "messages is required",
				Type:    "invalid_request_error",
				Param:   "messages",
			},
		})
	}

	if req.Stream {
		return h.handleStreamingRequest(c, ctx, runID, &req)
	}

	return h.handleNonStreamingRequest(c, ctx, runID, &req)
}

// handleNonStreamingRequest handles non-streaming chat completion requests.
func (h *Handler) handleNonStreamingRequest(c echo.Context, ctx context.Context, runID string, req *llm.ChatCompletionRequest) error {
	resp, err := h.service.ProxyChatCompletion(ctx, runID, req)
	if err != nil {
		// Error handling could be improved to map to OpenAI error types
		return c.JSON(http.StatusBadGateway, llm.ErrorResponse{
			Error: &llm.APIError{
				Message: err.Error(),
				Type:    "upstream_error",
			},
		})
	}

	return c.JSON(http.StatusOK, resp)
}

// handleStreamingRequest handles streaming chat completion requests.
func (h *Handler) handleStreamingRequest(c echo.Context, ctx context.Context, runID string, req *llm.ChatCompletionRequest) error {
	c.Response().Header().Set("Content-Type", "text/event-stream")
	c.Response().Header().Set("Cache-Control", "no-cache")
	c.Response().Header().Set("Connection", "keep-alive")
	c.Response().WriteHeader(http.StatusOK)

	flusher, ok := c.Response().Writer.(http.Flusher)
	if !ok {
		return c.JSON(http.StatusInternalServerError, llm.ErrorResponse{
			Error: &llm.APIError{
				Message: "streaming not supported",
				Type:    "internal_error",
			},
		})
	}
	
	err := h.service.ProxyChatCompletionStream(ctx, runID, req, func(chunk *llm.StreamChunk) error {
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

	if err != nil {
		// Can't change status code after writing response
		// Just log it
		fmt.Printf("ERROR: LLM streaming request failed: %v\n", err)
	}

	return nil
}

// ListModels handles the models list request.
// GET /v1/models
func (h *Handler) ListModels(c echo.Context) error {
	ctx := c.Request().Context()

	models, err := h.service.ListModels(ctx)
	if err != nil {
		return c.JSON(http.StatusBadGateway, llm.ErrorResponse{
			Error: &llm.APIError{
				Message: err.Error(),
				Type:    "upstream_error",
			},
		})
	}

	return c.JSON(http.StatusOK, llm.ModelsResponse{
		Object: "list",
		Data:   models,
	})
}
