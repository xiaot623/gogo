package domain

import (
	"encoding/json"
	"time"
)

// Agent represents a registered agent.
type Agent struct {
	AgentID       string          `json:"agent_id"`
	Name          string          `json:"name"`
	Endpoint      string          `json:"endpoint"`
	Capabilities  json.RawMessage `json:"capabilities,omitempty"`
	Status        string          `json:"status"`
	LastHeartbeat *time.Time      `json:"last_heartbeat,omitempty"`
	CreatedAt     time.Time       `json:"created_at"`
}
