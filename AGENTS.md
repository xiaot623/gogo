# Multi-Agent Platform - Implementation Guide

> Working reference for `docs/ARCHITECTURE_MVP.md`.

---

## 1. Implementation Order

1. **Orchestrator** - Data model, state machine, Agent invoke protocol, events table (source of truth) ✅
2. **Agent (Demo)** - Validate orchestrator's SSE handling
3. **Ingress** - WebSocket adapter (can bypass early via direct HTTP)

---

## 2. M0 Milestone

**Goal:** `Ingress → Orchestrator → Agent → SSE → Events persisted → Client`

### Orchestrator (M0) ✅ DONE

- [x] `POST /internal/invoke` - create run, call agent, stream events
- [x] `GET /v1/runs/{run_id}/events` - replay
- [x] Tables: `sessions`, `runs`, `events`, `messages`, `agents`
- [x] Events: `run_started`, `user_input`, `agent_invoke_started`, `agent_stream_delta`, `agent_invoke_done`, `run_done`, `run_failed`
- [x] **Push to Ingress**: call `POST /internal/send` to deliver events to client
- [x] Agent registry: `POST /v1/agents/register`, `GET /v1/agents`

**Location:** `orchestrator/`

**Run:**
```bash
cd orchestrator && go build && ./orchestrator
```

**Test:**
```bash
# Health check
curl http://localhost:8080/health

# Register agent
curl -X POST http://localhost:8080/v1/agents/register \
  -H "Content-Type: application/json" \
  -d '{"agent_id":"demo","name":"Demo Agent","endpoint":"http://localhost:8000"}'

# Invoke (requires agent running)
curl -X POST http://localhost:8080/internal/invoke \
  -H "Content-Type: application/json" \
  -d '{"session_id":"s1","agent_id":"demo","input_message":{"role":"user","content":"hello"}}'
```

### Agent Demo (M0)

- `POST /invoke` → SSE (`delta`, `done`)
- Echo input with simulated streaming

### Ingress (M0)

- WebSocket: `hello`, `agent_invoke` → call orchestrator `/internal/invoke`
- `POST /internal/send` - receive events from orchestrator, push to client via WebSocket
- Connection registry: map `session_id` → WebSocket conn

---

## 3. Directory Structure

```
orchestrator/           # Go: domain, store, api, agent client ✅
├── main.go
├── domain/models.go
├── store/sqlite.go
├── api/handler.go
├── agentclient/client.go
├── config/config.go
├── README.md
└── API.md

ingress/                # Go: ws, protocol, orchestrator client (TODO)

agent-demo/             # Python: FastAPI + SSE (TODO)
```

---

## 4. Tech Stack

| Component    | Lang   | Framework         | Status |
| ------------ | ------ | ----------------- | ------ |
| Orchestrator | Go     | echo              | ✅ Done |
| Ingress      | Go     | gorilla/websocket | TODO   |
| Agent Demo   | Python | FastAPI           | TODO   |
| DB           | SQLite | -                 | ✅ Done |

---

## 5. API Summary

### Orchestrator Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/internal/invoke` | Invoke agent (from Ingress) |
| GET | `/v1/runs/:run_id/events` | Replay events |
| GET | `/v1/sessions/:session_id/messages` | Get messages |
| POST | `/v1/agents/register` | Register agent |
| GET | `/v1/agents` | List agents |
| GET | `/health` | Health check |

### Event Flow

```
Ingress                 Orchestrator              Agent
   │                         │                      │
   │  POST /internal/invoke  │                      │
   │────────────────────────>│                      │
   │                         │   POST /invoke       │
   │                         │─────────────────────>│
   │                         │                      │
   │                         │   SSE: delta         │
   │                         │<─────────────────────│
   │  POST /internal/send    │                      │
   │<────────────────────────│                      │
   │                         │   SSE: done          │
   │                         │<─────────────────────│
   │  POST /internal/send    │                      │
   │<────────────────────────│                      │
```

---

## 6. Open Questions

- [x] Confirm Go + Python stack → **Confirmed**
- [ ] Single-process mode for local dev?
- [ ] M0 auth: static API key?

---

## 7. Next Steps

1. **Agent Demo** - Python FastAPI service with SSE streaming
2. **Ingress** - WebSocket server with connection registry
3. **End-to-end test** - Client → Ingress → Orchestrator → Agent → back
