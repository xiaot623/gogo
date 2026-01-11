import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import { Hono } from "hono";
import { streamSSE } from "hono/streaming";
import { Gateway, registerProviderFactory } from "../src/gateway.js";
import { ChatCompletionRequestSchema } from "../src/types.js";
import type { Provider, ChatCompletionRequest, ChatCompletionResponse, ChatCompletionChunk, ModelInfo } from "../src/types.js";

// Create a test app factory (mirrors server.ts but allows injection)
function createTestApp(gateway: Gateway) {
  const app = new Hono();

  app.get("/health", (c) => {
    return c.json({ status: "ok", providers: gateway.listProviders() });
  });

  app.get("/v1/providers", (c) => {
    return c.json({
      providers: gateway.listProviders(),
      default: gateway.getDefaultProvider(),
    });
  });

  app.get("/v1/models", async (c) => {
    const providerName = c.req.query("provider");
    try {
      const provider = gateway.getProvider(providerName);
      const models = await provider.listModels();
      return c.json({ object: "list", data: models });
    } catch (error) {
      const message = error instanceof Error ? error.message : "Unknown error";
      return c.json({ error: { message } }, 500);
    }
  });

  app.post("/v1/chat/completions", async (c) => {
    const providerName = c.req.query("provider");

    let body: unknown;
    try {
      body = await c.req.json();
    } catch {
      return c.json({ error: { message: "Invalid JSON body" } }, 400);
    }

    const parsed = ChatCompletionRequestSchema.safeParse(body);
    if (!parsed.success) {
      return c.json({
        error: {
          message: "Invalid request",
          details: parsed.error.errors,
        },
      }, 400);
    }

    const request = parsed.data;

    try {
      const provider = gateway.getProvider(providerName);

      if (request.stream) {
        return streamSSE(c, async (stream) => {
          try {
            for await (const chunk of provider.chatStream(request)) {
              await stream.writeSSE({
                data: JSON.stringify(chunk),
              });
            }
            await stream.writeSSE({ data: "[DONE]" });
          } catch (error) {
            const message = error instanceof Error ? error.message : "Unknown error";
            await stream.writeSSE({
              data: JSON.stringify({ error: { message } }),
            });
          }
        });
      }

      const response = await provider.chat(request);
      return c.json(response);
    } catch (error) {
      const message = error instanceof Error ? error.message : "Unknown error";
      return c.json({ error: { message } }, 500);
    }
  });

  return app;
}

// Mock provider for testing
class MockProvider implements Provider {
  readonly name = "mock";
  
  async chat(request: ChatCompletionRequest): Promise<ChatCompletionResponse> {
    return {
      id: "mock-id",
      object: "chat.completion",
      created: Date.now(),
      model: request.model,
      choices: [{
        index: 0,
        message: { role: "assistant", content: `Response to: ${request.messages[0]?.content}` },
        finish_reason: "stop",
      }],
    };
  }

  async *chatStream(request: ChatCompletionRequest): AsyncIterable<ChatCompletionChunk> {
    yield {
      id: "mock-chunk",
      object: "chat.completion.chunk",
      created: Date.now(),
      model: request.model,
      choices: [{
        index: 0,
        delta: { role: "assistant", content: "Streamed response" },
        finish_reason: "stop",
      }],
    };
  }

  async listModels(): Promise<ModelInfo[]> {
    return [
      { id: "mock/model-1", name: "Mock Model 1", provider: "mock", contextLength: 4096 },
    ];
  }
}

