import type {
  Provider,
  ProviderConfig,
  ChatCompletionRequest,
  ChatCompletionResponse,
  ChatCompletionChunk,
  ModelInfo,
} from "../types.js";

const OPENROUTER_API_URL = "https://openrouter.ai/api/v1";

interface OpenRouterModel {
  id: string;
  name: string;
  context_length: number;
  pricing: {
    prompt: string;
    completion: string;
  };
}

interface OpenRouterModelsResponse {
  data: OpenRouterModel[];
}

export class OpenRouterProvider implements Provider {
  readonly name = "openrouter";
  private apiKey: string;
  private baseUrl: string;
  private defaultHeaders: Record<string, string>;

  constructor(config: ProviderConfig) {
    if (!config.apiKey) {
      throw new Error("OpenRouter API key is required");
    }
    this.apiKey = config.apiKey;
    this.baseUrl = config.baseUrl ?? OPENROUTER_API_URL;
    this.defaultHeaders = {
      "Authorization": `Bearer ${this.apiKey}`,
      "Content-Type": "application/json",
      "HTTP-Referer": "https://github.com/gogo/llm-gateway",
      "X-Title": "LLM Gateway",
      ...config.headers,
    };
  }

  async chat(request: ChatCompletionRequest): Promise<ChatCompletionResponse> {
    const response = await fetch(`${this.baseUrl}/chat/completions`, {
      method: "POST",
      headers: this.defaultHeaders,
      body: JSON.stringify({
        ...request,
        stream: false,
      }),
    });

    if (!response.ok) {
      const error = await response.text();
      throw new Error(`OpenRouter API error (${response.status}): ${error}`);
    }

    return response.json() as Promise<ChatCompletionResponse>;
  }

  async *chatStream(request: ChatCompletionRequest): AsyncIterable<ChatCompletionChunk> {
    const response = await fetch(`${this.baseUrl}/chat/completions`, {
      method: "POST",
      headers: this.defaultHeaders,
      body: JSON.stringify({
        ...request,
        stream: true,
      }),
    });

    if (!response.ok) {
      const error = await response.text();
      throw new Error(`OpenRouter API error (${response.status}): ${error}`);
    }

    if (!response.body) {
      throw new Error("Response body is null");
    }

    const reader = response.body.getReader();
    const decoder = new TextDecoder();
    let buffer = "";

    try {
      while (true) {
        const { done, value } = await reader.read();
        if (done) break;

        buffer += decoder.decode(value, { stream: true });
        const lines = buffer.split("\n");
        buffer = lines.pop() ?? "";

        for (const line of lines) {
          const trimmed = line.trim();
          if (!trimmed || !trimmed.startsWith("data: ")) continue;

          const data = trimmed.slice(6);
          if (data === "[DONE]") return;

          try {
            const chunk = JSON.parse(data) as ChatCompletionChunk;
            yield chunk;
          } catch {
            // Skip malformed JSON
          }
        }
      }

      // Process remaining buffer
      if (buffer.trim()) {
        const trimmed = buffer.trim();
        if (trimmed.startsWith("data: ") && trimmed.slice(6) !== "[DONE]") {
          try {
            const chunk = JSON.parse(trimmed.slice(6)) as ChatCompletionChunk;
            yield chunk;
          } catch {
            // Skip malformed JSON
          }
        }
      }
    } finally {
      reader.releaseLock();
    }
  }

  async listModels(): Promise<ModelInfo[]> {
    const response = await fetch(`${this.baseUrl}/models`, {
      method: "GET",
      headers: this.defaultHeaders,
    });

    if (!response.ok) {
      const error = await response.text();
      throw new Error(`OpenRouter API error (${response.status}): ${error}`);
    }

    const data = (await response.json()) as OpenRouterModelsResponse;

    return data.data.map((model) => ({
      id: model.id,
      name: model.name,
      provider: this.extractProvider(model.id),
      contextLength: model.context_length,
      pricing: {
        prompt: parseFloat(model.pricing.prompt) * 1_000_000,
        completion: parseFloat(model.pricing.completion) * 1_000_000,
      },
    }));
  }

  private extractProvider(modelId: string): string {
    // Model IDs are formatted as "provider/model-name"
    const parts = modelId.split("/");
    return parts[0] ?? "unknown";
  }
}
