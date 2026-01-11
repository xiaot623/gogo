package domain

// InputMessage represents the input message from the client.
type InputMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// InvokeRequest represents the request to invoke an agent.
type InvokeRequest struct {
	SessionID    string            `json:"session_id"`
	AgentID      string            `json:"agent_id"`
	InputMessage InputMessage      `json:"input_message"`
	RequestID    string            `json:"request_id,omitempty"`
	Context      map[string]string `json:"context,omitempty"`
}

// InvokeResponse represents the response from invoking an agent.
type InvokeResponse struct {
	RunID     string `json:"run_id"`
	SessionID string `json:"session_id"`
	AgentID   string `json:"agent_id"`
}

// AgentInvokeRequest is the request sent to an external agent.
type AgentInvokeRequest struct {
	AgentID      string            `json:"agent_id"`
	SessionID    string            `json:"session_id"`
	RunID        string            `json:"run_id"`
	InputMessage InputMessage      `json:"input_message"`
	Messages     []Message         `json:"messages,omitempty"`
	Context      map[string]string `json:"context,omitempty"`
}
