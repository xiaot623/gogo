# 多 Agent 平台（MVP）架构设计

## 1. 背景与目标

本项目的核心目的：**解耦 Agent 的实现（算法侧外部服务）与工程链路（平台侧治理/转发/可观测/审批）**。

- Agent 以“外部 HTTP 服务”的形态存在（通常 Python 实现），通过实现平台定义的最小接入协议接入。
- Agent 在开发时“以为自己在直连 LLM/Tool/其他 Agent”，实际统一通过平台代理转发，从而实现全链路 trace、回放、审计与审批。
- 用户与平台的交互在 MVP 阶段统一走 **WebSocket**（复用类似 IM 的双向通信体验），不做性能优化。

### MVP 范围（In-scope）
- 用户 ↔ 平台：WebSocket 双向交互（发起 run、接收流式输出、接收 tool/审批节点、提交 tool_result、提交审批决策）。
- 平台 ↔ Agent：HTTP 调用 `POST /invoke`，Agent 用 **SSE** 流式返回。
- Agent ↔ 平台：
  - LLM 走平台的 OpenAI 兼容代理（`/v1/chat/completions`，支持 streaming）。
  - Tool 走平台 tool 代理（server tool 由平台执行；client tool 由平台下发给用户客户端执行）。
  - Agent-to-Agent 走平台转发，形成 parent/child 调用链。
- Trace：会话级链路追踪 + 回放（记录用户输入、agent 调用、LLM 调用、tool 调用、审批节点）。
- 审批：作为工作流阻塞节点（`WAITING_APPROVAL`），审批通过后平台自动继续执行该 tool 节点。

### MVP 非目标（Out-of-scope）
- 多租户/用户隔离、AB 实验、生态市场、合规治理（保留接口/扩展点即可）。
- 针对 IM 网关的高性能优化（先用原生 WebSocket server 实现）。

## 2. 核心概念与实体

- `Agent`：外部服务（HTTP endpoint），由算法侧实现。
- `Session`：一次对话/任务会话（用户维度）。
- `Run`：用户调用某个 root agent 的一次执行。
- `Tool`：可调用工具，分为：
  - `server_tool`：平台直接执行（HTTP/内部服务）。
  - `client_tool`：平台下发给用户客户端执行（WebSocket）。
- `ToolCall`：一次具体工具调用（工作流节点）。
- `Approval`：一次审批任务，与某个 ToolCall 绑定。
- `Event`：用于 trace/回放的事件流记录（append-only）。

## 3. 总体架构（组件与职责）

建议 MVP 以 Docker Compose 形态启动以下服务：

- `ws-gateway`
  - 用户 WebSocket 入口
  - 创建/管理 Session、Run
  - 转发调用外部 Agent（HTTP + SSE）并将 SSE 转换为 WS 事件
  - 记录事件流（trace）

- `agent-registry`（MVP 可与 `ws-gateway` 合并）
  - Agent 动态注册/心跳/发现
  - Agent-to-Agent 调用代理（平台转发调用 agentB）

- `tool-proxy`
  - Tool registry（tool_name → kind/策略/超时等）
  - Policy 决策与审批流程（allow/require_approval/block）
  - server tool 执行
  - client tool 下发与回收

- `llm-proxy`
  - OpenAI 兼容接口（`/v1/chat/completions`）
  - 透传到外部多模型路由系统
  - 记录 LLM 调用事件

- `postgres`
  - 持久化 sessions/runs/tool_calls/approvals/events

> 说明：MVP 为降低复杂度，可以将 `agent-registry/tool-proxy/llm-proxy` 合并为单进程服务，但建议在架构上先分出清晰边界。

## 4. 关键链路时序

### 4.1 用户调用 Agent（WS → 平台 → Agent SSE → WS）
1) 客户端与 `ws-gateway` 建立 WS 连接并发送 `hello` 鉴权。
2) 客户端发送 `agent_invoke`，包含 `agent_id/session_id/messages[]`。
3) `ws-gateway` 创建 `run_id`，写入事件 `run_started`。
4) `ws-gateway` 调用外部 agent：`POST {agent.endpoint}/invoke`（携带 `run_id/session_id/traceparent`）。
5) agent SSE 返回 `delta/state/done` 事件，`ws-gateway` 转成 WS 事件推送给客户端，并写入 `events`。

### 4.2 Agent 调用 LLM（Agent → llm-proxy → 外部路由系统）
- agent 使用 OpenAI 兼容 SDK，将 `base_url` 指向 `llm-proxy`。
- `llm-proxy` 将请求透传到外部多模型路由系统，并记录 `llm_call_*` 事件（含 latency、token、错误等）。

