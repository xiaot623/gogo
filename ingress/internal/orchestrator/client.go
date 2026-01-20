// Package orchestrator provides an RPC client for the orchestrator internal API.
package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/rpc/jsonrpc"
	"net/url"
	"strings"
	"time"
)

// Client is an RPC client for the orchestrator internal API.
type Client struct {
	addr        string
	dialTimeout time.Duration
	callTimeout time.Duration
}

// NewClient creates a new orchestrator client.
func NewClient(baseURL string) *Client {
	return &Client{
		addr:        resolveRPCAddr(baseURL),
		dialTimeout: 5 * time.Second,
		callTimeout: 30 * time.Second,
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

// ToolCallResultArgs wraps tool call IDs with the tool result payload.
type ToolCallResultArgs struct {
	ToolCallID string                `json:"tool_call_id"`
	Request    ToolCallResultRequest `json:"request"`
}

// ApprovalDecisionArgs wraps approval IDs with the decision payload.
type ApprovalDecisionArgs struct {
	ApprovalID string                  `json:"approval_id"`
	Request    ApprovalDecisionRequest `json:"request"`
}

// CancelRunRequest identifies a run to cancel.
type CancelRunRequest struct {
	RunID string `json:"run_id"`
}

// AckResponse is a generic OK response.
type AckResponse struct {
	OK bool `json:"ok"`
}

// Invoke calls orchestrator Invoke over RPC.
func (c *Client) Invoke(ctx context.Context, req *InvokeRequest) (*InvokeResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("invoke request is required")
	}

	var invokeResp InvokeResponse
	if err := c.call(ctx, "Orchestrator.Invoke", req, &invokeResp); err != nil {
		return nil, fmt.Errorf("failed to invoke orchestrator: %w", err)
	}

	return &invokeResp, nil
}

// SubmitToolResult calls orchestrator SubmitToolResult over RPC.
func (c *Client) SubmitToolResult(ctx context.Context, toolCallID string, req *ToolCallResultRequest) (*ToolCallResultResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("tool result request is required")
	}

	args := &ToolCallResultArgs{
		ToolCallID: toolCallID,
		Request:    *req,
	}

	var resultResp ToolCallResultResponse
	if err := c.call(ctx, "Orchestrator.SubmitToolResult", args, &resultResp); err != nil {
		return nil, fmt.Errorf("failed to submit tool result: %w", err)
	}

	return &resultResp, nil
}

// SubmitApprovalDecision calls orchestrator SubmitApprovalDecision over RPC.
func (c *Client) SubmitApprovalDecision(ctx context.Context, approvalID string, req *ApprovalDecisionRequest) (*ApprovalDecisionResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("approval decision request is required")
	}

	args := &ApprovalDecisionArgs{
		ApprovalID: approvalID,
		Request:    *req,
	}

	var ack AckResponse
	if err := c.call(ctx, "Orchestrator.SubmitApprovalDecision", args, &ack); err != nil {
		return nil, fmt.Errorf("failed to submit approval decision: %w", err)
	}

	return &ApprovalDecisionResponse{}, nil
}

// CancelRun calls orchestrator CancelRun over RPC.
func (c *Client) CancelRun(ctx context.Context, runID string) (*CancelRunResponse, error) {
	args := &CancelRunRequest{RunID: runID}

	var cancelResp CancelRunResponse
	if err := c.call(ctx, "Orchestrator.CancelRun", args, &cancelResp); err != nil {
		return nil, fmt.Errorf("failed to cancel run: %w", err)
	}

	return &cancelResp, nil
}

func (c *Client) call(ctx context.Context, method string, args, reply interface{}) error {
	if c.addr == "" {
		return fmt.Errorf("orchestrator rpc address is empty")
	}

	conn, err := net.DialTimeout("tcp", c.addr, c.dialTimeout)
	if err != nil {
		return err
	}
	defer conn.Close()

	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	} else if c.callTimeout > 0 {
		_ = conn.SetDeadline(time.Now().Add(c.callTimeout))
	}

	client := jsonrpc.NewClient(conn)
	call := client.Go(method, args, reply, nil)

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-call.Done:
		return call.Error
	}
}

func resolveRPCAddr(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if strings.Contains(raw, "://") {
		parsed, err := url.Parse(raw)
		if err == nil && parsed.Host != "" {
			return parsed.Host
		}
	}
	return raw
}
