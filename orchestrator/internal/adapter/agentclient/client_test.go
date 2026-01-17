package agentclient

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/xiaot623/gogo/orchestrator/internal/domain"
)

func TestClientInvokeParsesSSE(t *testing.T) {
	var gotHeaders http.Header
	var gotReq domain.AgentInvokeRequest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/invoke" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		gotHeaders = r.Header.Clone()
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("failed to read body: %v", err)
		}
		if err := json.Unmarshal(body, &gotReq); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}

		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "event: delta\ndata: {\"text\":\"hi\"}\n\n")
		fmt.Fprint(w, "event: done\ndata: {\"final_message\":\"bye\"}\n\n")
	}))
	defer server.Close()

	client := &Client{httpClient: server.Client()}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	req := &domain.AgentInvokeRequest{
		AgentID:      "agent-1",
		SessionID:    "sess-1",
		RunID:        "run-1",
		InputMessage: domain.InputMessage{Role: "user", Content: "hello"},
	}

	var events []SSEEvent
	err := client.Invoke(ctx, server.URL, req, func(event SSEEvent) error {
		events = append(events, event)
		return nil
	})
	if err != nil {
		t.Fatalf("invoke failed: %v", err)
	}

	if gotReq.SessionID != req.SessionID || gotReq.RunID != req.RunID {
		t.Fatalf("unexpected request payload: %+v", gotReq)
	}
	if gotHeaders.Get("X-Session-ID") != req.SessionID {
		t.Fatalf("missing X-Session-ID header")
	}
	if gotHeaders.Get("X-Run-ID") != req.RunID {
		t.Fatalf("missing X-Run-ID header")
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[0].Event != "delta" || events[1].Event != "done" {
		t.Fatalf("unexpected events: %+v", events)
	}
}

func TestParseSSEMultilineData(t *testing.T) {
	input := "event: delta\n" +
		"data: first line\n" +
		"data: second line\n\n"

	var events []SSEEvent
	client := &Client{}
	if err := client.parseSSE(strings.NewReader(input), func(event SSEEvent) error {
		events = append(events, event)
		return nil
	}); err != nil {
		t.Fatalf("parseSSE failed: %v", err)
	}

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Data != "first line\nsecond line" {
		t.Fatalf("unexpected data: %q", events[0].Data)
	}
}

func TestParseEvents(t *testing.T) {
	delta, err := ParseDeltaEvent(`{"text":"hi"}`)
	if err != nil {
		t.Fatalf("ParseDeltaEvent failed: %v", err)
	}
	if delta.Text != "hi" {
		t.Fatalf("unexpected delta: %+v", delta)
	}

	done, err := ParseDoneEvent(`{"final_message":"bye"}`)
	if err != nil {
		t.Fatalf("ParseDoneEvent failed: %v", err)
	}
	if done.FinalMessage != "bye" {
		t.Fatalf("unexpected done: %+v", done)
	}

	errEvt, err := ParseErrorEvent(`{"code":"boom","message":"bad"}`)
	if err != nil {
		t.Fatalf("ParseErrorEvent failed: %v", err)
	}
	if errEvt.Code != "boom" {
		t.Fatalf("unexpected error event: %+v", errEvt)
	}
}

func TestParseEventErrors(t *testing.T) {
	if _, err := ParseDeltaEvent("nope"); err == nil {
		t.Fatalf("expected error for invalid delta")
	}
	if _, err := ParseDoneEvent("nope"); err == nil {
		t.Fatalf("expected error for invalid done")
	}
	if _, err := ParseErrorEvent("nope"); err == nil {
		t.Fatalf("expected error for invalid error")
	}
}
