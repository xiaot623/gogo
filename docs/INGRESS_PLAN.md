# Ingress (Access Layer) Execution Plan

> **Status**: Draft
> **Based on**: `docs/ARCHITECTURE_MVP.md` v1.0

This document outlines the execution plan for implementing the **Ingress** service, which serves as the unified access layer for the Multi-Agent Platform.

## 1. Goal

Implement the `ingress` service to handle external communication (WebSocket) and bridge it with the `orchestrator` service (HTTP).

**Key Responsibilities:**
- **Protocol Conversion**: External Message (WebSocket) <-> Internal Command (HTTP).
- **Connection Management**: Maintain WebSocket connections with Clients.
- **Event Delivery**: Push events from Orchestrator to the correct Client connection.

---

## 2. Architecture Review

### Interactions

1.  **Client -> Ingress (WebSocket)**:
    -   `hello`: Connection initialization.
    -   `agent_invoke`: Start a run.
    -   `tool_result`: Submit client tool result.
    -   `approval_decision`: Submit approval.
    -   `cancel_run`: Cancel execution.

2.  **Ingress -> Orchestrator (HTTP Client)**:
    -   `POST /internal/invoke`: Trigger agent execution.
    -   `POST /internal/tool_calls/{id}/submit`: Forward tool result.
    -   `POST /internal/approvals/{id}/submit`: Forward approval decision.
    -   `POST /internal/runs/{id}/cancel`: Forward cancellation.

3.  **Orchestrator -> Ingress (HTTP Server)**:
    -   `POST /internal/send`: Orchestrator pushes events (deltas, state changes, etc.).
    -   Ingress looks up the WebSocket connection by `session_id` and pushes the payload.

---

## 3. Implementation Steps

### Phase 1: Project Skeleton & Service Setup

-   [ ] **Create Directory**: `ingress/`
-   [ ] **Initialize Module**: `go mod init github.com/xiaot623/gogo/ingress`
-   [ ] **Project Structure**:
    ```text
    ingress/
    ├── cmd/
    │   └── server/
    │       └── main.go       # Entry point
    ├── internal/
    │   ├── config/           # Env vars (PORT, ORCHESTRATOR_URL)
    │   ├── connection/       # WebSocket connection manager
    │   ├── handler/          # HTTP & WebSocket handlers
    │   ├── service/          # Business logic & Orchestrator Client
    │   └── model/            # Protocol structs
    ├── Dockerfile
    └── go.mod
    ```
-   [ ] **Docker Integration**: Add `ingress` service to root `docker-compose.yaml`.

### Phase 2: Core Components (WebSocket & Connections)

-   [ ] **Protocol Models**: Define structs for all Client and Platform messages in `internal/model`.
-   [ ] **Connection Manager (`internal/connection`)**:
    -   `Manager` struct with thread-safe map: `map[sessionID]*Connection`.
    -   Methods: `Register`, `Unregister`, `Get`.
-   [ ] **WebSocket Handler (`internal/handler/ws.go`)**:
    -   Upgrade HTTP to WebSocket (using `github.com/gorilla/websocket` or `nhooyr.io/websocket`).
    -   **Read Loop**: Parse incoming JSON -> Dispatch to Service.
    -   **Write Loop**: Channel-based write to ensure concurrency safety.
    -   Handle `hello` message to bind `session_id` to connection.

### Phase 3: Orchestrator Client (Downstream)

-   [ ] **Orchestrator Client (`internal/service/orchestrator_client.go`)**:
    -   Implement methods calling `orchestrator` Internal APIs:
        -   `InvokeAgent(req domain.InvokeRequest)`
        -   `SubmitToolResult(id string, result domain.Result)`
        -   `SubmitApproval(id string, decision string)`
        -   `CancelRun(id string)`
-   [ ] **Message Processing**:
    -   Map `agent_invoke` -> `InvokeAgent`.
    -   Map `tool_result` -> `SubmitToolResult`.
    -   Map `approval_decision` -> `SubmitApproval`.

### Phase 4: Internal Event Handler (Upstream)

-   [ ] **Internal HTTP Server**:
    -   Listen on internal port (or same port with specific path).
-   [ ] **Event Handler (`internal/handler/internal.go`)**:
    -   Endpoint: `POST /internal/send`.
    -   Payload: `{"session_id": "...", "event": {...}}`.
    -   Logic:
        1.  Extract `session_id`.
        2.  Get Connection from Manager.
        3.  If found, push event to WebSocket write channel.
        4.  If not found, log warning (or handle offline logic if P1).

### Phase 5: Testing & Verification

-   [ ] **Unit Tests**: Protocol marshaling/unmarshaling, Connection Manager logic.
-   [ ] **Integration**:
    -   Spin up `orchestrator` + `ingress`.
    -   Use `wscat` to connect to Ingress.
    -   Send `agent_invoke` and verify `run_started` and `delta` events are received.

---

## 4. Dependencies

-   **Go Modules**:
    -   `github.com/labstack/echo/v4` (HTTP Framework, consistent with Orchestrator)
    -   `github.com/gorilla/websocket` (WebSocket)
    -   `github.com/spf13/viper` (Configuration)

## 5. Configuration (Env Vars)

| Variable | Description | Default |
| :--- | :--- | :--- |
| `PORT` | HTTP/WebSocket Port | `8090` |
| `ORCHESTRATOR_URL` | Orchestrator Internal API Base URL | `http://orchestrator:8081` |
| `LOG_LEVEL` | Logging level | `info` |

