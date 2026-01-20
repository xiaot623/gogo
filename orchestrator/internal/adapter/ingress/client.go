package ingress

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/rpc/jsonrpc"
	"net/url"
	"strings"
	"time"
)

type Client struct {
	addr        string
	dialTimeout time.Duration
	callTimeout time.Duration
}

func NewClient(baseURL string) *Client {
	return &Client{
		addr:        resolveRPCAddr(baseURL),
		dialTimeout: 5 * time.Second,
		callTimeout: 5 * time.Second,
	}
}

// SendRequest represents the request body for internal event delivery.
type SendRequest struct {
	SessionID string                 `json:"session_id"`
	Event     map[string]interface{} `json:"event"`
}

// SendResponse represents the response for internal event delivery.
type SendResponse struct {
	OK        bool `json:"ok"`
	Delivered bool `json:"delivered"`
}

func (c *Client) PushEvent(sessionID string, event map[string]interface{}) error {
	if c.addr == "" {
		return nil
	}

	req := &SendRequest{
		SessionID: sessionID,
		Event:     event,
	}

	var resp SendResponse
	ctx, cancel := context.WithTimeout(context.Background(), c.callTimeout)
	defer cancel()

	if err := c.call(ctx, "Ingress.PushEvent", req, &resp); err != nil {
		return fmt.Errorf("failed to push event to ingress: %w", err)
	}
	if !resp.OK {
		log.Printf("WARN: ingress rpc returned ok=false (delivered=%v)", resp.Delivered)
		return fmt.Errorf("ingress rpc returned ok=false")
	}

	return nil
}

func (c *Client) call(ctx context.Context, method string, args, reply interface{}) error {
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
