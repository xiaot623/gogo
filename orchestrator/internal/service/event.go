package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/xiaot623/gogo/orchestrator/internal/domain"
)

// recordEvent records an event to the store.
func (s *Service) recordEvent(ctx context.Context, runID string, eventType domain.EventType, payload interface{}) error {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	event := &domain.Event{
		EventID: "evt_" + uuid.New().String()[:8],
		RunID:   runID,
		Ts:      time.Now().UnixMilli(),
		Type:    eventType,
		Payload: payloadBytes,
	}

	return s.store.CreateEvent(ctx, event)
}
