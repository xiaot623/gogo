package tools

import (
	"context"
	"encoding/json"
	"fmt"
)

func init() {
	MustRegister("weather.query", func(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
		return json.RawMessage(`{"weather":"Sunny","temperature":25}`), nil
	})
	MustRegister("payments.transfer", func(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
		return json.RawMessage(`{"status":"completed","transaction_id":"tx_123"}`), nil
	})
	MustRegister("dangerous.command", func(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
		return nil, fmt.Errorf("tool execution disabled")
	})
}
