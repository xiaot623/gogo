import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import { OpenRouterProvider } from "../../src/providers/openrouter.js";
import type { ChatCompletionResponse, ChatCompletionChunk } from "../../src/types.js";

// Helper to create a mock Response
function createMockResponse(body: unknown, options: { ok?: boolean; status?: number } = {}) {
  const { ok = true, status = 200 } = options;
  return {
    ok,
    status,
    json: () => Promise.resolve(body),
    text: () => Promise.resolve(typeof body === "string" ? body : JSON.stringify(body)),
    body: null,
  } as unknown as Response;
}

// Helper to create a mock streaming Response
function createMockStreamResponse(chunks: string[], options: { ok?: boolean; status?: number } = {}) {
  const { ok = true, status = 200 } = options;
  
  let chunkIndex = 0;
  const encoder = new TextEncoder();
  
  const mockReader = {
    read: vi.fn().mockImplementation(() => {
      if (chunkIndex < chunks.length) {
        const chunk = chunks[chunkIndex++];
        return Promise.resolve({ done: false, value: encoder.encode(chunk) });
      }
      return Promise.resolve({ done: true, value: undefined });
    }),
    releaseLock: vi.fn(),
  };

  return {
    ok,
    status,
    text: () => Promise.resolve("error"),
    body: {
      getReader: () => mockReader,
    },
  } as unknown as Response;
}

