package rpc

import (
	"context"
	"errors"
	"log"
	"net"
	"net/rpc"
	"net/rpc/jsonrpc"
	"time"

	"github.com/xiaot623/gogo/ingress/internal/hub"
)

// Server exposes ingress RPC endpoints.
type Server struct {
	listener  net.Listener
	rpcServer *rpc.Server
	done      chan struct{}
}

// NewServer creates a new ingress RPC server.
func NewServer(h *hub.Hub) (*Server, error) {
	rpcServer := rpc.NewServer()
	handler := &Handler{hub: h}
	if err := rpcServer.RegisterName("Ingress", handler); err != nil {
		return nil, err
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

// Handler implements ingress RPC methods.
type Handler struct {
	hub *hub.Hub
}

// SendRequest represents the request body for event delivery.
type SendRequest struct {
	SessionID string                 `json:"session_id"`
	Event     map[string]interface{} `json:"event"`
}

// SendResponse represents the response for event delivery.
type SendResponse struct {
	OK        bool `json:"ok"`
	Delivered bool `json:"delivered"`
}

// PushEvent forwards events from the orchestrator to WebSocket clients.
func (h *Handler) PushEvent(req *SendRequest, resp *SendResponse) error {
	if req == nil {
		return errors.New("send request is required")
	}
	if req.SessionID == "" {
		return errors.New("session_id is required")
	}
	if req.Event == nil {
		return errors.New("event is required")
	}

	if _, ok := req.Event["ts"]; !ok {
		req.Event["ts"] = time.Now().UnixMilli()
	}

	hasConnections := h.hub.HasActiveConnections(req.SessionID)
	if err := h.hub.BroadcastJSON(req.SessionID, req.Event); err != nil {
		return err
	}

	log.Printf("Event sent to session %s: type=%v, delivered=%v", req.SessionID, req.Event["type"], hasConnections)

	if resp != nil {
		resp.OK = true
		resp.Delivered = hasConnections
	}
	return nil
}
