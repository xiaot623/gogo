// Package main provides a simple CLI client for connecting to the ingress WebSocket server.
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

// Message types
const (
	TypeHello       = "hello"
	TypeHelloAck    = "hello_ack"
	TypeAgentInvoke = "agent_invoke"
	TypeDelta       = "delta"
	TypeDone        = "done"
	TypeError       = "error"
)

// BaseMessage contains common fields for all messages.
type BaseMessage struct {
	Type      string `json:"type"`
	Ts        int64  `json:"ts"`
	RequestID string `json:"request_id,omitempty"`
	SessionID string `json:"session_id,omitempty"`
	RunID     string `json:"run_id,omitempty"`
}

// HelloMessage is sent to establish connection.
type HelloMessage struct {
	BaseMessage
	UserID     string            `json:"user_id,omitempty"`
	APIKey     string            `json:"api_key,omitempty"`
	ClientMeta map[string]string `json:"client_meta,omitempty"`
}

// AgentInvokeMessage is sent to invoke an agent.
type AgentInvokeMessage struct {
	BaseMessage
	AgentID string       `json:"agent_id"`
	Message InputMessage `json:"message"`
}

// InputMessage represents the input message content.
type InputMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ErrorMessage represents an error from the server.
type ErrorMessage struct {
	BaseMessage
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Client represents a WebSocket client.
type Client struct {
	conn      *websocket.Conn
	sessionID string
	done      chan struct{}
}

// NewClient creates a new client and connects to the server.
func NewClient(addr string) (*Client, error) {
	conn, _, err := websocket.DefaultDialer.Dial(addr, nil)
	if err != nil {
		return nil, fmt.Errorf("dial: %w", err)
	}

	return &Client{
		conn: conn,
		done: make(chan struct{}),
	}, nil
}

// Close closes the client connection.
func (c *Client) Close() error {
	close(c.done)
	return c.conn.Close()
}

// SendHello sends a hello message and waits for hello_ack.
func (c *Client) SendHello(apiKey string) error {
	msg := HelloMessage{
		BaseMessage: BaseMessage{
			Type: TypeHello,
			Ts:   time.Now().UnixMilli(),
		},
		APIKey: apiKey,
		ClientMeta: map[string]string{
			"client": "gogo-cli",
		},
	}

	if err := c.conn.WriteJSON(msg); err != nil {
		return fmt.Errorf("write hello: %w", err)
	}

	// Wait for hello_ack
	_, data, err := c.conn.ReadMessage()
	if err != nil {
		return fmt.Errorf("read hello_ack: %w", err)
	}

	var base BaseMessage
	if err := json.Unmarshal(data, &base); err != nil {
		return fmt.Errorf("unmarshal hello_ack: %w", err)
	}

	if base.Type == TypeError {
		var errMsg ErrorMessage
		json.Unmarshal(data, &errMsg)
		return fmt.Errorf("hello failed: %s - %s", errMsg.Code, errMsg.Message)
	}

	if base.Type != TypeHelloAck {
		return fmt.Errorf("expected hello_ack, got: %s", base.Type)
	}

	c.sessionID = base.SessionID
	return nil
}

// SendAgentInvoke sends an agent invoke message.
func (c *Client) SendAgentInvoke(agentID, content string) error {
	msg := AgentInvokeMessage{
		BaseMessage: BaseMessage{
			Type:      TypeAgentInvoke,
			Ts:        time.Now().UnixMilli(),
			SessionID: c.sessionID,
			RequestID: fmt.Sprintf("req_%d", time.Now().UnixNano()),
		},
		AgentID: agentID,
		Message: InputMessage{
			Role:    "user",
			Content: content,
		},
	}

	return c.conn.WriteJSON(msg)
}

// ReadMessages reads and prints messages from the server.
func (c *Client) ReadMessages() {
	for {
		select {
		case <-c.done:
			return
		default:
			_, data, err := c.conn.ReadMessage()
			if err != nil {
				if !websocket.IsCloseError(err, websocket.CloseNormalClosure) {
					log.Printf("Read error: %v", err)
				}
				return
			}

			var base BaseMessage
			if err := json.Unmarshal(data, &base); err != nil {
				log.Printf("Unmarshal error: %v", err)
				continue
			}

			// Pretty print the message
			var prettyJSON map[string]interface{}
			json.Unmarshal(data, &prettyJSON)
			formatted, _ := json.MarshalIndent(prettyJSON, "", "  ")
			fmt.Printf("\n[%s] Received:\n%s\n", base.Type, string(formatted))
		}
	}
}

func main() {
	addr := flag.String("addr", "ws://localhost:8090/ws", "WebSocket server address")
	apiKey := flag.String("api-key", "", "API key for authentication")
	agentID := flag.String("agent", "default", "Agent ID to invoke")
	flag.Parse()

	log.SetFlags(log.Ltime)

	fmt.Printf("Connecting to %s...\n", *addr)

	client, err := NewClient(*addr)
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	defer client.Close()

	fmt.Println("Connected. Sending hello...")

	if err := client.SendHello(*apiKey); err != nil {
		log.Fatalf("Hello failed: %v", err)
	}

	fmt.Printf("Session established: %s\n", client.sessionID)
	fmt.Println("\nType a message and press Enter to send.")
	fmt.Println("Commands: /quit to exit\n")

	// Start reading messages in background
	go client.ReadMessages()

	// Handle Ctrl+C
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)

	// Read user input
	scanner := bufio.NewScanner(os.Stdin)

	for {
		fmt.Print("> ")
		select {
		case <-interrupt:
			fmt.Println("\nInterrupted")
			return
		default:
			if !scanner.Scan() {
				return
			}

			input := strings.TrimSpace(scanner.Text())
			if input == "" {
				continue
			}

			if input == "/quit" {
				fmt.Println("Bye!")
				return
			}

			if err := client.SendAgentInvoke(*agentID, input); err != nil {
				log.Printf("Send error: %v", err)
				continue
			}

			fmt.Println("Message sent, waiting for response...")
		}
	}
}
