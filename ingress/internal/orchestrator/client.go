// Package orchestrator provides an HTTP client for the orchestrator internal API.
package orchestrator

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client is an HTTP client for the orchestrator internal API.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// NewClient creates a new orchestrator client.
func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: strings.TrimSuffix(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// InvokeRequest represents the request to invoke an agent.
type InvokeRequest struct {
	SessionID    string            `json:"session_id"`
	AgentID      string            `json:"agent_id"`
	InputMessage InputMessage      `json:"input_message"`
	RequestID    string            `json:"request_id,omitempty"`
	Context      map[string]string `json:"context,omitempty"`
}

// InputMessage represents the input message content.
type InputMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// InvokeResponse represents the response from invoking an agent.
type InvokeResponse struct {
	RunID     string `json:"run_id"`
	SessionID string `json:"session_id"`
	AgentID   string `json:"agent_id"`
}

// ToolCallResultRequest represents a request to submit a tool call result.
type ToolCallResultRequest struct {
	Status string          `json:"status"` // SUCCEEDED or FAILED
	Result json.RawMessage `json:"result,omitempty"`
	Error  json.RawMessage `json:"error,omitempty"`
}

// ToolCallResultResponse represents the response after submitting a tool call result.
type ToolCallResultResponse struct {
	ToolCallID  string          `json:"tool_call_id"`
	Status      string          `json:"status"`
	Result      json.RawMessage `json:"result,omitempty"`
	Error       json.RawMessage `json:"error,omitempty"`
	CompletedAt int64           `json:"completed_at"`
}

// ApprovalDecisionRequest represents a decision on an approval.
type ApprovalDecisionRequest struct {
	Decision  string `json:"decision"` // APPROVED or REJECTED
	Reason    string `json:"reason,omitempty"`
	DecidedBy string `json:"decided_by,omitempty"`
}

// ApprovalDecisionResponse represents the response after submitting an approval decision.
type ApprovalDecisionResponse struct {
	ApprovalID     string          `json:"approval_id"`
	Status         string          `json:"status"`
	ToolCallID     string          `json:"tool_call_id"`
	ToolCallStatus string          `json:"tool_call_status"`
	Result         json.RawMessage `json:"result,omitempty"`
	Error          json.RawMessage `json:"error,omitempty"`
}

// CancelRunResponse represents the response from canceling a run.
type CancelRunResponse struct {
	RunID  string `json:"run_id"`
	Status string `json:"status"`
}

// ErrorResponse represents an error response from the orchestrator.
type ErrorResponse struct {
	Error string `json:"error"`
}

// Invoke calls POST /internal/invoke on the orchestrator.
func (c *Client) Invoke(ctx context.Context, req *InvokeRequest) (*InvokeResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal invoke request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/internal/invoke", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to invoke orchestrator: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		var errResp ErrorResponse
		if json.Unmarshal(respBody, &errResp) == nil && errResp.Error != "" {
			return nil, fmt.Errorf("orchestrator error: %s", errResp.Error)
		}
		return nil, fmt.Errorf("orchestrator returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var invokeResp InvokeResponse
	if err := json.NewDecoder(resp.Body).Decode(&invokeResp); err != nil {
		return nil, fmt.Errorf("failed to decode invoke response: %w", err)
	}

	return &invokeResp, nil
}

// SubmitToolResult calls POST /internal/tool_calls/:tool_call_id/submit on the orchestrator.
func (c *Client) SubmitToolResult(ctx context.Context, toolCallID string, req *ToolCallResultRequest) (*ToolCallResultResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal tool result request: %w", err)
	}

	url := fmt.Sprintf("%s/internal/tool_calls/%s/submit", c.baseURL, toolCallID)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to submit tool result: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		var errResp ErrorResponse
		if json.Unmarshal(respBody, &errResp) == nil && errResp.Error != "" {
			return nil, fmt.Errorf("orchestrator error: %s", errResp.Error)
		}
		return nil, fmt.Errorf("orchestrator returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var resultResp ToolCallResultResponse
	if err := json.NewDecoder(resp.Body).Decode(&resultResp); err != nil {
		return nil, fmt.Errorf("failed to decode tool result response: %w", err)
	}

	return &resultResp, nil
}

// SubmitApprovalDecision calls POST /internal/approvals/:approval_id/submit on the orchestrator.
func (c *Client) SubmitApprovalDecision(ctx context.Context, approvalID string, req *ApprovalDecisionRequest) (*ApprovalDecisionResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal approval decision request: %w", err)
	}

	url := fmt.Sprintf("%s/internal/approvals/%s/submit", c.baseURL, approvalID)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to submit approval decision: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		var errResp ErrorResponse
		if json.Unmarshal(respBody, &errResp) == nil && errResp.Error != "" {
			return nil, fmt.Errorf("orchestrator error: %s", errResp.Error)
		}
		return nil, fmt.Errorf("orchestrator returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var decisionResp ApprovalDecisionResponse
	if err := json.NewDecoder(resp.Body).Decode(&decisionResp); err != nil {
		return nil, fmt.Errorf("failed to decode approval decision response: %w", err)
	}

	return &decisionResp, nil
}

// CancelRun calls POST /internal/runs/:run_id/cancel on the orchestrator.
func (c *Client) CancelRun(ctx context.Context, runID string) (*CancelRunResponse, error) {
	url := fmt.Sprintf("%s/internal/runs/%s/cancel", c.baseURL, runID)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to cancel run: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		var errResp ErrorResponse
		if json.Unmarshal(respBody, &errResp) == nil && errResp.Error != "" {
			return nil, fmt.Errorf("orchestrator error: %s", errResp.Error)
		}
		return nil, fmt.Errorf("orchestrator returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var cancelResp CancelRunResponse
	if err := json.NewDecoder(resp.Body).Decode(&cancelResp); err != nil {
		return nil, fmt.Errorf("failed to decode cancel response: %w", err)
	}

	return &cancelResp, nil
}
