// Package agentclient provides HTTP client for invoking external agents with SSE streaming.
package agentclient

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gogo/orchestrator/domain"
)

// SSEEvent represents a parsed SSE event.
type SSEEvent struct {
	Event string
	Data  string
}

// EventHandler is called for each SSE event from the agent.
type EventHandler func(event SSEEvent) error

// Client is an HTTP client for invoking agents.
type Client struct {
	httpClient *http.Client
}

// NewClient creates a new agent client.
func NewClient() *Client {
	return &Client{
		httpClient: &http.Client{
			Timeout: 5 * time.Minute, // Long timeout for streaming
		},
	}
}

// Invoke calls an agent's /invoke endpoint and streams SSE events.
func (c *Client) Invoke(ctx context.Context, endpoint string, req *domain.AgentInvokeRequest, handler EventHandler) error {
	// Prepare request body
	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	url := strings.TrimSuffix(endpoint, "/") + "/invoke"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	httpReq.Header.Set("X-Session-ID", req.SessionID)
	httpReq.Header.Set("X-Run-ID", req.RunID)

	// Execute request
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("failed to invoke agent: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("agent returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	// Parse SSE stream
	return c.parseSSE(resp.Body, handler)
}

// parseSSE parses an SSE stream and calls the handler for each event.
func (c *Client) parseSSE(reader io.Reader, handler EventHandler) error {
	scanner := bufio.NewScanner(reader)
	var event SSEEvent

	for scanner.Scan() {
		line := scanner.Text()

		// Empty line marks end of event
		if line == "" {
			if event.Event != "" || event.Data != "" {
				if err := handler(event); err != nil {
					return err
				}
				event = SSEEvent{}
			}
			continue
		}

		// Parse event/data lines
		if strings.HasPrefix(line, "event:") {
			event.Event = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		} else if strings.HasPrefix(line, "data:") {
			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if event.Data != "" {
				event.Data += "\n" + data
			} else {
				event.Data = data
			}
		}
		// Ignore comments (lines starting with :) and other fields
	}

	// Handle any remaining event
	if event.Event != "" || event.Data != "" {
		if err := handler(event); err != nil {
			return err
		}
	}

	return scanner.Err()
}

// ParseDeltaEvent parses a delta event data.
func ParseDeltaEvent(data string) (*domain.DeltaEventData, error) {
	var delta domain.DeltaEventData
	if err := json.Unmarshal([]byte(data), &delta); err != nil {
		return nil, fmt.Errorf("failed to parse delta event: %w", err)
	}
	return &delta, nil
}

// ParseDoneEvent parses a done event data.
func ParseDoneEvent(data string) (*domain.DoneEventData, error) {
	var done domain.DoneEventData
	if err := json.Unmarshal([]byte(data), &done); err != nil {
		return nil, fmt.Errorf("failed to parse done event: %w", err)
	}
	return &done, nil
}

// ParseErrorEvent parses an error event data.
func ParseErrorEvent(data string) (*domain.ErrorEventData, error) {
	var errEvt domain.ErrorEventData
	if err := json.Unmarshal([]byte(data), &errEvt); err != nil {
		return nil, fmt.Errorf("failed to parse error event: %w", err)
	}
	return &errEvt, nil
}
