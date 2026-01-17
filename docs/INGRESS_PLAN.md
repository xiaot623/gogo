# Ingress Implementation Plan (MVP)

> **Status**: Draft (actionable)
>
> **References**:
> - Architecture: `docs/ARCHITECTURE_MVP.md`
> - Design details: `docs/INGRESS_IMPLEMENTATION.md`
> - Orchestrator internal API: `docs/api/Orchestrator.md` + `orchestrator/internal/transport/http/internalapi`

This plan combines a checklist-style execution plan with the milestone/acceptance detail of `docs/INGRESS_IMPLEMENTATION.md`, and aligns the contracts with the *current* orchestrator code (notably: `/internal/invoke` returns JSON; events are pushed via `/internal/send`).

## 1. Scope

### Goals (MVP)

- WebSocket ingress for client messages: `hello/agent_invoke/tool_result/approval_decision/cancel_run`.
- Connection management: register/unregister, ping/pong keepalive, multi-device support per `session_id`.
- Orchestrator bridge (downstream): call orchestrator internal APIs for invoke/tool/approval/cancel.
- Orchestrator fanout (upstream): receive `POST /internal/send` and forward event payload to the correct session’s active WebSocket connections.
- End-to-end loop works: Client → Ingress → Orchestrator → Agent (SSE) → Orchestrator → Ingress → Client.

### Non-goals (MVP)

- Offline buffering / durable delivery on ingress side (orchestrator already persists events for replay).
- Multi-tenant auth, rate limiting, and full IM-grade reliability (keep extension points).

## 2. Contracts (What Ingress Must Speak)

### 2.1 Client ↔ Ingress (WebSocket)

Follow `docs/ARCHITECTURE_MVP.md` §5.2/§5.3.

- Client → Ingress: `hello`, `agent_invoke`, `tool_result`, `approval_decision`, `cancel_run`
- Ingress → Client: `run_started`, `delta`, `state`, `tool_request`, `approval_required`, `done`, `error`
- **Ingress extension**: `hello_ack` (returns/echoes `session_id` after auth + binding)

### 2.2 Ingress → Orchestrator (HTTP client)

Align with orchestrator internal routes:

- `POST /internal/invoke`
- `POST /internal/tool_calls/:tool_call_id/submit`
- `POST /internal/approvals/:approval_id/submit`
- `POST /internal/runs/:run_id/cancel`

Required data transforms (MVP):

- `agent_invoke.message` → orchestrator `input_message`
- `tool_result.ok` → orchestrator `status` (`SUCCEEDED` if ok else `FAILED`)
- `approval_decision.decision` passes through (`approve|reject`)

### 2.3 Orchestrator → Ingress (HTTP server)

Ingress must expose:

- `POST /internal/send` with payload `{"session_id":"...", "event":{...}}`

Ingress behavior:

- Find all active connections bound to `session_id`.
- Forward `event` as JSON over WebSocket to all those connections.
- If no active connection: respond `200` and log (MVP); optionally include a response body like `{"ok":true,"delivered":false}` for debugging.

## 3. High-level Architecture Choices (Resolve Early)

1. **Ports**:
   - `WS_PORT` (external) for `/ws`
   - `HTTP_PORT` (internal) for `/internal/send` + `/health`
   - In docker compose: expose WS to host; keep internal HTTP unexposed; set orchestrator `INGRESS_URL=http://ingress:${HTTP_PORT}`.
2. **Frameworks**:
   - Use `echo/v4` for HTTP routing (consistency with orchestrator).
   - Use `gorilla/websocket` (or `nhooyr.io/websocket`) for WS upgrade + ping/pong.
3. **Connection model**:
   - Support multi-device: `session_id -> set(connection_id -> *Conn)`.
   - Broadcast all platform events to all connections of the same `session_id`.

## 4. Execution Plan (Milestones + Acceptance)

### M0 — Skeleton + Health

**Deliverables**
- Create `ingress/` module with minimal runnable server.
- Expose:
  - `GET /health` (on internal HTTP port)
  - `GET /ws` (WebSocket upgrade)

**Acceptance**
- `curl http://localhost:${HTTP_PORT}/health` returns ok.
- A WS client can connect to `/ws` and stays connected.

### M1 — Connection Registry + `hello` Handshake

**Deliverables**
- Connection manager:
  - `Register(sessionID, conn)` / `Unregister(connID)` / `Broadcast(sessionID, payload)`
  - ping/pong timeouts + write deadlines
