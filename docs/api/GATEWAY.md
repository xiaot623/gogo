# LLM Gateway API

A small HTTP service that exposes an OpenAI-compatible API and forwards requests to a configured LLM provider (currently: `openrouter`).

Base URL is whatever you run the gateway on (default `http://0.0.0.0:3000`).

## Overview

- **Provider selection**: optional `provider` query param on relevant endpoints. If omitted, the gateway uses the configured default provider.
- **Models**: `GET /v1/models` returns a simplified model list aggregated from the selected provider.
- **Chat completions**: `POST /v1/chat/completions` is **OpenAI Chat Completions compatible** for request/response shapes (including streaming via SSE).

## Content Types

- Requests: `Content-Type: application/json`
- JSON responses: `application/json`
- Streaming responses: `text/event-stream` (SSE)

## Errors

The gateway returns errors in this shape:

```json
{ "error": { "message": "..." } }
```

Validation errors (400) from `POST /v1/chat/completions` include `details`:

```json
{ "error": { "message": "Invalid request", "details": [/* zod issues */] } }
```

For streaming requests, errors are emitted as an SSE `data:` payload:

```json
{ "error": { "message": "..." } }
```

## Endpoints

### GET /health

Health check.

**Response (200)**

```json
{
  "status": "ok",
  "providers": ["openrouter"]
}
```

### GET /v1/providers

Lists configured providers and the default provider.

**Response (200)**

```json
{
  "providers": ["openrouter"],
  "default": "openrouter"
}
```

### GET /v1/models

Lists models for a provider.

**Query params**

- `provider` (optional, string): provider name. If omitted, uses the default provider.

**Response (200)**

```json
{
  "object": "list",
  "data": [
    {
      "id": "openai/gpt-4o",
      "name": "GPT-4o",
      "provider": "openai",
      "contextLength": 128000,
      "pricing": { "prompt": 5000, "completion": 15000 }
    }
  ]
}
```

Notes:

- `pricing.prompt` / `pricing.completion` are **per 1M tokens** (numbers).

**Errors (500)**

- Unknown provider name.
- Provider upstream errors.

```json
{ "error": { "message": "Provider not found: ..." } }
```

### POST /v1/chat/completions

OpenAI-compatible chat completions endpoint.

**Query params**

- `provider` (optional, string): provider name. If omitted, uses the default provider.

**Request body**

At minimum:

```json
{
  "model": "openai/gpt-4o",
  "messages": [{ "role": "user", "content": "Hello" }]
}
```

Supported request fields (validated):

- `model` (string, required)
- `messages` (array, required)
  - `role`: `system` | `user` | `assistant` | `tool`
  - `content`: string | array of content parts | null
  - `name` (optional)
  - `tool_calls` (optional): OpenAI tool call objects
  - `tool_call_id` (optional): required for `tool` role messages
- `temperature` (number, optional, 0..2)
- `top_p` (number, optional, 0..1)
- `max_tokens` (positive number, optional)
- `stream` (boolean, optional)
- `stop` (string or string[], optional)
- `presence_penalty` (number, optional, -2..2)
- `frequency_penalty` (number, optional, -2..2)
- `tools` (array, optional): function tool definitions
- `tool_choice` (optional): `none` | `auto` | `required` | `{ type: "function", function: { name: string } }`
- `response_format` (optional): `{ type: "text" | "json_object" }`
- `seed` (int, optional)
- `user` (string, optional)

Content parts in `messages[].content` (when using array form):

- `{ "type": "text", "text": "..." }`
- `{ "type": "image_url", "image_url": { "url": "...", "detail": "auto"|"low"|"high" } }`

#### Non-streaming response

**Response (200)** is an OpenAI-style chat completion object:

```json
{
  "id": "...",
  "object": "chat.completion",
  "created": 1234567890,
  "model": "openai/gpt-4o",
  "choices": [
    {
      "index": 0,
      "message": { "role": "assistant", "content": "..." },
      "finish_reason": "stop"
    }
  ],
  "usage": {
    "prompt_tokens": 10,
    "completion_tokens": 20,
    "total_tokens": 30
  }
}
```

#### Streaming response (SSE)

If `stream: true`, the gateway responds with `text/event-stream`.

- Each SSE message contains a single line: `data: <json>` where `<json>` is an OpenAI `chat.completion.chunk` object.
- The stream ends with a final SSE message `data: [DONE]`.
- On streaming errors, the gateway writes `data: {"error":{"message":"..."}}` and then ends the stream.

Example (illustrative):

```
data: {"id":"...","object":"chat.completion.chunk","created":123,"model":"...","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}

data: {"id":"...","object":"chat.completion.chunk","created":123,"model":"...","choices":[{"index":0,"delta":{"content":"Hello"},"finish_reason":null}]}

data: [DONE]

```

**Errors (400)**

- Invalid JSON body:

```json
{ "error": { "message": "Invalid JSON body" } }
```

- Validation failed:

```json
{ "error": { "message": "Invalid request", "details": [/* ... */] } }
```

**Errors (500)**

- Unknown provider name.
- Provider upstream errors.

```json
{ "error": { "message": "..." } }
```

## Configuration notes (runtime)

The server requires at least one provider configured. You can configure via YAML (recommended) or environment variables.

- YAML file lookup order:
  1. `LLM_GATEWAY_CONFIG` (path)
  2. `config.yaml` / `config.yml`
  3. `llm-gateway.yaml` / `llm-gateway.yml`

- Environment fallback:
  - `OPENROUTER_API_KEY` enables the `openrouter` provider
  - `OPENROUTER_BASE_URL` (optional)
  - `PORT` (default `3000`)
  - `HOST` (default `0.0.0.0`)

See `infra/gateway/config.example.yaml` for a full example.
