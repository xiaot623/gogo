package ingress

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	baseURL string
	client  *http.Client
}

func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		client:  &http.Client{Timeout: 5 * time.Second},
	}
}

func (c *Client) PushEvent(sessionID string, event map[string]interface{}) error {
	if c.baseURL == "" {
		return nil
	}

	payload := map[string]interface{}{
		"session_id": sessionID,
		"event":      event,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal event for ingress: %w", err)
	}

	url := strings.TrimSuffix(c.baseURL, "/") + "/internal/send"
	resp, err := c.client.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to push event to ingress: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("WARN: ingress returned status %d", resp.StatusCode)
		return fmt.Errorf("ingress returned status %d", resp.StatusCode)
	}

	return nil
}