### 4.3 Agent 调用 Tool（阻塞节点）
- agent 调用 `tool-proxy`：`POST /v1/tools/{tool_name}:invoke`。
- `tool-proxy` 对该 tool_call 执行 policy：
  - `allow`：执行 tool（server 或 client）。
  - `require_approval`：创建 approval，使 run 进入 `WAITING_APPROVAL`。
  - `block`：直接失败并记录。

### 4.4 Client Tool 下发与回收（tool-proxy ↔ 客户端 WS）
- `tool-proxy` 通过 `ws-gateway`（或共享 WS channel）向客户端推送 `tool_request`。
- 客户端执行后回传 `tool_result`。
- `tool-proxy` 将结果返回给 agent（见 §6 的 tool invoke 语义），并记录事件。

### 4.5 审批节点（WAITING_APPROVAL → approve → 继续执行）
- 当 policy 决策为 `require_approval`：
  - 创建 `approval_id`，tool_call 状态置为 `WAITING_APPROVAL`。
  - 平台推送 `approval_required` 给客户端。
- 客户端回传 `approval_decision`：approve/reject。
- approve：平台继续执行该 tool_call（dispatch），成功后 run 才能继续推进。

## 5. WebSocket 协议（用户 ↔ 平台）

### 5.1 通用字段
所有消息建议包含：
- `type`：消息类型
- `ts`：毫秒时间戳
- `request_id`：请求侧生成（用于幂等/关联）
- `session_id`：会话 ID
- `run_id`：执行 ID（平台生成）

### 5.2 客户端 → 平台

- `hello`
```json
{"type":"hello","ts":0,"user_id":"u1","api_key":"...","client_meta":{"app":"web"}}
```

- `agent_invoke`
```json
{"type":"agent_invoke","ts":0,"request_id":"r1","session_id":"s1","agent_id":"agentA","messages":[{"role":"user","content":"hi"}]}
```

- `tool_result`
```json
{"type":"tool_result","ts":0,"run_id":"run1","tool_call_id":"tc1","ok":true,"result":{"x":1}}
```

- `approval_decision`
```json
{"type":"approval_decision","ts":0,"run_id":"run1","approval_id":"ap1","decision":"approve","reason":"ok"}
```

- `cancel_run`
```json
{"type":"cancel_run","ts":0,"run_id":"run1"}
```

### 5.3 平台 → 客户端

- `run_started`
```json
{"type":"run_started","ts":0,"request_id":"r1","run_id":"run1","session_id":"s1","agent_id":"agentA"}
```

- `delta`
```json
{"type":"delta","ts":0,"run_id":"run1","text":"hello"}
```

- `state`
```json
{"type":"state","ts":0,"run_id":"run1","state":"WAITING_APPROVAL","detail":{"approval_id":"ap1"}}
```

- `tool_request`（client tool 下发）
```json
{"type":"tool_request","ts":0,"run_id":"run1","tool_call_id":"tc1","tool_name":"browser.open","args":{"url":"https://..."},"deadline_ts":0}
```

- `approval_required`
```json
{"type":"approval_required","ts":0,"run_id":"run1","approval_id":"ap1","tool_call_id":"tc1","tool_name":"payments.transfer","args_summary":"transfer $10 to ..."}
```

- `error` / `done`
```json
{"type":"error","ts":0,"run_id":"run1","code":"client_offline","message":"no active client"}
```
```json
{"type":"done","ts":0,"run_id":"run1","usage":{"tokens":123}}
```

## 6. Agent 接入协议（平台 → Agent，HTTP + SSE）

### 6.1 HTTP 调用
- `POST /invoke`
  - Headers：
    - `traceparent`（W3C）
    - `x-session-id`
    - `x-run-id`
  - Body（建议）：
```json
{"agent_id":"agentA","session_id":"s1","run_id":"run1","messages":[{"role":"user","content":"hi"}],"context":{"user_id":"u1"}}
```

### 6.2 SSE 事件
SSE 建议采用 JSON data，便于平台统一写 events：
- `event: delta` data: `{ "text": "...", "run_id":"..." }`
- `event: state` data: `{ "state":"...", "detail":{...} }`
- `event: done` data: `{ "usage":{...} }`
- `event: error` data: `{ "code":"...", "message":"..." }`

### 6.3 健康检查
- `GET /health` → 200

## 7. 平台代理接口（Agent → 平台）

