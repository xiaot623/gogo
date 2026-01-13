package llmproxy

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestClientCreateChatCompletion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"c1","object":"chat.completion","created":1,"model":"gpt","choices":[{"index":0,"message":{"role":"assistant","content":"hi"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":2,"total_tokens":3}}`)
	}))
	defer server.Close()

	client := NewClient(server.URL, "", time.Second)
	resp, err := client.CreateChatCompletion(context.Background(), &ChatCompletionRequest{
		Model: "gpt",
		Messages: []ChatMessage{
			{Role: "user", Content: "hello"},
		},
	})
	if err != nil {
		t.Fatalf("CreateChatCompletion failed: %v", err)
	}
	if resp.Model != "gpt" || len(resp.Choices) != 1 {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

func TestClientCreateChatCompletionError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `{"error":{"message":"bad","type":"invalid_request_error"}}`)
	}))
	defer server.Close()

	client := NewClient(server.URL, "", time.Second)
	_, err := client.CreateChatCompletion(context.Background(), &ChatCompletionRequest{
		Model: "gpt",
		Messages: []ChatMessage{{Role: "user", Content: "hello"}},
	})
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestClientCreateChatCompletionStream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"id\":\"c1\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"gpt\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"hi\"}}]}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	client := NewClient(server.URL, "", time.Second)
	var chunks []StreamChunk
	usage, err := client.CreateChatCompletionStream(context.Background(), &ChatCompletionRequest{
		Model: "gpt",
		Messages: []ChatMessage{{Role: "user", Content: "hello"}},
	}, func(chunk *StreamChunk) error {
		chunks = append(chunks, *chunk)
		return nil
	})
	if err != nil {
		t.Fatalf("CreateChatCompletionStream failed: %v", err)
	}
	if usage != nil {
		t.Fatalf("expected nil usage, got %+v", usage)
	}
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
}

func TestClientListModels(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"object":"list","data":[{"id":"gpt","object":"model","created":1,"owned_by":"openai"}]}`)
	}))
	defer server.Close()

	client := NewClient(server.URL, "", time.Second)
	models, err := client.ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels failed: %v", err)
	}
	if len(models) != 1 || models[0].ID != "gpt" {
		t.Fatalf("unexpected models: %+v", models)
	}
}

func TestClientListModelsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		fmt.Fprint(w, "bad")
	}))
	defer server.Close()

	client := NewClient(server.URL, "", time.Second)
	_, err := client.ListModels(context.Background())
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestClientSetHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer secret" {
			t.Fatalf("unexpected Authorization header: %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"object":"list","data":[]}`)
	}))
	defer server.Close()

	client := NewClient(server.URL, "secret", time.Second)
	_, err := client.ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels failed: %v", err)
	}
}

func TestDecodeErrorResponse(t *testing.T) {
	data := []byte(`{"error":{"message":"bad","type":"invalid_request_error","code":"401"}}`)
	var resp ErrorResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if resp.Error == nil || resp.Error.Code != "401" {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}
}
