import { describe, it, expect, beforeEach, vi } from "vitest";
import { Gateway, registerProviderFactory } from "../src/gateway.js";
import type { Provider, ChatCompletionRequest, ChatCompletionResponse, ChatCompletionChunk, ModelInfo } from "../src/types.js";

// Mock provider for testing
class MockProvider implements Provider {
  readonly name = "mock";
  
  constructor(public config: { apiKey: string }) {}

  async chat(request: ChatCompletionRequest): Promise<ChatCompletionResponse> {
    return {
      id: "mock-id",
      object: "chat.completion",
      created: Date.now(),
      model: request.model,
      choices: [{
        index: 0,
        message: { role: "assistant", content: "Mock response" },
        finish_reason: "stop",
      }],
    };
  }

  async *chatStream(request: ChatCompletionRequest): AsyncIterable<ChatCompletionChunk> {
    yield {
      id: "mock-chunk-id",
      object: "chat.completion.chunk",
      created: Date.now(),
      model: request.model,
      choices: [{
        index: 0,
        delta: { content: "Mock " },
        finish_reason: null,
      }],
    };
    yield {
      id: "mock-chunk-id",
      object: "chat.completion.chunk",
      created: Date.now(),
      model: request.model,
      choices: [{
        index: 0,
        delta: { content: "stream" },
        finish_reason: "stop",
      }],
    };
  }

  async listModels(): Promise<ModelInfo[]> {
    return [
      { id: "mock-model-1", name: "Mock Model 1", provider: "mock", contextLength: 4096 },
      { id: "mock-model-2", name: "Mock Model 2", provider: "mock", contextLength: 8192 },
    ];
  }
}

describe("Gateway", () => {
  beforeEach(() => {
    // Register mock provider factory
    registerProviderFactory("mock", (config) => new MockProvider(config));
  });

  describe("constructor", () => {
    it("should create gateway with providers", () => {
      const gateway = new Gateway({
        providers: {
          mock: { apiKey: "test-key" },
        },
      });

      expect(gateway.listProviders()).toEqual(["mock"]);
    });

    it("should set default provider from config", () => {
      const gateway = new Gateway({
        providers: {
          mock: { apiKey: "test-key" },
        },
        defaultProvider: "mock",
      });

      expect(gateway.getDefaultProvider()).toBe("mock");
    });

    it("should auto-set default provider if not specified", () => {
      const gateway = new Gateway({
        providers: {
          mock: { apiKey: "test-key" },
        },
      });

      expect(gateway.getDefaultProvider()).toBe("mock");
    });

    it("should throw error for unknown provider", () => {
      expect(() => {
        new Gateway({
          providers: {
            unknown: { apiKey: "test-key" },
          },
        });
      }).toThrow("Unknown provider: unknown");
    });

    it("should handle empty providers", () => {
      const gateway = new Gateway({
        providers: {},
      });

      expect(gateway.listProviders()).toEqual([]);
      expect(gateway.getDefaultProvider()).toBeUndefined();
    });
  });

  describe("getProvider", () => {
    it("should get provider by name", () => {
      const gateway = new Gateway({
        providers: {
          mock: { apiKey: "test-key" },
        },
      });

      const provider = gateway.getProvider("mock");

      expect(provider).toBeInstanceOf(MockProvider);
      expect(provider.name).toBe("mock");
    });

    it("should get default provider when no name specified", () => {
      const gateway = new Gateway({
        providers: {
          mock: { apiKey: "test-key" },
        },
        defaultProvider: "mock",
      });

      const provider = gateway.getProvider();

      expect(provider).toBeInstanceOf(MockProvider);
    });

    it("should throw error when provider not found", () => {
      const gateway = new Gateway({
        providers: {
          mock: { apiKey: "test-key" },
        },
      });

      expect(() => {
        gateway.getProvider("nonexistent");
      }).toThrow("Provider not found: nonexistent");
    });

    it("should throw error when no default provider and none specified", () => {
      const gateway = new Gateway({
        providers: {},
      });

      expect(() => {
        gateway.getProvider();
      }).toThrow("No provider specified and no default provider configured");
    });
  });

  describe("listProviders", () => {
    it("should list all registered providers", () => {
      // Register another mock provider
      registerProviderFactory("mock2", (config) => new MockProvider(config));

      const gateway = new Gateway({
        providers: {
          mock: { apiKey: "key1" },
          mock2: { apiKey: "key2" },
        },
      });

      const providers = gateway.listProviders();

      expect(providers).toContain("mock");
      expect(providers).toContain("mock2");
      expect(providers.length).toBe(2);
    });
  });

  describe("provider operations", () => {
    it("should call chat on provider", async () => {
      const gateway = new Gateway({
        providers: {
          mock: { apiKey: "test-key" },
        },
      });

      const provider = gateway.getProvider("mock");
      const response = await provider.chat({
        model: "test-model",
        messages: [{ role: "user", content: "Hello" }],
      });

      expect(response.choices[0].message.content).toBe("Mock response");
    });

    it("should stream chat from provider", async () => {
      const gateway = new Gateway({
        providers: {
          mock: { apiKey: "test-key" },
        },
      });

      const provider = gateway.getProvider("mock");
      const chunks: ChatCompletionChunk[] = [];

      for await (const chunk of provider.chatStream({
        model: "test-model",
        messages: [{ role: "user", content: "Hello" }],
      })) {
        chunks.push(chunk);
      }

      expect(chunks.length).toBe(2);
      expect(chunks[0].choices[0].delta.content).toBe("Mock ");
      expect(chunks[1].choices[0].delta.content).toBe("stream");
    });

    it("should list models from provider", async () => {
      const gateway = new Gateway({
        providers: {
          mock: { apiKey: "test-key" },
        },
      });

      const provider = gateway.getProvider("mock");
      const models = await provider.listModels();

      expect(models.length).toBe(2);
      expect(models[0].id).toBe("mock-model-1");
      expect(models[1].id).toBe("mock-model-2");
    });
  });
});

describe("registerProviderFactory", () => {
  it("should register a custom provider factory", () => {
    class CustomProvider implements Provider {
      readonly name = "custom";
      async chat(): Promise<ChatCompletionResponse> {
        return {} as ChatCompletionResponse;
      }
      async *chatStream(): AsyncIterable<ChatCompletionChunk> {}
      async listModels(): Promise<ModelInfo[]> {
        return [];
      }
    }

    registerProviderFactory("custom", () => new CustomProvider());

    const gateway = new Gateway({
      providers: {
        custom: { apiKey: "key" },
      },
    });

    const provider = gateway.getProvider("custom");
    expect(provider.name).toBe("custom");
  });
});
