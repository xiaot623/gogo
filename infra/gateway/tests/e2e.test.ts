/**
 * End-to-end tests for LLM Gateway
 *
 * These tests spin up a real HTTP server and make actual HTTP requests
 * to test the full request/response cycle.
 */
import { describe, it, expect, beforeAll, afterAll } from "vitest";
import { Hono } from "hono";
import { serve } from "@hono/node-server";
import { streamSSE } from "hono/streaming";
import { Gateway, registerProviderFactory } from "../src/gateway.js";
import { ChatCompletionRequestSchema } from "../src/types.js";
import type {
  Provider,
  ChatCompletionRequest,
  ChatCompletionResponse,
  ChatCompletionChunk,
  ModelInfo,
} from "../src/types.js";

// ============================================================================
// Test Configuration
// ============================================================================

const TEST_PORT = 3999;
const BASE_URL = `http://localhost:${TEST_PORT}`;

// ============================================================================
// Mock Provider for E2E Testing
// ============================================================================

class E2EMockProvider implements Provider {
  readonly name = "e2e-mock";
  private delay: number;

  constructor(delay = 10) {
    this.delay = delay;
  }

  async chat(request: ChatCompletionRequest): Promise<ChatCompletionResponse> {
    await this.simulateDelay();

    const content = request.messages
      .filter((m) => m.role === "user")
      .map((m) => m.content)
      .join(" ");

    return {
      id: `e2e-${Date.now()}`,
      object: "chat.completion",
      created: Math.floor(Date.now() / 1000),
      model: request.model,
      choices: [
        {
          index: 0,
          message: {
            role: "assistant",
            content: `E2E Response: ${content}`,
          },
          finish_reason: "stop",
        },
      ],
      usage: {
        prompt_tokens: 10,
        completion_tokens: 20,
        total_tokens: 30,
      },
    };
  }

  async *chatStream(
    request: ChatCompletionRequest
  ): AsyncIterable<ChatCompletionChunk> {
    const content = request.messages
      .filter((m) => m.role === "user")
      .map((m) => m.content)
      .join(" ");

    const words = `E2E Streamed: ${content}`.split(" ");
    const baseId = `e2e-stream-${Date.now()}`;

    for (let i = 0; i < words.length; i++) {
      await this.simulateDelay();
      yield {
        id: baseId,
        object: "chat.completion.chunk",
        created: Math.floor(Date.now() / 1000),
        model: request.model,
        choices: [
          {
            index: 0,
            delta: {
              role: i === 0 ? "assistant" : undefined,
              content: i === 0 ? words[i] : ` ${words[i]}`,
            },
            finish_reason: i === words.length - 1 ? "stop" : null,
          },
        ],
      };
    }
  }

  async listModels(): Promise<ModelInfo[]> {
    await this.simulateDelay();
    return [
      {
        id: "e2e-mock/gpt-4",
        name: "E2E Mock GPT-4",
        provider: "e2e-mock",
        contextLength: 128000,
        pricing: { prompt: 30, completion: 60 },
      },
      {
        id: "e2e-mock/gpt-3.5",
        name: "E2E Mock GPT-3.5",
        provider: "e2e-mock",
        contextLength: 16000,
        pricing: { prompt: 0.5, completion: 1.5 },
      },
    ];
  }

  private simulateDelay(): Promise<void> {
    return new Promise((resolve) => setTimeout(resolve, this.delay));
  }
}

// ============================================================================
// Server Setup
// ============================================================================