### 7.1 LLM Proxy（OpenAI 兼容）
- `POST /v1/chat/completions`（支持 streaming）
- `llm-proxy` 透传到外部多模型路由系统
- 平台记录：request_id、model、latency、token、错误

### 7.2 Tool Proxy（异步对象模型 + 可阻塞 wait）
为了支持审批/客户端执行的“未来完成”，Tool invoke 推荐采用异步对象模型：

- `POST /v1/tools/{tool_name}:invoke`
  - Request：`{run_id, args, idempotency_key?, timeout_ms?}`
  - Response：
    - 立即成功：`{status:"succeeded", result}`
    - 需要等待：`{status:"pending", tool_call_id}`
    - 失败：`{status:"failed", error}`

- `GET /v1/tool_calls/{tool_call_id}`
  - Response：`{status, result?, error?, state, timestamps}`

- `POST /v1/tool_calls/{tool_call_id}:wait?timeout_ms=...`
  - 用途：agent 侧 SDK 封装成“看起来同步”的 tool.invoke()

> MVP 推荐提供 Python SDK：`tool.invoke()` 内部调用 invoke + wait，算法侧几乎无感知。

### 7.3 Agent Registry / Agent-to-Agent
- `POST /v1/agents/register`
  - Request：`{agent_id,name,endpoint,capabilities,auth?}`
  - Response：`{ok:true}`

- `GET /v1/agents`

- `POST /v1/agents/{agent_id}:invoke`（平台转发给 agentB）

## 8. 状态机设计

### 8.1 ToolCall 状态机（工作流阻塞节点）
- `CREATED`
- `POLICY_CHECKED`
  - `BLOCKED`
  - `WAITING_APPROVAL`
  - `DISPATCHED`
- server tool：`RUNNING` → `SUCCEEDED|FAILED|TIMEOUT`
- client tool：`WAITING_CLIENT` → `SUCCEEDED|FAILED|TIMEOUT`

推进规则：**只有 `SUCCEEDED` 才允许 run 从该节点继续**。

### 8.2 Approval 状态机
- `PENDING` → `APPROVED|REJECTED|EXPIRED`

### 8.3 Run 状态机
- `RUNNING`
- `PAUSED_WAITING_APPROVAL`
- `PAUSED_WAITING_TOOL`
- `DONE|FAILED|CANCELLED`

## 9. Trace 与回放

### 9.1 事件记录（events 表）
事件采用 append-only，最小必存：
- `user_input`
- `run_started/run_done/run_failed`
- `agent_invoke_started/agent_stream_delta/agent_invoke_done`
- `llm_call_started/llm_call_done`
- `tool_call_created/policy_decision/tool_dispatched/tool_result`
- `approval_created/approval_decision`
- `agent_called_child`（A→B）

### 9.2 回放
- `GET /v1/runs/{run_id}/events` 返回时间序列
- 回放客户端按事件序列重建 UI（或平台直接重放为 WS 事件）

## 10. 可靠性与默认策略（MVP）

- 鉴权：WS `hello` 使用静态 `api_key`（MVP）
- client tool 离线：立即失败 `client_offline`
- 超时：tool 默认 60s，审批默认 10min（可配置）
- 幂等：tool invoke 支持 `idempotency_key`

## 11. 数据模型（Postgres，建议 JSONB）

- `sessions(session_id, user_id, created_at, metadata_jsonb)`
- `runs(run_id, session_id, root_agent_id, status, started_at, ended_at, error_jsonb)`
- `events(event_id, run_id, ts, type, payload_jsonb)`
- `agents(agent_id, name, endpoint, capabilities_jsonb, status, last_heartbeat_at)`
- `tools(tool_name, kind, policy_jsonb, timeout_ms, metadata_jsonb)`
- `tool_calls(tool_call_id, run_id, tool_name, kind, status, args_jsonb, result_jsonb, error_jsonb, approval_id)`
- `approvals(approval_id, run_id, tool_call_id, status, created_at, decided_at, decided_by, reason)`

## 12. 实施里程碑（建议顺序）

- M0：WS 调用与 agent SSE 透传（`agent_invoke → delta → done`），events 落库可回放
- M1：agent 动态注册/发现（`agents/register`, `GET /agents`）
- M2：OpenAI 兼容 `llm-proxy`（含 streaming）+ LLM 事件
- M3：server tool（同步）+ tool_call 状态机 + 事件
- M4：client tool（WS 下发/回收）+ 超时/离线处理
- M5：审批节点（policy → approval_required → decision → 继续执行 tool）
- M6：基础监控指标（run/tool 成功率、p95、等待审批时长）
