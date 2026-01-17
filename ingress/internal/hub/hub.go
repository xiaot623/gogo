// Package hub provides connection management for WebSocket clients.
package hub

import (
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

// Connection represents a single WebSocket connection.
type Connection struct {
	ID        string
	SessionID string
	Conn      *websocket.Conn
	Send      chan []byte
	hub       *Hub
	mu        sync.Mutex
}

// Hub manages all WebSocket connections.
type Hub struct {
	// Connections indexed by connection ID
	connections map[string]*Connection

	// Sessions maps session_id to set of connection IDs
	sessions map[string]map[string]bool

	// Channels for registration/unregistration
	register   chan *Connection
	unregister chan *Connection

	// Broadcast channel for sending to specific session
	broadcast chan *SessionMessage

	mu sync.RWMutex
}

// SessionMessage is used to broadcast a message to a session.
type SessionMessage struct {
	SessionID string
	Data      []byte
}

// NewHub creates a new Hub.
func NewHub() *Hub {
	return &Hub{
		connections: make(map[string]*Connection),
		sessions:    make(map[string]map[string]bool),
		register:    make(chan *Connection),
		unregister:  make(chan *Connection),
		broadcast:   make(chan *SessionMessage, 256),
	}
}

// Run starts the hub's main loop.
func (h *Hub) Run() {
	for {
		select {
		case conn := <-h.register:
			h.mu.Lock()
			h.connections[conn.ID] = conn
			if conn.SessionID != "" {
				if h.sessions[conn.SessionID] == nil {
					h.sessions[conn.SessionID] = make(map[string]bool)
				}
				h.sessions[conn.SessionID][conn.ID] = true
			}
			h.mu.Unlock()
			log.Printf("Connection registered: %s (session: %s)", conn.ID, conn.SessionID)

		case conn := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.connections[conn.ID]; ok {
				delete(h.connections, conn.ID)
				if conn.SessionID != "" && h.sessions[conn.SessionID] != nil {
					delete(h.sessions[conn.SessionID], conn.ID)
					if len(h.sessions[conn.SessionID]) == 0 {
						delete(h.sessions, conn.SessionID)
					}
				}
				close(conn.Send)
			}
			h.mu.Unlock()
			log.Printf("Connection unregistered: %s", conn.ID)

		case msg := <-h.broadcast:
			h.mu.RLock()
			if connIDs, ok := h.sessions[msg.SessionID]; ok {
				for connID := range connIDs {
					if conn, exists := h.connections[connID]; exists {
						select {
						case conn.Send <- msg.Data:
						default:
							// Buffer full, close the connection
							log.Printf("Connection %s buffer full, closing", connID)
							go h.Unregister(conn)
						}
					}
				}
			}
			h.mu.RUnlock()
		}
	}
}

// NewConnection creates a new connection and registers it with the hub.
func (h *Hub) NewConnection(ws *websocket.Conn) *Connection {
	conn := &Connection{
		ID:   uuid.New().String(),
		Conn: ws,
		Send: make(chan []byte, 256),
		hub:  h,
	}
	return conn
}

// Register registers a connection with the hub.
func (h *Hub) Register(conn *Connection) {
	h.register <- conn
}

// Unregister unregisters a connection from the hub.
func (h *Hub) Unregister(conn *Connection) {
	h.unregister <- conn
}

// BindSession binds a connection to a session.
func (h *Hub) BindSession(conn *Connection, sessionID string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Remove from old session if any
	if conn.SessionID != "" && h.sessions[conn.SessionID] != nil {
		delete(h.sessions[conn.SessionID], conn.ID)
		if len(h.sessions[conn.SessionID]) == 0 {
			delete(h.sessions, conn.SessionID)
		}
	}

	// Add to new session
	conn.SessionID = sessionID
	if h.sessions[sessionID] == nil {
		h.sessions[sessionID] = make(map[string]bool)
	}
	h.sessions[sessionID][conn.ID] = true
}

// Broadcast sends a message to all connections of a session.
func (h *Hub) Broadcast(sessionID string, data []byte) {
	h.broadcast <- &SessionMessage{
		SessionID: sessionID,
		Data:      data,
	}
}

// BroadcastJSON sends a JSON message to all connections of a session.
func (h *Hub) BroadcastJSON(sessionID string, v interface{}) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	h.Broadcast(sessionID, data)
	return nil
}

// SendToConnection sends a message to a specific connection.
func (h *Hub) SendToConnection(conn *Connection, data []byte) error {
	select {
	case conn.Send <- data:
		return nil
	default:
		return ErrBufferFull
	}
}

// SendJSONToConnection sends a JSON message to a specific connection.
func (h *Hub) SendJSONToConnection(conn *Connection, v interface{}) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return h.SendToConnection(conn, data)
}

// GetConnectionCount returns the number of active connections.
func (h *Hub) GetConnectionCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.connections)
}

// GetSessionCount returns the number of active sessions.
func (h *Hub) GetSessionCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.sessions)
}

// HasActiveConnections checks if a session has any active connections.
func (h *Hub) HasActiveConnections(sessionID string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	connIDs, ok := h.sessions[sessionID]
	return ok && len(connIDs) > 0
}

// WriteMessage writes a message to the connection with proper locking.
func (c *Connection) WriteMessage(messageType int, data []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.Conn.WriteMessage(messageType, data)
}

// SetWriteDeadline sets the write deadline for the connection.
func (c *Connection) SetWriteDeadline(t time.Time) error {
	return c.Conn.SetWriteDeadline(t)
}

// SetReadDeadline sets the read deadline for the connection.
func (c *Connection) SetReadDeadline(t time.Time) error {
	return c.Conn.SetReadDeadline(t)
}

// Close closes the connection.
func (c *Connection) Close() error {
	return c.Conn.Close()
}

// ErrBufferFull is returned when the send buffer is full.
var ErrBufferFull = &BufferFullError{}

// BufferFullError represents a buffer full error.
type BufferFullError struct{}

func (e *BufferFullError) Error() string {
	return "send buffer full"
}
