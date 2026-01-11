# Orchestrator API Documentation

> **Version**: 0.1.0 (M0)  
> **Base URL**: `http://localhost:8080`

---

## Overview

The Orchestrator is the core service of the multi-agent platform. It handles:
- Agent invocation and SSE streaming
- Run state management
- Event recording and replay
- Session and message storage
- Agent registry

---

## Authentication

M0 does not implement authentication. Future versions will support API key authentication.

---

## Endpoints

### Health Check

#### `GET /health`

Returns the service health status.

**Response**

```json
{
  "status": "healthy",
  "version": "0.1.0"
}
```

---

### Internal API (for Ingress)

#### `POST /internal/invoke`

Invokes an agent to handle a user message. This endpoint is called by the Ingress service.

**Request Body**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `session_id` | string | Yes | Session identifier |
| `agent_id` | string | Yes | ID of the agent to invoke |
| `input_message` | object | Yes | User's input message |
| `input_message.role` | string | Yes | Message role (typically "user") |
| `input_message.content` | string | Yes | Message content |
| `request_id` | string | No | Client-generated request ID for idempotency |
| `context` | object | No | Additional context (e.g., `user_id`, `timezone`) |

**Example Request**

```json
{
  "session_id": "sess_001",
  "agent_id": "demo_agent",
  "input_message": {
    "role": "user",
    "content": "Hello, what's the weather today?"
  },
  "request_id": "req_abc123",
  "context": {
    "user_id": "u1",
    "timezone": "Asia/Shanghai"
  }
}
```

**Response**

```json
{
  "run_id": "run_d43a87e9",
  "session_id": "sess_001",
  "agent_id": "demo_agent"
}
```

**Response Codes**

| Code | Description |
|------|-------------|
| 200 | Run created successfully |
| 400 | Invalid request (missing required fields) |
| 404 | Agent not found |
| 500 | Internal server error |

**Notes**

- The agent is invoked asynchronously after the response is returned
- Events are pushed to the Ingress service via `POST /internal/send`
- Events are also persisted and can be replayed via `/v1/runs/:run_id/events`

---

### Run Events

#### `GET /v1/runs/:run_id/events`

Retrieves the event stream for a run. Used for replay and debugging.

**Path Parameters**

| Parameter | Description |
|-----------|-------------|
| `run_id` | The run identifier |

**Query Parameters**

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `after_ts` | int64 | 0 | Return events after this timestamp (Unix ms) |
| `types` | string | all | Comma-separated event types to filter |
| `limit` | int | 100 | Maximum number of events to return |

**Example Request**

```
GET /v1/runs/run_d43a87e9/events?limit=50&types=run_started,agent_stream_delta,run_done
```

**Response**

```json
{
  "events": [
    {
      "event_id": "evt_80281856",
      "run_id": "run_d43a87e9",
      "ts": 1768109957143,
      "type": "run_started",
      "payload": {
        "session_id": "sess_001",
        "agent_id": "demo_agent"
      }
    },
    {
      "event_id": "evt_120e3076",
      "run_id": "run_d43a87e9",
      "ts": 1768109957143,
      "type": "user_input",
      "payload": {
        "message_id": "msg_79c0257e",
        "content": "Hello, what's the weather today?"
      }
    },
    {
      "event_id": "evt_aa852198",
      "run_id": "run_d43a87e9",
      "ts": 1768109957144,
      "type": "agent_stream_delta",
      "payload": {
        "text": "The weather today is"
      }
    }
  ],
  "has_more": true,
  "next_cursor": "evt_aa852198"
}
```

**Event Types**

| Type | Description |
|------|-------------|
| `run_started` | Run execution started |
| `user_input` | User input message recorded |
| `agent_invoke_started` | Agent invocation initiated |
| `agent_stream_delta` | Streaming text chunk from agent |
| `agent_invoke_done` | Agent completed execution |
| `run_done` | Run completed successfully |
| `run_failed` | Run failed with error |

**Response Codes**

| Code | Description |
|------|-------------|
| 200 | Success |
| 404 | Run not found |
| 500 | Internal server error |

---

### Session Messages

#### `GET /v1/sessions/:session_id/messages`

Retrieves messages (transcript) for a session.

**Path Parameters**

| Parameter | Description |
|-----------|-------------|
| `session_id` | The session identifier |

**Query Parameters**

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `limit` | int | 50 | Maximum number of messages to return |
| `before` | string | - | Return messages before this message_id (cursor) |

**Example Request**

```
GET /v1/sessions/sess_001/messages?limit=20
```

**Response**

```json
{
  "messages": [
    {
      "message_id": "msg_001",
      "session_id": "sess_001",
      "run_id": "run_001",
      "role": "user",
      "content": "Hello",
      "created_at": "2024-01-15T10:00:00Z"
    },
    {
      "message_id": "msg_002",
      "session_id": "sess_001",
      "run_id": "run_001",
      "role": "assistant",
      "content": "Hello! How can I help you today?",
      "created_at": "2024-01-15T10:00:01Z"
    }
  ],
  "has_more": false
}
```

**Response Codes**

| Code | Description |
|------|-------------|
| 200 | Success |
| 500 | Internal server error |

---

### Agent Registry

#### `POST /v1/agents/register`

