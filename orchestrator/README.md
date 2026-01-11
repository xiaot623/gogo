# Orchestrator

The Orchestrator is the core service of the multi-agent platform. It manages agent invocations, event streaming, state machines, and provides a unified API for the platform.

## Features

- **Agent Invocation**: Invoke registered agents via HTTP + SSE streaming
- **Event Recording**: Append-only event log for full trace and replay
- **Session Management**: Maintain conversation sessions and message history
- **Agent Registry**: Register and discover agents dynamically
- **State Machine**: Track run lifecycle (CREATED → RUNNING → DONE/FAILED)

## Quick Start

### Prerequisites

- Go 1.21+
- SQLite (embedded, no separate installation needed)

### Build

```bash
cd orchestrator
go mod tidy
go build -o orchestrator .
```

### Run

```bash
./orchestrator
```

The server starts on port 8080 by default.

### Verify

```bash
curl http://localhost:8080/health
```

## Configuration

Configure via environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `HTTP_PORT` | 8080 | HTTP server port |
| `DATABASE_URL` | `file:orchestrator.db?cache=shared&mode=rwc` | SQLite database path |
| `INGRESS_URL` | `http://localhost:8090` | Ingress service URL for event push |
| `AGENT_TIMEOUT_MS` | 300000 | Agent invocation timeout (5 min) |
| `LOG_LEVEL` | info | Logging level |

Example:

```bash
HTTP_PORT=9000 DATABASE_URL="file:data.db" ./orchestrator
```

## Usage

### 1. Register an Agent

```bash
curl -X POST http://localhost:8080/v1/agents/register \
  -H "Content-Type: application/json" \
  -d '{
    "agent_id": "demo_agent",
    "name": "Demo Agent",
    "endpoint": "http://localhost:8000"
  }'
```

### 2. Invoke an Agent

```bash
curl -X POST http://localhost:8080/internal/invoke \
  -H "Content-Type: application/json" \
  -d '{
    "session_id": "sess_001",
    "agent_id": "demo_agent",
    "input_message": {
      "role": "user",
      "content": "Hello, how are you?"
    }
  }'
```

Response:

```json
{
  "run_id": "run_abc123",
  "session_id": "sess_001",
  "agent_id": "demo_agent"
}
```

### 3. Get Run Events (Replay)

```bash
curl http://localhost:8080/v1/runs/run_abc123/events
```

### 4. Get Session Messages

```bash
curl http://localhost:8080/v1/sessions/sess_001/messages
```

## API Reference

See [API.md](./API.md) for complete API documentation.

### Key Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/internal/invoke` | Invoke an agent (from Ingress) |
| GET | `/v1/runs/:run_id/events` | Get events for replay |
| GET | `/v1/sessions/:session_id/messages` | Get session messages |
| POST | `/v1/agents/register` | Register an agent |
| GET | `/v1/agents` | List all agents |
| GET | `/health` | Health check |

## Architecture

```
orchestrator/
├── main.go              # Application entrypoint
├── domain/
│   └── models.go        # Domain models (Session, Run, Event, Message, Agent)
├── store/
│   ├── store.go         # Store interface
│   └── sqlite.go        # SQLite implementation
├── api/
│   └── handler.go       # HTTP handlers (Echo framework)
├── agentclient/
│   └── client.go        # SSE client for agent invocation
└── config/
    └── config.go        # Configuration management
```

## Data Flow

```
                                    ┌─────────────┐
                                    │   Agent     │
                                    │  (External) │
                                    └──────▲──────┘
                                           │ SSE
┌─────────┐     ┌─────────────┐     ┌──────┴──────┐     ┌──────────┐
│ Ingress │────▶│ Orchestrator│────▶│ AgentClient │     │  SQLite  │
└─────────┘     └──────┬──────┘     └─────────────┘     └────▲─────┘
     ▲                 │                                     │
     │                 └─────────────────────────────────────┘
     │                        Store events/messages
     │
     └── Push events via POST /internal/send
```

## Event Types

| Event | Description |
|-------|-------------|
| `run_started` | Run execution began |
| `user_input` | User message recorded |
| `agent_invoke_started` | Agent invocation started |
| `agent_stream_delta` | Streaming text from agent |
| `agent_invoke_done` | Agent completed |
| `run_done` | Run completed successfully |
| `run_failed` | Run failed with error |

## Agent Protocol

Agents must implement `POST /invoke` endpoint that returns SSE events:

```
event: delta
data: {"text": "Hello", "run_id": "run_001"}

event: delta
data: {"text": " world!", "run_id": "run_001"}

event: done
data: {"final_message": "Hello world!", "usage": {"tokens": 10}}
```

See [API.md](./API.md#agent-protocol) for details.

## Database Schema

The orchestrator uses SQLite with the following tables:

- `sessions` - Conversation sessions
- `messages` - Chat messages (transcript)
- `runs` - Execution runs with status
- `events` - Append-only event log
- `agents` - Registered agents

Tables are auto-created on startup.

## Development

### Run Tests

```bash
go test ./...
```

### Build for Production

```bash
CGO_ENABLED=1 go build -ldflags="-s -w" -o orchestrator .
```

Note: CGO is required for SQLite.

## Roadmap (M0 → M1)

- [x] M0: Basic invoke flow with SSE streaming
- [x] M0: Event recording and replay
- [x] M0: Agent registry
- [ ] M1: Agent health checks and heartbeat
- [ ] M2: LLM proxy (`/v1/chat/completions`)
- [ ] M3: Tool invocation support
- [ ] M4: Client tool dispatch
- [ ] M5: Approval workflow

## License

Internal use only.