describe("OpenRouterProvider", () => {
  let originalFetch: typeof global.fetch;

  beforeEach(() => {
    originalFetch = global.fetch;
  });

  afterEach(() => {
    global.fetch = originalFetch;
    vi.restoreAllMocks();
  });

  describe("constructor", () => {
    it("should create provider with API key", () => {
      const provider = new OpenRouterProvider({ apiKey: "test-key" });
      expect(provider.name).toBe("openrouter");
    });

    it("should throw error without API key", () => {
      expect(() => {
        new OpenRouterProvider({ apiKey: "" });
      }).toThrow("OpenRouter API key is required");
    });

    it("should use custom base URL", () => {
      const provider = new OpenRouterProvider({
        apiKey: "test-key",
        baseUrl: "https://custom.api.com",
      });
      expect(provider.name).toBe("openrouter");
    });
  });

  describe("chat", () => {
    it("should make chat completion request", async () => {
      const mockResponse: ChatCompletionResponse = {
        id: "chatcmpl-123",
        object: "chat.completion",
        created: 1234567890,
        model: "openai/gpt-4",
        choices: [{
          index: 0,
          message: { role: "assistant", content: "Hello! How can I help?" },
          finish_reason: "stop",
        }],
        usage: {
          prompt_tokens: 10,
          completion_tokens: 20,
          total_tokens: 30,
        },
      };

      global.fetch = vi.fn().mockResolvedValue(createMockResponse(mockResponse));

      const provider = new OpenRouterProvider({ apiKey: "test-key" });
      const response = await provider.chat({
        model: "openai/gpt-4",
        messages: [{ role: "user", content: "Hello" }],
      });

      expect(response).toEqual(mockResponse);
      expect(global.fetch).toHaveBeenCalledWith(
        "https://openrouter.ai/api/v1/chat/completions",
        expect.objectContaining({
          method: "POST",
          headers: expect.objectContaining({
            Authorization: "Bearer test-key",
            "Content-Type": "application/json",
          }),
        })
      );
    });

    it("should send request with all parameters", async () => {
      const mockResponse: ChatCompletionResponse = {
        id: "chatcmpl-123",
        object: "chat.completion",
        created: 1234567890,
        model: "openai/gpt-4",
        choices: [{
          index: 0,
          message: { role: "assistant", content: "Response" },
          finish_reason: "stop",
        }],
      };

      global.fetch = vi.fn().mockResolvedValue(createMockResponse(mockResponse));

      const provider = new OpenRouterProvider({ apiKey: "test-key" });
      await provider.chat({
        model: "openai/gpt-4",
        messages: [
          { role: "system", content: "You are helpful" },
          { role: "user", content: "Hello" },
        ],
        temperature: 0.7,
        max_tokens: 100,
        top_p: 0.9,
      });

      const fetchCall = vi.mocked(global.fetch).mock.calls[0];
      const body = JSON.parse(fetchCall[1]?.body as string);

      expect(body.model).toBe("openai/gpt-4");
      expect(body.messages).toHaveLength(2);
      expect(body.temperature).toBe(0.7);
      expect(body.max_tokens).toBe(100);
      expect(body.top_p).toBe(0.9);
      expect(body.stream).toBe(false);
    });

    it("should throw error on API failure", async () => {
      global.fetch = vi.fn().mockResolvedValue(
        createMockResponse("Rate limit exceeded", { ok: false, status: 429 })
      );

      const provider = new OpenRouterProvider({ apiKey: "test-key" });

      await expect(provider.chat({
        model: "openai/gpt-4",
        messages: [{ role: "user", content: "Hello" }],
      })).rejects.toThrow("OpenRouter API error (429)");
    });

    it("should include custom headers", async () => {
      const mockResponse: ChatCompletionResponse = {
        id: "chatcmpl-123",
        object: "chat.completion",
        created: 1234567890,
        model: "openai/gpt-4",
        choices: [{
          index: 0,
          message: { role: "assistant", content: "Response" },
          finish_reason: "stop",
        }],
      };

      global.fetch = vi.fn().mockResolvedValue(createMockResponse(mockResponse));

      const provider = new OpenRouterProvider({
        apiKey: "test-key",
        headers: { "X-Custom": "value" },
      });
      await provider.chat({
        model: "openai/gpt-4",
        messages: [{ role: "user", content: "Hello" }],
      });

      expect(global.fetch).toHaveBeenCalledWith(
        expect.any(String),
        expect.objectContaining({
          headers: expect.objectContaining({
            "X-Custom": "value",
          }),
        })
      );
    });
  });

  describe("chatStream", () => {
    it("should stream chat completion", async () => {
      const chunks = [
        'data: {"id":"chatcmpl-1","object":"chat.completion.chunk","created":1234567890,"model":"gpt-4","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}\n\n',
        'data: {"id":"chatcmpl-1","object":"chat.completion.chunk","created":1234567890,"model":"gpt-4","choices":[{"index":0,"delta":{"content":"Hello"},"finish_reason":null}]}\n\n',
        'data: {"id":"chatcmpl-1","object":"chat.completion.chunk","created":1234567890,"model":"gpt-4","choices":[{"index":0,"delta":{"content":"!"},"finish_reason":"stop"}]}\n\n',
        'data: [DONE]\n\n',
      ];

      global.fetch = vi.fn().mockResolvedValue(createMockStreamResponse(chunks));

      const provider = new OpenRouterProvider({ apiKey: "test-key" });
      const receivedChunks: ChatCompletionChunk[] = [];

      for await (const chunk of provider.chatStream({
        model: "openai/gpt-4",
        messages: [{ role: "user", content: "Hello" }],
      })) {
        receivedChunks.push(chunk);
      }

      expect(receivedChunks).toHaveLength(3);
      expect(receivedChunks[0].choices[0].delta.role).toBe("assistant");
      expect(receivedChunks[1].choices[0].delta.content).toBe("Hello");
      expect(receivedChunks[2].choices[0].delta.content).toBe("!");
      expect(receivedChunks[2].choices[0].finish_reason).toBe("stop");
    });

    it("should set stream: true in request", async () => {
      global.fetch = vi.fn().mockResolvedValue(
        createMockStreamResponse(['data: [DONE]\n\n'])
      );

      const provider = new OpenRouterProvider({ apiKey: "test-key" });

      // Consume the stream
      for await (const _ of provider.chatStream({
        model: "openai/gpt-4",
        messages: [{ role: "user", content: "Hello" }],
      })) {
        // empty
      }

      const fetchCall = vi.mocked(global.fetch).mock.calls[0];
      const body = JSON.parse(fetchCall[1]?.body as string);
      expect(body.stream).toBe(true);
    });

    it("should throw error on API failure", async () => {
      global.fetch = vi.fn().mockResolvedValue(
        createMockStreamResponse([], { ok: false, status: 500 })
      );

      const provider = new OpenRouterProvider({ apiKey: "test-key" });

      await expect(async () => {
        for await (const _ of provider.chatStream({
          model: "openai/gpt-4",
          messages: [{ role: "user", content: "Hello" }],
        })) {
          // empty
        }
      }).rejects.toThrow("OpenRouter API error (500)");
    });

    it("should handle malformed JSON in stream", async () => {
      const chunks = [
        'data: {"id":"1","object":"chat.completion.chunk","created":1,"model":"gpt-4","choices":[{"index":0,"delta":{"content":"Hi"},"finish_reason":null}]}\n',
        'data: {malformed json}\n',
        'data: {"id":"2","object":"chat.completion.chunk","created":1,"model":"gpt-4","choices":[{"index":0,"delta":{"content":"!"},"finish_reason":"stop"}]}\n',
        'data: [DONE]\n',
      ];

      global.fetch = vi.fn().mockResolvedValue(createMockStreamResponse(chunks));

      const provider = new OpenRouterProvider({ apiKey: "test-key" });
      const receivedChunks: ChatCompletionChunk[] = [];

      for await (const chunk of provider.chatStream({
        model: "openai/gpt-4",
        messages: [{ role: "user", content: "Hello" }],
      })) {
        receivedChunks.push(chunk);
      }

      // Should skip malformed JSON and continue
      expect(receivedChunks).toHaveLength(2);
    });
  });

  describe("listModels", () => {
    it("should list available models", async () => {
      const mockModels = {
        data: [
          {
            id: "openai/gpt-4",
            name: "GPT-4",
            context_length: 8192,
            pricing: { prompt: "0.00003", completion: "0.00006" },
          },
          {
            id: "anthropic/claude-3-opus",
            name: "Claude 3 Opus",
            context_length: 200000,
            pricing: { prompt: "0.000015", completion: "0.000075" },
          },
        ],
      };

      global.fetch = vi.fn().mockResolvedValue(createMockResponse(mockModels));

      const provider = new OpenRouterProvider({ apiKey: "test-key" });
      const models = await provider.listModels();

      expect(models).toHaveLength(2);
      expect(models[0]).toEqual({
        id: "openai/gpt-4",
        name: "GPT-4",
        provider: "openai",
        contextLength: 8192,
        pricing: { prompt: 30, completion: 60 },
      });
      expect(models[1]).toEqual({
        id: "anthropic/claude-3-opus",
        name: "Claude 3 Opus",
        provider: "anthropic",
        contextLength: 200000,
        pricing: { prompt: 15, completion: 75 },
      });
    });

    it("should call correct endpoint", async () => {
      global.fetch = vi.fn().mockResolvedValue(createMockResponse({ data: [] }));

      const provider = new OpenRouterProvider({ apiKey: "test-key" });
      await provider.listModels();

      expect(global.fetch).toHaveBeenCalledWith(
        "https://openrouter.ai/api/v1/models",
        expect.objectContaining({
          method: "GET",
        })
      );
    });

    it("should throw error on API failure", async () => {
      global.fetch = vi.fn().mockResolvedValue(
        createMockResponse("Unauthorized", { ok: false, status: 401 })
      );

      const provider = new OpenRouterProvider({ apiKey: "invalid-key" });

      await expect(provider.listModels()).rejects.toThrow("OpenRouter API error (401)");
    });

    it("should extract provider from model ID", async () => {
      const mockModels = {
        data: [
          {
            id: "meta-llama/llama-3.1-70b",
            name: "Llama 3.1 70B",
            context_length: 131072,
            pricing: { prompt: "0.0000008", completion: "0.0000008" },
          },
        ],
      };

      global.fetch = vi.fn().mockResolvedValue(createMockResponse(mockModels));

      const provider = new OpenRouterProvider({ apiKey: "test-key" });
      const models = await provider.listModels();

      expect(models[0].provider).toBe("meta-llama");
    });
  });
});
