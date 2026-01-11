package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/xiaot623/gogo/orchestrator/domain"
	"github.com/google/uuid"
)

// recordEvent records an event to the store.
func (h *Handler) recordEvent(ctx context.Context, runID string, eventType domain.EventType, payload interface{}) error {
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

	return h.store.CreateEvent(ctx, event)
}

// pushEventToIngress sends an event to the ingress service.
func (h *Handler) pushEventToIngress(sessionID string, event map[string]interface{}) {
	if h.config.IngressURL == "" {
		return
	}

	payload := map[string]interface{}{
		"session_id": sessionID,
		"event":      event,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		log.Printf("ERROR: failed to marshal event for ingress: %v", err)
		return
	}

	url := strings.TrimSuffix(h.config.IngressURL, "/") + "/internal/send"
	resp, err := http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		log.Printf("WARN: failed to push event to ingress: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("WARN: ingress returned status %d", resp.StatusCode)
	}
}