- `hello` handling:
  - static `api_key` validation (MVP)
  - create/resume `session_id` (generate if missing)
  - bind the WS connection to the chosen `session_id`
  - respond `hello_ack` with `session_id`

**Acceptance**
- After sending `hello`, client receives `hello_ack` and can receive subsequent pushes on that session.
- Multiple WS connections using the same `session_id` all receive broadcasts.

### M2 — `agent_invoke` → Orchestrator Invoke

**Deliverables**
- Orchestrator client wrapper with:
  - `Invoke(session_id, agent_id, input_message, request_id, context)`
- `agent_invoke` WS handler:
  - validate required fields
  - call orchestrator `POST /internal/invoke`
  - on success: ensure client can correlate the run

**Notes (to avoid protocol drift)**
- Prefer: orchestrator pushes `run_started` via `/internal/send`.
- If orchestrator does not currently push `run_started`, ingress may *temporarily* synthesize a `run_started` message from the invoke response (guarded so it can be removed when orchestrator is fixed).

**Acceptance**
- Sending `agent_invoke` results in:
  - a `run_id` being created in orchestrator
  - client receiving streaming `delta` and final `done` (via `/internal/send` fanout)

### M3 — Fanout Bridge: `/internal/send` → WebSocket

**Deliverables**
- `POST /internal/send` handler:
  - validate payload shape
  - forward `event` to the target session’s WS connections
- Backpressure handling:
  - per-connection buffered send channel
  - drop/close policy when client is too slow (MVP: close + log)

**Acceptance**
- Orchestrator can push `delta/done/error/tool_request/approval_required` and client receives them in-order per connection.

### M4 — Client Tool + Approval + Cancel Loops

**Deliverables**
- `tool_result` handler:
  - map to orchestrator `POST /internal/tool_calls/:tool_call_id/submit`
- `approval_decision` handler:
  - map to orchestrator `POST /internal/approvals/:approval_id/submit`
- `cancel_run` handler:
  - map to orchestrator `POST /internal/runs/:run_id/cancel`

**Acceptance**
- When orchestrator sends `tool_request` / `approval_required`, the client can respond with `tool_result` / `approval_decision`, and the run continues.
- `cancel_run` stops an in-flight run (best-effort; orchestrator ultimately owns state).

### M5 (Optional) — Hardening

- Correlation: propagate `traceparent` + `run_id` in logs.
- Timeouts/retry: orchestrator HTTP client timeouts; exponential backoff for `/internal/invoke` (idempotent via `request_id`).
- Basic auth hardening: static API key allowlist + optional origin checks.

## 5. Suggested Project Layout (Ingress)

Pick one layout and stick to it (both map cleanly to `docs/INGRESS_IMPLEMENTATION.md`):

Option A (echo-oriented):

```text
ingress/
├── cmd/server/main.go
└── internal/
    ├── config/
    ├── http/           # internal HTTP: /health, /internal/send
    ├── ws/             # /ws: reader/writer loops, handshake, dispatch
    ├── hub/            # connection registry + broadcast
    ├── orchestrator/   # HTTP client to orchestrator internal APIs
    └── protocol/       # message structs (client <-> ingress)
```

Option B (close to design doc sections):

```text
ingress/
├── main.go
└── internal/
    ├── domain/         # connection + message models
    ├── service/        # hub + dispatcher
    ├── adapter/        # orchestrator HTTP client
    ├── transport/
    │   ├── ws/         # WebSocket server + connection loops
    │   └── http/       # internal HTTP: /internal/send, /health
    └── config/
```

## 6. Dependencies

- `github.com/labstack/echo/v4` (HTTP routing)
- WebSocket: `github.com/gorilla/websocket` (or `nhooyr.io/websocket`)
- Configuration: keep simple env parsing (add viper only if needed)

## 7. Configuration (Env Vars)

| Variable | Description | Default |
| :--- | :--- | :--- |
| `WS_PORT` | External WebSocket port | `8090` |
| `HTTP_PORT` | Internal HTTP port (`/internal/send`, `/health`) | `8091` |
| `ORCHESTRATOR_URL` | Orchestrator internal API base URL | `http://orchestrator:8081` |
| `API_KEY` | Static key for `hello.api_key` | (empty) |
| `LOG_LEVEL` | Logging level | `info` |
