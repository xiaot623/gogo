# Ingress Service

The Ingress service provides WebSocket connectivity between clients and the orchestrator. It handles:

- **WebSocket connections** for real-time bidirectional communication
- **Connection management** with session binding and multi-device support
- **Protocol translation** between WebSocket messages and orchestrator RPC APIs
- **Event fanout** from orchestrator to connected WebSocket clients

## Architecture

```
┌─────────────┐
│   Client    │
└──────┬──────┘
       │ (WebSocket)
       ▼
┌──────────────────────────────┐
│        Ingress Service       │
│ ┌────────────────────────┐   │
│ │ WebSocket Server (:8090)│   │
│ │ - hello/agent_invoke   │   │
│ │ - tool_result/approval │   │
│ │ - cancel_run           │   │
│ └────────────────────────┘   │
│                              │
│ ┌────────────────────────┐   │
│ │ Internal RPC (:8091)   │   │
│ │ - PushEvent            │   │
│ └────────────────────────┘   │
│                              │
│ ┌────────────────────────┐   │
│ │ Connection Hub         │   │
│ │ - session_id -> conns  │   │
│ │ - broadcast            │   │
│ └────────────────────────┘   │
└──────────────────────────────┘
       │
       │ RPC calls
       ▼
┌──────────────────────────────┐
│       Orchestrator           │
└──────────────────────────────┘
```

## Ports

| Port | Protocol | Purpose |
|------|----------|---------|
| 8090 | WebSocket/HTTP | Client connections (`/ws`), health (`/health`) |
| 8091 | RPC (TCP) | Internal event fanout |

## Configuration

| Variable | Description | Default |
|----------|-------------|---------|
| `WS_PORT` | External WebSocket port | `8090` |
| `RPC_PORT` | Internal RPC port | `8091` |
| `ORCHESTRATOR_RPC_ADDR` | Orchestrator RPC address | `orchestrator:8081` |
| `API_KEY` | Static key for hello.api_key validation | (empty) |
| `LOG_LEVEL` | Logging level | `info` |
| `WS_PING_INTERVAL_MS` | WebSocket ping interval | `30000` |
| `WS_WRITE_TIMEOUT_MS` | WebSocket write timeout | `10000` |
| `WS_READ_TIMEOUT_MS` | WebSocket read timeout | `60000` |
| `WS_MAX_MESSAGE_SIZE` | Max message size in bytes | `65536` |

Legacy environment variables `HTTP_PORT` and `ORCHESTRATOR_URL` are still supported.

## WebSocket Protocol

### Client → Ingress

#### `hello` - Establish connection

```json
{
  "type": "hello",
  "ts": 1704067200000,
  "api_key": "sk-xxx",
  "session_id": "sess_001",
  "client_meta": {"app": "web", "version": "1.0.0"}
}
```

#### `agent_invoke` - Invoke an agent

```json
{
  "type": "agent_invoke",
  "ts": 1704067200000,
  "request_id": "req_abc123",
  "session_id": "sess_001",
  "agent_id": "agent_a",
  "message": {
    "role": "user",
    "content": "Hello, how are you?"
  }
}
```

#### `tool_result` - Submit tool result

```json
{
  "type": "tool_result",
  "ts": 1704067200000,
  "run_id": "run_001",
  "tool_call_id": "tc_001",
  "ok": true,
  "result": {"file_path": "/tmp/screenshot.png"}
}
```

#### `approval_decision` - Submit approval decision

```json
{
  "type": "approval_decision",
  "ts": 1704067200000,
  "run_id": "run_001",
  "approval_id": "ap_001",
  "decision": "approve",
  "reason": "Confirmed"
}
```

#### `cancel_run` - Cancel a run

```json
{
  "type": "cancel_run",
  "ts": 1704067200000,
  "run_id": "run_001"
}
```

### Ingress → Client

#### `hello_ack` - Connection confirmed

```json
{
  "type": "hello_ack",
  "ts": 1704067200000,
  "session_id": "sess_001"
}
```

#### `run_started`, `delta`, `done`, `error`, `tool_request`, `approval_required`

These events are forwarded from the orchestrator via the `Ingress.PushEvent` RPC call.

## HTTP Endpoints (WebSocket server)

### `GET /health`

Health check endpoint (served on the WebSocket port).

**Response:**
```json
{
  "status": "healthy",
  "connections": 5,
  "sessions": 3
}
```

## Internal RPC API

### `Ingress.PushEvent`

Receive events from orchestrator and forward to WebSocket clients.

**Request:**
```json
{
  "session_id": "sess_001",
  "event": {
    "type": "delta",
    "ts": 1704067200000,
    "run_id": "run_001",
    "text": "Hello world"
  }
}
```

**Response:**
```json
{
  "ok": true,
  "delivered": true
}
```

## Running Locally

```bash
cd ingress
go run .
```

## Building

```bash
cd ingress
go build -o ingress .
```

## Docker

```bash
docker build -t ingress .
docker run -p 8090:8090 -p 8091:8091 ingress
```

## Testing Connection

Using websocat:
```bash
websocat ws://localhost:8090/ws
# Send: {"type":"hello","ts":1704067200000,"api_key":""}
# Receive: {"type":"hello_ack","ts":...,"session_id":"sess_..."}
```

Health check:
```bash
curl http://localhost:8090/health
```
