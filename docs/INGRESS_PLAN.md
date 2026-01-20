# Ingress Implementation Plan (MVP)

> **Status**: Draft (actionable)
>
> **References**:
> - Architecture: `docs/ARCHITECTURE_MVP.md`
> - Design details: `docs/INGRESS_IMPLEMENTATION.md`
> - Orchestrator internal API: `docs/api/Orchestrator.md` + `orchestrator/internal/transport/http/internalapi`

This plan combines a checklist-style execution plan with the milestone/acceptance detail of `docs/INGRESS_IMPLEMENTATION.md`.

> Note: Internal ingress/orchestrator communication now uses RPC (JSON-RPC over TCP). HTTP endpoints like `/internal/invoke` and `/internal/send` are deprecated and replaced by RPC methods.

## 1. Scope

### Goals (MVP)

- WebSocket ingress for client messages: `hello/agent_invoke/tool_result/approval_decision/cancel_run`.
- Connection management: register/unregister, ping/pong keepalive, multi-device support per `session_id`.
- Orchestrator bridge (downstream): call orchestrator RPC methods for invoke/tool/approval/cancel.
- Orchestrator fanout (upstream): receive `Ingress.PushEvent` RPC calls and forward event payload to the correct session’s active WebSocket connections.
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

### 2.2 Ingress → Orchestrator (RPC client)

Align with orchestrator RPC methods:

- `Orchestrator.Invoke`
- `Orchestrator.SubmitToolResult`
- `Orchestrator.SubmitApprovalDecision`
- `Orchestrator.CancelRun`

Required data transforms (MVP):

- `agent_invoke.message` → orchestrator `input_message`
- `tool_result.ok` → orchestrator `status` (`SUCCEEDED` if ok else `FAILED`)
- `approval_decision.decision` passes through (`approve|reject`)

### 2.3 Orchestrator → Ingress (RPC server)

Ingress must expose:

- `Ingress.PushEvent` with payload `{"session_id":"...", "event":{...}}`

Ingress behavior:

- Find all active connections bound to `session_id`.
- Forward `event` as JSON over WebSocket to all those connections.
- If no active connection: respond `200` and log (MVP); optionally include a response body like `{"ok":true,"delivered":false}` for debugging.

## 3. High-level Architecture Choices (Resolve Early)

1. **Ports**:
   - `WS_PORT` (external) for `/ws`
   - `RPC_PORT` (internal) for ingress RPC fanout
   - In docker compose: expose WS to host; keep internal RPC unexposed; set orchestrator `INGRESS_RPC_ADDR=ingress:${RPC_PORT}`.
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
  - `GET /health` (on WebSocket port)
  - `GET /ws` (WebSocket upgrade)

**Acceptance**
- `curl http://localhost:${WS_PORT}/health` returns ok.
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
  - call orchestrator `Orchestrator.Invoke` RPC
  - on success: ensure client can correlate the run

**Notes (to avoid protocol drift)**
- Prefer: orchestrator pushes `run_started` via `Ingress.PushEvent` RPC.
- If orchestrator does not currently push `run_started`, ingress may *temporarily* synthesize a `run_started` message from the invoke response (guarded so it can be removed when orchestrator is fixed).

**Acceptance**
- Sending `agent_invoke` results in:
  - a `run_id` being created in orchestrator
  - client receiving streaming `delta` and final `done` (via `Ingress.PushEvent` fanout)

### M3 — Fanout Bridge: `Ingress.PushEvent` → WebSocket

**Deliverables**
- `Ingress.PushEvent` RPC handler:
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
  - map to orchestrator `Orchestrator.SubmitToolResult` RPC
- `approval_decision` handler:
  - map to orchestrator `Orchestrator.SubmitApprovalDecision` RPC
- `cancel_run` handler:
  - map to orchestrator `Orchestrator.CancelRun` RPC

**Acceptance**
- When orchestrator sends `tool_request` / `approval_required`, the client can respond with `tool_result` / `approval_decision`, and the run continues.
- `cancel_run` stops an in-flight run (best-effort; orchestrator ultimately owns state).

### M5 (Optional) — Hardening

- Correlation: propagate `traceparent` + `run_id` in logs.
- Timeouts/retry: orchestrator RPC client timeouts; exponential backoff for `Orchestrator.Invoke` (idempotent via `request_id`).
- Basic auth hardening: static API key allowlist + optional origin checks.

## 5. Suggested Project Layout (Ingress)

Pick one layout and stick to it (both map cleanly to `docs/INGRESS_IMPLEMENTATION.md`):

Option A (echo-oriented):

```text
ingress/
├── cmd/server/main.go
└── internal/
    ├── config/
    ├── rpc/            # ingress RPC server (Ingress.PushEvent)
    ├── ws/             # /ws + /health: reader/writer loops, handshake, dispatch
    ├── hub/            # connection registry + broadcast
    ├── orchestrator/   # RPC client to orchestrator
    └── protocol/       # message structs (client <-> ingress)
```

Option B (close to design doc sections):

```text
ingress/
├── main.go
└── internal/
    ├── domain/         # connection + message models
    ├── service/        # hub + dispatcher
    ├── adapter/        # orchestrator RPC client
    ├── transport/
    │   ├── ws/         # WebSocket server + connection loops
    │   └── rpc/        # ingress RPC server (Ingress.PushEvent)
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
| `RPC_PORT` | Internal RPC port | `8091` |
| `ORCHESTRATOR_RPC_ADDR` | Orchestrator RPC address | `orchestrator:8081` |
| `API_KEY` | Static key for `hello.api_key` | (empty) |
| `LOG_LEVEL` | Logging level | `info` |