describe("Server API", () => {
  let app: Hono;
  let gateway: Gateway;

  beforeEach(() => {
    // Register mock provider
    registerProviderFactory("mock", () => new MockProvider());
    
    gateway = new Gateway({
      providers: {
        mock: { apiKey: "test-key" },
      },
      defaultProvider: "mock",
    });
    
    app = createTestApp(gateway);
  });

  describe("GET /health", () => {
    it("should return health status", async () => {
      const res = await app.request("/health");
      const json = await res.json();

      expect(res.status).toBe(200);
      expect(json.status).toBe("ok");
      expect(json.providers).toContain("mock");
    });
  });

  describe("GET /v1/providers", () => {
    it("should list providers", async () => {
      const res = await app.request("/v1/providers");
      const json = await res.json();

      expect(res.status).toBe(200);
      expect(json.providers).toContain("mock");
      expect(json.default).toBe("mock");
    });
  });

  describe("GET /v1/models", () => {
    it("should list models", async () => {
      const res = await app.request("/v1/models");
      const json = await res.json();

      expect(res.status).toBe(200);
      expect(json.object).toBe("list");
      expect(json.data).toHaveLength(1);
      expect(json.data[0].id).toBe("mock/model-1");
    });

    it("should list models for specific provider", async () => {
      const res = await app.request("/v1/models?provider=mock");
      const json = await res.json();

      expect(res.status).toBe(200);
      expect(json.data[0].provider).toBe("mock");
    });

    it("should return error for unknown provider", async () => {
      const res = await app.request("/v1/models?provider=unknown");
      const json = await res.json();

      expect(res.status).toBe(500);
      expect(json.error.message).toContain("Provider not found");
    });
  });

  describe("POST /v1/chat/completions", () => {
    it("should create chat completion", async () => {
      const res = await app.request("/v1/chat/completions", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          model: "mock/model-1",
          messages: [{ role: "user", content: "Hello" }],
        }),
      });
      const json = await res.json();

      expect(res.status).toBe(200);
      expect(json.id).toBe("mock-id");
      expect(json.choices[0].message.content).toContain("Response to: Hello");
    });

    it("should validate request body", async () => {
      const res = await app.request("/v1/chat/completions", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          // Missing model and messages
        }),
      });
      const json = await res.json();

      expect(res.status).toBe(400);
      expect(json.error.message).toBe("Invalid request");
    });

    it("should return error for invalid JSON", async () => {
      const res = await app.request("/v1/chat/completions", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: "not json",
      });
      const json = await res.json();

      expect(res.status).toBe(400);
      expect(json.error.message).toBe("Invalid JSON body");
    });

    it("should handle temperature parameter", async () => {
      const res = await app.request("/v1/chat/completions", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          model: "mock/model-1",
          messages: [{ role: "user", content: "Hello" }],
          temperature: 0.7,
        }),
      });

      expect(res.status).toBe(200);
    });

    it("should reject invalid temperature", async () => {
      const res = await app.request("/v1/chat/completions", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          model: "mock/model-1",
          messages: [{ role: "user", content: "Hello" }],
          temperature: 3.0, // Invalid: max is 2
        }),
      });

      expect(res.status).toBe(400);
    });

    it("should use specific provider via query param", async () => {
      const res = await app.request("/v1/chat/completions?provider=mock", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          model: "mock/model-1",
          messages: [{ role: "user", content: "Hello" }],
        }),
      });

      expect(res.status).toBe(200);
    });

    it("should return error for unknown provider", async () => {
      const res = await app.request("/v1/chat/completions?provider=unknown", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          model: "some-model",
          messages: [{ role: "user", content: "Hello" }],
        }),
      });
      const json = await res.json();

      expect(res.status).toBe(500);
      expect(json.error.message).toContain("Provider not found");
    });

    it("should handle streaming request", async () => {
      const res = await app.request("/v1/chat/completions", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          model: "mock/model-1",
          messages: [{ role: "user", content: "Hello" }],
          stream: true,
        }),
      });

      expect(res.status).toBe(200);
      expect(res.headers.get("content-type")).toContain("text/event-stream");

      const text = await res.text();
      expect(text).toContain("data:");
      expect(text).toContain("Streamed response");
      expect(text).toContain("[DONE]");
    });

    it("should handle tool calls in messages", async () => {
      const res = await app.request("/v1/chat/completions", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          model: "mock/model-1",
          messages: [
            { role: "user", content: "What's the weather?" },
            {
              role: "assistant",
              content: null,
              tool_calls: [{
                id: "call_123",
                type: "function",
                function: { name: "get_weather", arguments: '{"city":"NYC"}' },
              }],
            },
            {
              role: "tool",
              tool_call_id: "call_123",
              content: "Sunny, 72°F",
            },
          ],
        }),
      });

      expect(res.status).toBe(200);
    });

    it("should handle tools parameter", async () => {
      const res = await app.request("/v1/chat/completions", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          model: "mock/model-1",
          messages: [{ role: "user", content: "Hello" }],
          tools: [{
            type: "function",
            function: {
              name: "get_weather",
              description: "Get weather for a city",
              parameters: {
                type: "object",
                properties: {
                  city: { type: "string" },
                },
              },
            },
          }],
          tool_choice: "auto",
        }),
      });

      expect(res.status).toBe(200);
    });
  });
});
