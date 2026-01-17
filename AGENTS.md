# Multi-Agent Platform - Implementation Guide (Core)

Reference: `docs/ARCHITECTURE_MVP.md`

## Implementation Order
1. Orchestrator ✅
2. Demo Agent ✅
3. Ingress (WebSocket) - TODO

## Current Status
- Orchestrator live: run creation, SSE invoke, event persistence/replay, OPA tool policy, agent registry, push-to-ingress hook.
- LLM proxy live: OpenAI-compatible `/v1/chat/completions` + `/v1/models`, streaming/non-streaming, LiteLLM passthrough with `llm_call_*` trace.
- Demo agent live: SSE echo for streaming validation.
- Data ready: SQLite tables for sessions/runs/events/messages/tool_calls/approvals; healthcheck/config wired.
- Gap: ingress WebSocket adapter + orchestrator bridge not built.

## Next Execution Plan
1. Build ingress WebSocket handler: connection registry; validate `hello/agent_invoke/tool_result/approval_decision/cancel_run` (per docs §5).
2. Wire `/internal/send` to fan out `run_started/delta/state/tool_request/approval_required/done/error` to sessions.
3. End-to-end test: Client → Ingress → Orchestrator → Agent (SSE) → Ingress → Client; ensure events persisted/replayable.
4. Close client tool + approval loops (docs §4.4/4.5) so runs resume from waiting states.
5. Hardening: ingress healthcheck/logging, traceparent+run_id correlation, timeouts/backoff; optional static API key for M0.
