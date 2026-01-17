# Transport Layer Refactor Plan

## Objective
Organize the `orchestrator/internal/transport` directory to improved maintainability, separation of concerns, and scalability. The current structure mixes public API handlers with the transport entry point, making it harder to add new API versions or distinguish between different API surfaces.

## Proposed Structure

```
orchestrator/internal/transport/
└── http/
    ├── server.go        # (NEW) Centralized Server/Echo setup
    ├── v1/              # (NEW) Public API V1 Handlers
    │   ├── handler.go   # (MOVED/RENAMED) Handler struct & RegisterRoutes
    │   ├── agents.go    # (MOVED)
    │   ├── tools.go     # (MOVED)
    │   ├── ...          # (MOVED) all other public handler files
    ├── internalapi/     # Existing Internal API Handlers
    │   ├── handler.go
    │   └── ...
    └── llmproxy/        # Existing LLM Proxy Handlers
        ├── handler.go
        └── ...
```

## Key Changes

1.  **Create `v1` Package**:
    -   Move `agents.go`, `approvals.go`, `messages.go`, `tools.go` (and their tests) from `transport/http/` to `transport/http/v1/`.
    -   Move `handler.go` to `transport/http/v1/handler.go` and update its package to `v1`.

2.  **Create `transport/http/server.go`**:
    -   This file will act as the entry point for the HTTP transport layer.
    -   It will provide functions like `NewExternalServer(svc, cfg)` and `NewInternalServer(svc, cfg)` that return configured `*echo.Echo` instances.
    -   It will handle middleware registration (Logger, Recover, CORS) and route registration by calling `v1.NewHandler`, `internalapi.NewHandler`, etc.

3.  **Refactor `main.go`**:
    -   Simplify `main.go` by removing direct Echo configuration and route registration.
    -   Instead, call `transport.NewExternalServer(...)` and `transport.NewInternalServer(...)`.

## Benefits

-   **Separation of Concerns**: Handlers are grouped by their API surface (V1, Internal, LLM Proxy).
-   **Centralized Configuration**: Server configuration (middleware, timeouts) is encapsulated in `server.go`, keeping `main.go` clean.
-   **Extensibility**: Easy to add `v2` in the future without cluttering the root directory.
