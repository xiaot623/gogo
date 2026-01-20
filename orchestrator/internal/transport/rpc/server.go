package rpc

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/rpc"
	"net/rpc/jsonrpc"
	"strings"

	"github.com/xiaot623/gogo/orchestrator/internal/domain"
	"github.com/xiaot623/gogo/orchestrator/internal/service"
)

// Server exposes internal RPC endpoints for ingress and other internal clients.
type Server struct {
	listener  net.Listener
	rpcServer *rpc.Server
	done      chan struct{}
}

// NewServer creates a new RPC server bound to the orchestrator service.
func NewServer(svc *service.Service) (*Server, error) {
	rpcServer := rpc.NewServer()
	handler := &Handler{service: svc}
	if err := rpcServer.RegisterName("Orchestrator", handler); err != nil {
		return nil, fmt.Errorf("register rpc handler: %w", err)
	}

	return &Server{
		rpcServer: rpcServer,
		done:      make(chan struct{}),
	}, nil
}

// Start begins accepting RPC connections on the given address.
func (s *Server) Start(addr string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	s.listener = ln

	for {
		conn, err := ln.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				close(s.done)
				return nil
			}
			log.Printf("RPC accept error: %v", err)
			continue
		}

		go s.rpcServer.ServeCodec(jsonrpc.NewServerCodec(conn))
	}
}

// Shutdown stops accepting new RPC connections.
func (s *Server) Shutdown(ctx context.Context) error {
	if s.listener == nil {
		return nil
	}

	if err := s.listener.Close(); err != nil {
		return err
	}

	select {
	case <-s.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Handler implements orchestrator RPC methods.
type Handler struct {
	service *service.Service
}

// ToolCallResultArgs wraps tool call IDs with the tool result payload.
type ToolCallResultArgs struct {
	ToolCallID string                       `json:"tool_call_id"`
	Request    domain.ToolCallResultRequest `json:"request"`
}

// ApprovalDecisionArgs wraps approval IDs with the decision payload.
type ApprovalDecisionArgs struct {
	ApprovalID string                         `json:"approval_id"`
	Request    domain.ApprovalDecisionRequest `json:"request"`
}

// CancelRunRequest identifies a run to cancel.
type CancelRunRequest struct {
	RunID string `json:"run_id"`
}

// CancelRunResponse is returned after a run cancellation request.
type CancelRunResponse struct {
	RunID   string           `json:"run_id"`
	Status  domain.RunStatus `json:"status"`
	Message string           `json:"message"`
}

// AckResponse is a generic OK response.
type AckResponse struct {
	OK bool `json:"ok"`
}

// Invoke invokes an agent run.
func (h *Handler) Invoke(req *domain.InvokeRequest, resp *domain.InvokeResponse) error {
	if req == nil {
		return errors.New("invoke request is required")
	}

	result, err := h.service.InvokeAgent(context.Background(), *req)
	if err != nil {
		return err
	}
	if resp != nil && result != nil {
		*resp = *result
	}
	return nil
}

// RegisterTools registers tools from a client.
func (h *Handler) RegisterTools(req *domain.ToolRegistrationRequest, resp *domain.ToolRegistrationResponse) error {
	if req == nil {
		return errors.New("tool registration request is required")
	}
	if req.ClientID == "" {
		return errors.New("client_id is required")
	}
	if len(req.Tools) == 0 {
		return errors.New("tools array is required")
	}

	result, err := h.service.RegisterTools(context.Background(), *req)
	if err != nil {
		return err
	}
	if resp != nil {
		*resp = *result
	}
	return nil
}

// SubmitToolResult submits a tool call result.
func (h *Handler) SubmitToolResult(req *ToolCallResultArgs, resp *domain.ToolCallResultResponse) error {
	if req == nil {
		return errors.New("tool result request is required")
	}
	if req.ToolCallID == "" {
		return errors.New("tool_call_id is required")
	}
	if req.Request.Status != "SUCCEEDED" && req.Request.Status != "FAILED" {
		return errors.New("status must be SUCCEEDED or FAILED")
	}

	result, err := h.service.SubmitToolResult(context.Background(), req.ToolCallID, req.Request)
	if err != nil {
		return err
	}
	if resp != nil && result != nil {
		*resp = *result
	}
	return nil
}

// SubmitApprovalDecision records an approval decision.
func (h *Handler) SubmitApprovalDecision(req *ApprovalDecisionArgs, resp *AckResponse) error {
	if req == nil {
		return errors.New("approval decision request is required")
	}
	if req.ApprovalID == "" {
		return errors.New("approval_id is required")
	}

	decision := normalizeDecision(req.Request.Decision)
	if decision == "" {
		return errors.New("decision must be approve or reject")
	}
	req.Request.Decision = decision

	if err := h.service.UpdateApproval(context.Background(), req.ApprovalID, req.Request); err != nil {
		return err
	}
	if resp != nil {
		resp.OK = true
	}
	return nil
}

// CancelRun cancels a running execution.
func (h *Handler) CancelRun(req *CancelRunRequest, resp *CancelRunResponse) error {
	if req == nil {
		return errors.New("cancel request is required")
	}
	if req.RunID == "" {
		return errors.New("run_id is required")
	}

	if err := h.service.CancelRun(context.Background(), req.RunID); err != nil {
		return err
	}
	if resp != nil {
		resp.RunID = req.RunID
		resp.Status = domain.RunStatusCancelled
		resp.Message = "run cancelled successfully"
	}
	return nil
}

func normalizeDecision(decision string) string {
	switch strings.ToLower(strings.TrimSpace(decision)) {
	case "approve", "approved":
		return "approve"
	case "reject", "rejected":
		return "reject"
	default:
		return ""
	}
}
