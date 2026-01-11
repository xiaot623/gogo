import { z } from "zod";

// ============================================================================
// Message Types
// ============================================================================

export const MessageRoleSchema = z.enum(["system", "user", "assistant", "tool"]);
export type MessageRole = z.infer<typeof MessageRoleSchema>;

export const TextContentSchema = z.object({
  type: z.literal("text"),
  text: z.string(),
});

export const ImageContentSchema = z.object({
  type: z.literal("image_url"),
  image_url: z.object({
    url: z.string(),
    detail: z.enum(["auto", "low", "high"]).optional(),
  }),
});

export const ContentPartSchema = z.union([TextContentSchema, ImageContentSchema]);
export type ContentPart = z.infer<typeof ContentPartSchema>;

export const MessageContentSchema = z.union([z.string(), z.array(ContentPartSchema)]);
export type MessageContent = z.infer<typeof MessageContentSchema>;

export const ToolCallSchema = z.object({
  id: z.string(),
  type: z.literal("function"),
  function: z.object({
    name: z.string(),
    arguments: z.string(),
  }),
});
export type ToolCall = z.infer<typeof ToolCallSchema>;

export const MessageSchema = z.object({
  role: MessageRoleSchema,
  content: MessageContentSchema.nullable().optional(),
  name: z.string().optional(),
  tool_calls: z.array(ToolCallSchema).optional(),
  tool_call_id: z.string().optional(),
});
export type Message = z.infer<typeof MessageSchema>;

// ============================================================================
// Tool/Function Types
// ============================================================================

export const FunctionDefinitionSchema = z.object({
  name: z.string(),
  description: z.string().optional(),
  parameters: z.record(z.any()).optional(),
});
export type FunctionDefinition = z.infer<typeof FunctionDefinitionSchema>;

export const ToolSchema = z.object({
  type: z.literal("function"),
  function: FunctionDefinitionSchema,
});
export type Tool = z.infer<typeof ToolSchema>;

// ============================================================================
// Request Types
// ============================================================================

export const ChatCompletionRequestSchema = z.object({
  model: z.string(),
  messages: z.array(MessageSchema),
  temperature: z.number().min(0).max(2).optional(),
  top_p: z.number().min(0).max(1).optional(),
  max_tokens: z.number().positive().optional(),
  stream: z.boolean().optional(),
  stop: z.union([z.string(), z.array(z.string())]).optional(),
  presence_penalty: z.number().min(-2).max(2).optional(),
  frequency_penalty: z.number().min(-2).max(2).optional(),
  tools: z.array(ToolSchema).optional(),
  tool_choice: z.union([
    z.literal("none"),
    z.literal("auto"),
    z.literal("required"),
    z.object({
      type: z.literal("function"),
      function: z.object({ name: z.string() }),
    }),
  ]).optional(),
  response_format: z.object({
    type: z.enum(["text", "json_object"]),
  }).optional(),
  seed: z.number().int().optional(),
  user: z.string().optional(),
});
export type ChatCompletionRequest = z.infer<typeof ChatCompletionRequestSchema>;

// ============================================================================
// Response Types
// ============================================================================

export interface ChatCompletionChoice {
  index: number;
  message: {
    role: "assistant";
    content: string | null;
    tool_calls?: ToolCall[];
  };
  finish_reason: "stop" | "length" | "tool_calls" | "content_filter" | null;
}

export interface ChatCompletionUsage {
  prompt_tokens: number;
  completion_tokens: number;
  total_tokens: number;
}

export interface ChatCompletionResponse {
  id: string;
  object: "chat.completion";
  created: number;
  model: string;
  choices: ChatCompletionChoice[];
  usage?: ChatCompletionUsage;
}

// ============================================================================
// Streaming Types
// ============================================================================

export interface ChatCompletionChunkDelta {
  role?: "assistant";
  content?: string | null;
  tool_calls?: Array<{
    index: number;
    id?: string;
    type?: "function";
    function?: {
      name?: string;
      arguments?: string;
    };
  }>;
}

export interface ChatCompletionChunkChoice {
  index: number;
  delta: ChatCompletionChunkDelta;
  finish_reason: "stop" | "length" | "tool_calls" | "content_filter" | null;
}

export interface ChatCompletionChunk {
  id: string;
  object: "chat.completion.chunk";
  created: number;
  model: string;
  choices: ChatCompletionChunkChoice[];
  usage?: ChatCompletionUsage;
}

// ============================================================================
// Provider Types
// ============================================================================

export interface ProviderConfig {
  apiKey: string;
  baseUrl?: string;
  defaultModel?: string;
  headers?: Record<string, string>;
}

export interface ModelInfo {
  id: string;
  name: string;
  provider: string;
  contextLength: number;
  pricing?: {
    prompt: number;  // per 1M tokens
    completion: number;  // per 1M tokens
  };
}

export interface Provider {
  readonly name: string;
  
  chat(request: ChatCompletionRequest): Promise<ChatCompletionResponse>;
  
  chatStream(request: ChatCompletionRequest): AsyncIterable<ChatCompletionChunk>;
  
  listModels(): Promise<ModelInfo[]>;
}

// ============================================================================
// Gateway Types
// ============================================================================

export interface GatewayConfig {
  providers: Record<string, ProviderConfig>;
  defaultProvider?: string;
}