function createE2EApp(gateway: Gateway): Hono {
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
      return c.json(
        {
          error: {
            message: "Invalid request",
            details: parsed.error.errors,
          },
        },
        400
      );
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
            const message =
              error instanceof Error ? error.message : "Unknown error";
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

// ============================================================================
// E2E Tests
// ============================================================================

describe("E2E Gateway Tests", () => {
  let server: ReturnType<typeof serve>;
  let gateway: Gateway;

  beforeAll(async () => {
    // Register mock provider
    registerProviderFactory("e2e-mock", () => new E2EMockProvider(5));

    // Create gateway with mock provider
    gateway = new Gateway({
      providers: {
        "e2e-mock": { apiKey: "test-e2e-key" },
      },
      defaultProvider: "e2e-mock",
    });

    // Create and start server
    const app = createE2EApp(gateway);
    server = serve({
      fetch: app.fetch,
      port: TEST_PORT,
    });

    // Wait for server to be ready
    await new Promise((resolve) => setTimeout(resolve, 100));
  });

  afterAll(async () => {
    // Gracefully close server
    await new Promise<void>((resolve) => {
      server.close(() => resolve());
    });
  });

  // --------------------------------------------------------------------------
  // Health Check
  // --------------------------------------------------------------------------

  describe("GET /health", () => {
    it("should return health status with provider list", async () => {
      const response = await fetch(`${BASE_URL}/health`);
      const json = await response.json();

      expect(response.status).toBe(200);
      expect(json.status).toBe("ok");
      expect(json.providers).toContain("e2e-mock");
    });

    it("should respond quickly (under 100ms)", async () => {
      const start = Date.now();
      await fetch(`${BASE_URL}/health`);
      const elapsed = Date.now() - start;

      expect(elapsed).toBeLessThan(100);
    });
  });

  // --------------------------------------------------------------------------
  // Providers
  // --------------------------------------------------------------------------

  describe("GET /v1/providers", () => {
    it("should list all configured providers", async () => {
      const response = await fetch(`${BASE_URL}/v1/providers`);
      const json = await response.json();

      expect(response.status).toBe(200);
      expect(json.providers).toEqual(["e2e-mock"]);
      expect(json.default).toBe("e2e-mock");
    });
  });

  // --------------------------------------------------------------------------
  // Models
  // --------------------------------------------------------------------------

  describe("GET /v1/models", () => {
    it("should list models from default provider", async () => {
      const response = await fetch(`${BASE_URL}/v1/models`);
      const json = await response.json();

      expect(response.status).toBe(200);
      expect(json.object).toBe("list");
      expect(json.data).toHaveLength(2);
      expect(json.data[0].id).toBe("e2e-mock/gpt-4");
      expect(json.data[1].id).toBe("e2e-mock/gpt-3.5");
    });

    it("should list models from specified provider", async () => {
      const response = await fetch(`${BASE_URL}/v1/models?provider=e2e-mock`);
      const json = await response.json();

      expect(response.status).toBe(200);
      expect(json.data.every((m: ModelInfo) => m.provider === "e2e-mock")).toBe(
        true
      );
    });

    it("should return error for non-existent provider", async () => {
      const response = await fetch(`${BASE_URL}/v1/models?provider=nonexistent`);
      const json = await response.json();

      expect(response.status).toBe(500);
      expect(json.error.message).toContain("Provider not found");
    });
  });

  // --------------------------------------------------------------------------
  // Chat Completions (Non-Streaming)
  // --------------------------------------------------------------------------

  describe("POST /v1/chat/completions (non-streaming)", () => {
    it("should create a chat completion", async () => {
      const response = await fetch(`${BASE_URL}/v1/chat/completions`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          model: "e2e-mock/gpt-4",
          messages: [{ role: "user", content: "Hello, World!" }],
        }),
      });
      const json = await response.json();

      expect(response.status).toBe(200);
      expect(json.object).toBe("chat.completion");
      expect(json.model).toBe("e2e-mock/gpt-4");
      expect(json.choices).toHaveLength(1);
      expect(json.choices[0].message.role).toBe("assistant");
      expect(json.choices[0].message.content).toContain("E2E Response");
      expect(json.choices[0].message.content).toContain("Hello, World!");
      expect(json.choices[0].finish_reason).toBe("stop");
      expect(json.usage).toBeDefined();
      expect(json.usage.total_tokens).toBe(30);
    });

    it("should handle multi-turn conversation", async () => {
      const response = await fetch(`${BASE_URL}/v1/chat/completions`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          model: "e2e-mock/gpt-4",
          messages: [
            { role: "system", content: "You are a helpful assistant." },
            { role: "user", content: "First message" },
            { role: "assistant", content: "First response" },
            { role: "user", content: "Second message" },
          ],
        }),
      });
      const json = await response.json();

      expect(response.status).toBe(200);
      expect(json.choices[0].message.content).toContain("First message");
      expect(json.choices[0].message.content).toContain("Second message");
    });

    it("should accept optional parameters", async () => {
      const response = await fetch(`${BASE_URL}/v1/chat/completions`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          model: "e2e-mock/gpt-4",
          messages: [{ role: "user", content: "Test" }],
          temperature: 0.7,
          max_tokens: 100,
          top_p: 0.9,
          frequency_penalty: 0.5,
          presence_penalty: 0.5,
        }),
      });

      expect(response.status).toBe(200);
    });

    it("should use specific provider via query parameter", async () => {
      const response = await fetch(
        `${BASE_URL}/v1/chat/completions?provider=e2e-mock`,
        {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({
            model: "e2e-mock/gpt-4",
            messages: [{ role: "user", content: "Test" }],
          }),
        }
      );

      expect(response.status).toBe(200);
    });

    it("should return 400 for missing model", async () => {
      const response = await fetch(`${BASE_URL}/v1/chat/completions`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          messages: [{ role: "user", content: "Test" }],
        }),
      });
      const json = await response.json();

      expect(response.status).toBe(400);
      expect(json.error.message).toBe("Invalid request");
    });

    it("should return 400 for missing messages", async () => {
      const response = await fetch(`${BASE_URL}/v1/chat/completions`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          model: "e2e-mock/gpt-4",
        }),
      });
      const json = await response.json();

      expect(response.status).toBe(400);
      expect(json.error.message).toBe("Invalid request");
    });

    it("should return 400 for invalid JSON", async () => {
      const response = await fetch(`${BASE_URL}/v1/chat/completions`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: "not valid json",
      });
      const json = await response.json();

      expect(response.status).toBe(400);
      expect(json.error.message).toBe("Invalid JSON body");
    });

    it("should return 400 for invalid temperature (out of range)", async () => {
      const response = await fetch(`${BASE_URL}/v1/chat/completions`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          model: "e2e-mock/gpt-4",
          messages: [{ role: "user", content: "Test" }],
          temperature: 3.0, // Invalid: max is 2
        }),
      });

      expect(response.status).toBe(400);
    });

    it("should return 500 for non-existent provider", async () => {
      const response = await fetch(
        `${BASE_URL}/v1/chat/completions?provider=nonexistent`,
        {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({
            model: "some-model",
            messages: [{ role: "user", content: "Test" }],
          }),
        }
      );
      const json = await response.json();

      expect(response.status).toBe(500);
      expect(json.error.message).toContain("Provider not found");
    });
  });

  // --------------------------------------------------------------------------
  // Chat Completions (Streaming)
  // --------------------------------------------------------------------------

  describe("POST /v1/chat/completions (streaming)", () => {
    it("should stream chat completion", async () => {
      const response = await fetch(`${BASE_URL}/v1/chat/completions`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          model: "e2e-mock/gpt-4",
          messages: [{ role: "user", content: "Stream test" }],
          stream: true,
        }),
      });

      expect(response.status).toBe(200);
      expect(response.headers.get("content-type")).toContain("text/event-stream");

      const text = await response.text();
      const lines = text.split("\n").filter((line) => line.startsWith("data:"));

      expect(lines.length).toBeGreaterThan(0);

      // Parse chunks (skip [DONE])
      const chunks: ChatCompletionChunk[] = [];
      for (const line of lines) {
        const data = line.replace("data: ", "").trim();
        if (data !== "[DONE]") {
          chunks.push(JSON.parse(data));
        }
      }

      expect(chunks.length).toBeGreaterThan(0);
      expect(chunks[0].object).toBe("chat.completion.chunk");
      expect(chunks[0].choices[0].delta.role).toBe("assistant");

      // Verify final [DONE] marker
      expect(text).toContain("data: [DONE]");
    });

    it("should accumulate streamed content correctly", async () => {
      const response = await fetch(`${BASE_URL}/v1/chat/completions`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          model: "e2e-mock/gpt-4",
          messages: [{ role: "user", content: "Hello" }],
          stream: true,
        }),
      });

      const text = await response.text();
      const lines = text.split("\n").filter((line) => line.startsWith("data:"));

      // Accumulate content from all chunks
      let accumulatedContent = "";
      for (const line of lines) {
        const data = line.replace("data: ", "").trim();
        if (data !== "[DONE]") {
          const chunk = JSON.parse(data) as ChatCompletionChunk;
          accumulatedContent += chunk.choices[0].delta.content ?? "";
        }
      }

      expect(accumulatedContent).toContain("E2E Streamed");
      expect(accumulatedContent).toContain("Hello");
    });

    it("should have finish_reason on last chunk", async () => {
      const response = await fetch(`${BASE_URL}/v1/chat/completions`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          model: "e2e-mock/gpt-4",
          messages: [{ role: "user", content: "Test" }],
          stream: true,
        }),
      });

      const text = await response.text();
      const lines = text.split("\n").filter((line) => line.startsWith("data:"));

      const chunks: ChatCompletionChunk[] = [];
      for (const line of lines) {
        const data = line.replace("data: ", "").trim();
        if (data !== "[DONE]") {
          chunks.push(JSON.parse(data));
        }
      }

      // Last chunk should have finish_reason
      const lastChunk = chunks[chunks.length - 1];
      expect(lastChunk.choices[0].finish_reason).toBe("stop");

      // Intermediate chunks should have null finish_reason
      if (chunks.length > 1) {
        const firstChunk = chunks[0];
        expect(firstChunk.choices[0].finish_reason).toBeNull();
      }
    });
  });

  // --------------------------------------------------------------------------
  // Tool Calls
  // --------------------------------------------------------------------------

  describe("Tool calls in messages", () => {
    it("should accept messages with tool calls", async () => {
      const response = await fetch(`${BASE_URL}/v1/chat/completions`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          model: "e2e-mock/gpt-4",
          messages: [
            { role: "user", content: "What's the weather?" },
            {
              role: "assistant",
              content: null,
              tool_calls: [
                {
                  id: "call_abc123",
                  type: "function",
                  function: {
                    name: "get_weather",
                    arguments: '{"location":"NYC"}',
                  },
                },
              ],
            },
            {
              role: "tool",
              tool_call_id: "call_abc123",
              content: "Sunny, 72F",
            },
          ],
        }),
      });

      expect(response.status).toBe(200);
    });

    it("should accept tools parameter", async () => {
      const response = await fetch(`${BASE_URL}/v1/chat/completions`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          model: "e2e-mock/gpt-4",
          messages: [{ role: "user", content: "Test" }],
          tools: [
            {
              type: "function",
              function: {
                name: "get_weather",
                description: "Get weather for a location",
                parameters: {
                  type: "object",
                  properties: {
                    location: { type: "string" },
                  },
                  required: ["location"],
                },
              },
            },
          ],
          tool_choice: "auto",
        }),
      });

      expect(response.status).toBe(200);
    });
  });

  // --------------------------------------------------------------------------
  // Response Format
  // --------------------------------------------------------------------------

  describe("Response format", () => {
    it("should accept response_format parameter", async () => {
      const response = await fetch(`${BASE_URL}/v1/chat/completions`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          model: "e2e-mock/gpt-4",
          messages: [{ role: "user", content: "Return JSON" }],
          response_format: { type: "json_object" },
        }),
      });

      expect(response.status).toBe(200);
    });
  });

  // --------------------------------------------------------------------------
  // Concurrent Requests
  // --------------------------------------------------------------------------

  describe("Concurrent requests", () => {
    it("should handle multiple concurrent requests", async () => {
      const requests = Array.from({ length: 5 }, (_, i) =>
        fetch(`${BASE_URL}/v1/chat/completions`, {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({
            model: "e2e-mock/gpt-4",
            messages: [{ role: "user", content: `Request ${i}` }],
          }),
        })
      );

      const responses = await Promise.all(requests);

      expect(responses.every((r) => r.status === 200)).toBe(true);

      const bodies = await Promise.all(responses.map((r) => r.json()));
      bodies.forEach((body, i) => {
        expect(body.choices[0].message.content).toContain(`Request ${i}`);
      });
    });

    it("should handle concurrent streaming requests", async () => {
      const requests = Array.from({ length: 3 }, (_, i) =>
        fetch(`${BASE_URL}/v1/chat/completions`, {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({
            model: "e2e-mock/gpt-4",
            messages: [{ role: "user", content: `Stream ${i}` }],
            stream: true,
          }),
        })
      );

      const responses = await Promise.all(requests);

      expect(responses.every((r) => r.status === 200)).toBe(true);
      expect(
        responses.every((r) =>
          r.headers.get("content-type")?.includes("text/event-stream")
        )
      ).toBe(true);

      const texts = await Promise.all(responses.map((r) => r.text()));
      texts.forEach((text, i) => {
        // Content is split across chunks, so check for "Stream" and the index separately
        expect(text).toContain("Stream");
        expect(text).toContain(`${i}`);
        expect(text).toContain("[DONE]");
      });
    });
  });
});
