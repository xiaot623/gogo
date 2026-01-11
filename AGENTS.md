# Multi-Agent Platform - Implementation Guide

> Working reference for `docs/ARCHITECTURE_MVP.md`.

---

## 1. Implementation Order

1. **Orchestrator** - Data model, state machine, Agent invoke protocol, events table (source of truth)
2. **Agent (Demo)** - Validate orchestrator's SSE handling
3. **Ingress** - WebSocket adapter (can bypass early via direct HTTP)

---

## 2. M0 Milestone

**Goal:** `Ingress → Orchestrator → Agent → SSE → Events persisted → Client`

### Orchestrator (M0)

- `POST /internal/invoke` - create run, call agent, stream events
- `GET /v1/runs/{run_id}/events` - replay
- Tables: `sessions`, `runs`, `events`, `messages`
- Events: `run_started`, `user_input`, `agent_stream_delta`, `run_done`, `run_failed`
- **Push to Ingress**: call `POST /internal/send` to deliver events to client

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
orchestrator/   # Go: domain, store, api, agent client
ingress/        # Go: ws, protocol, orchestrator client
agent-demo/     # Python: FastAPI + SSE
```

---

## 4. Tech Stack

| Component    | Lang                      | Framework         |
| ------------ | ------------------------- | ----------------- |
| Orchestrator | Go                        | echo              |
| Ingress      | Go                        | gorilla/websocket |
| Agent Demo   | Python                    | FastAPI           |
| DB           | SQLite (dev) / PostgreSQL |                   |

---

## 5. Open Questions

- [ ] Confirm Go + Python stack
- [ ] Single-process mode for local dev?
- [ ] M0 auth: static API key?