Registers a new agent or updates an existing one.

**Request Body**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `agent_id` | string | Yes | Unique agent identifier |
| `name` | string | Yes | Human-readable agent name |
| `endpoint` | string | Yes | Agent HTTP endpoint URL |
| `capabilities` | array | No | List of capability strings |

**Example Request**

```json
{
  "agent_id": "weather_agent",
  "name": "Weather Query Agent",
  "endpoint": "http://weather-agent:8000",
  "capabilities": ["weather_query", "location_parse"]
}
```

**Response**

```json
{
  "ok": true,
  "registered_at": 1768109933936
}
```

**Response Codes**

| Code | Description |
|------|-------------|
| 200 | Agent registered successfully |
| 400 | Invalid request |
| 500 | Internal server error |

---

#### `GET /v1/agents`

Lists all registered agents.

**Response**

```json
{
  "agents": [
    {
      "agent_id": "weather_agent",
      "name": "Weather Query Agent",
      "status": "healthy",
      "last_heartbeat_at": 1768109933936
    },
    {
      "agent_id": "demo_agent",
      "name": "Demo Agent",
      "status": "healthy",
      "last_heartbeat_at": null
    }
  ]
}
```

---

#### `GET /v1/agents/:agent_id`

Gets details of a specific agent.

**Path Parameters**

| Parameter | Description |
|-----------|-------------|
| `agent_id` | The agent identifier |

**Response**

```json
{
  "agent_id": "weather_agent",
  "name": "Weather Query Agent",
  "endpoint": "http://weather-agent:8000",
  "capabilities": ["weather_query", "location_parse"],
  "status": "healthy",
  "last_heartbeat": "2024-01-15T10:00:00Z",
  "created_at": "2024-01-14T08:00:00Z"
}
```

**Response Codes**

| Code | Description |
|------|-------------|
| 200 | Success |
| 404 | Agent not found |
| 500 | Internal server error |

---

## Event Payloads

### `run_started`

```json
{
  "request_id": "req_abc123",
  "session_id": "sess_001",
  "agent_id": "demo_agent"
}
```

### `user_input`

```json
{
  "message_id": "msg_79c0257e",
  "content": "Hello world"
}
```

### `agent_invoke_started`

```json
{
  "agent_id": "demo_agent",
  "endpoint": "http://localhost:8000"
}
```

### `agent_stream_delta`

```json
{
  "text": "The weather today is"
}
```

### `agent_invoke_done`

```json
{
  "final_message": "The weather today is sunny with 25°C.",
  "usage": {
    "tokens": 150,
    "prompt_tokens": 50,
    "completion_tokens": 100,
    "duration_ms": 1500
  }
}
```

### `run_done`

```json
{
  "final_message": "The weather today is sunny with 25°C.",
  "usage": {
    "total_tokens": 150,
    "prompt_tokens": 50,
    "completion_tokens": 100,
    "duration_ms": 1500
  }
}
```

### `run_failed`

```json
{
  "code": "agent_error",
  "message": "Connection refused"
}
```

---

## Error Responses

All error responses follow this format:

```json
{
  "error": "error message here"
}
```

### Common Error Codes

| HTTP Code | Description |
|-----------|-------------|
| 400 | Bad Request - Invalid input |
| 404 | Not Found - Resource doesn't exist |
| 500 | Internal Server Error |

---

## Configuration

The orchestrator is configured via environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `HTTP_PORT` | 8080 | HTTP server port |
| `INTERNAL_PORT` | 8081 | Internal API port |
| `DATABASE_URL` | `file:orchestrator.db?cache=shared&mode=rwc` | SQLite database path |
| `INGRESS_URL` | `http://localhost:8090` | Ingress service URL |
| `AGENT_TIMEOUT_MS` | 300000 | Agent invocation timeout (5 min) |
| `TOOL_TIMEOUT_MS` | 60000 | Tool execution timeout |
| `APPROVAL_TIMEOUT_MS` | 600000 | Approval timeout (10 min) |
| `LOG_LEVEL` | info | Logging level |

---

## Agent Protocol

The orchestrator communicates with agents using HTTP + SSE:

### Agent Endpoint: `POST /invoke`

**Request Headers**

| Header | Description |
|--------|-------------|
| `Content-Type` | `application/json` |
| `Accept` | `text/event-stream` |
| `X-Session-ID` | Session identifier |
| `X-Run-ID` | Run identifier |

**Request Body**

```json
{
  "agent_id": "demo_agent",
  "session_id": "sess_001",
  "run_id": "run_001",
  "input_message": {
    "role": "user",
    "content": "Hello"
  },
  "messages": [
    {"role": "user", "content": "Previous message"},
    {"role": "assistant", "content": "Previous response"}
  ],
  "context": {
    "user_id": "u1"
  }
}
```

**SSE Response Events**

```
event: delta
data: {"text": "Hello", "run_id": "run_001"}

event: delta
data: {"text": " there!", "run_id": "run_001"}

event: done
data: {"usage": {"tokens": 10}, "final_message": "Hello there!"}
```

### SSE Event Types

| Event | Description |
|-------|-------------|
| `delta` | Streaming text chunk |
| `done` | Execution completed |
| `error` | Execution failed |
| `state` | State change notification |
