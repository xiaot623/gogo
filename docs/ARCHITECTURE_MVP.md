# 多 Agent 平台（MVP）架构设计

## 1. 背景与目标

本项目的核心目的：**解耦 Agent 的实现（算法侧外部服务）与工程链路（平台侧治理/转发/可观测/审批）**。

- Agent 以“外部 HTTP 服务”的形态存在（通常 Python 实现），通过实现平台定义的最小接入协议接入。
- Agent 在开发时“以为自己在直连 LLM/Tool/其他 Agent”，实际统一通过平台代理转发，从而实现全链路 trace、回放、审计与审批。
- 用户与平台的交互由 **IM 系统承载**（双向通信体验）。客户端保持极简：只发送“单条用户输入”，渲染服务端推送的事件；会话消息与执行事件由服务端维护（客户端可做本地缓存）。MVP 阶段为了便于落地，可先用原生 **WebSocket** 实现 IM 链路的最小子集。

### MVP 范围（In-scope）

- 用户 ↔ 平台：由 IM 承载双向交互（MVP 可用 WebSocket 实现），客户端只发送单条输入消息；服务端负责会话消息存储与事件推送，支持：发起 run、接收流式输出、接收 tool/审批节点、提交 tool_result、提交审批决策。
- 平台 ↔ Agent：HTTP 调用 `POST /invoke`，Agent 用 **SSE** 流式返回。
- Agent ↔ 平台：
  - LLM 路由：平台提供 OpenAI 兼容代理（`/v1/chat/completions`，支持 streaming），透传到外部多模型路由系统。
  - Tool 路由：平台提供 tool invoke 代理（server tool 由平台执行；client tool 由平台通过 IM 下发给用户客户端执行）。
  - Agent 路由：Agent-to-Agent 必须走平台转发，形成 parent/child 调用链。
- Trace：会话级链路追踪 + 回放（记录用户输入、agent 调用、LLM 调用、tool 调用、审批节点）。
- 审批：作为工作流阻塞节点（`WAITING_APPROVAL`），审批通过后平台自动继续执行该 tool 节点。

### MVP 非目标（Out-of-scope）

- 多租户/用户隔离、AB 实验、生态市场、合规治理（保留接口/扩展点即可）。
- IM 链路侧的高性能优化（MVP 先用原生 WebSocket 复刻最小能力）。

## 2. 核心概念与实体

- `Agent`：外部服务（HTTP endpoint），由算法侧实现。
- `Session`：一次对话/任务会话（用户维度）。
- `Run`：用户调用某个 root agent 的一次执行。
- `Tool`：可调用工具，分为：
  - `server_tool`：平台直接执行（HTTP/内部服务）。
  - `client_tool`：平台下发给用户客户端执行（通过 `ingress` 对接的外部通道承载）。
- `ToolCall`：一次具体工具调用（工作流节点）。
- `Approval`：一次审批任务，与某个 ToolCall 绑定。
- `Event`：用于 trace/回放的事件流记录（append-only）。

## 3. 总体架构（组件与职责）

### 3.1 MVP 形态：协调者 + 统一接入（Ingress）

为降低部署与心智成本，MVP 推荐采用两类逻辑组件（可同进程部署，但边界要清晰）：

- **协调者服务（`orchestrator`）**：纯业务核心，负责三类路由与状态机推进

  - 跨 agent 调用路由（Agent-to-Agent）
  - 工具调用路由（server/client tool）
  - 大模型调用路由（OpenAI 兼容代理 → 外部多模型路由系统）
- **统一接入服务（`ingress`）**：对接外部通信与触发源，负责“把外部消息/任务转成 orchestrator 可执行的命令”

  - 可以对接 IM、WebSocket、HTTP Webhook、内部事件总线等
  - MVP 阶段为了便于落地，可先用 WebSocket 实现最小接入能力

> 关键边界：`ingress` 负责接入与投递；`orchestrator` 负责路由与执行推进。`orchestrator` 不承担连接管理、推送 ACK、重连、离线补偿等通信职责。

建议 MVP 以 Docker Compose 形态启动以下服务：

- `orchestrator`

  - Run/ToolCall/Approval 状态机与推进（工作流语义）
  - Agent registry：动态注册/心跳/发现
  - Agent invoke：平台 → 外部 Agent（HTTP + SSE）
  - Agent-to-Agent：平台转发调用 agentB，并记录 parent/child 调用关系
  - LLM route：OpenAI 兼容接口（`/v1/chat/completions`），透传到外部多模型路由系统
  - Tool route：
    - tool registry（tool_name → kind/策略/超时等）
    - policy 决策与审批流程（allow/require_approval/block）
    - server tool 执行
    - client tool 下发/回收通过 `ingress` 投递到终端
  - Trace：统一记录事件流（events），支持回放
- `ingress`（MVP 可用 WebSocket 实现）

  - 将外部消息转换为 orchestrator 命令（invoke/approval/tool_result/cancel）
  - 将 orchestrator 产生的事件（delta/state/tool_request/approval_required/done）投递回外部通道
- `postgres`

  - 持久化 sessions/runs/tool_calls/approvals/events

### 3.2 后续演进：同步/异步双链路（非 MVP）

未来可在 `ingress` 扩展异步处理链路，用于“实时性要求不高”的任务：

- 同步链路（高优）

  - 外部消息 → `ingress` → `orchestrator`（同步调用）→ 事件回推
- 异步链路（填谷）

  - 外部消息 → `ingress` → 投递 MQ（`task_enqueued`）
  - `ingress` 或独立 worker 从 MQ 消费任务 → 调用 `orchestrator`
  - `orchestrator` 执行推进并写 events → `ingress` 订阅/拉取并回推结果

同步/异步切换可通过“业务规则/算法插件”决定（例如按 agent_id、消息标签、成本预算、预计耗时、用户配置）。

## 4. 关键链路时序

### 4.1 用户调用 Agent（外部通道 → ingress → orchestrator → Agent SSE → ingress → 外部通道）

1) 外部通道（例如 IM/WebSocket）将 `agent_invoke` 投递给 `ingress`（含 `agent_id/session_id/message` 单条输入）。
2) `ingress` 将请求转换为 orchestrator 命令并调用 `orchestrator`（同步链路）。
3) `orchestrator` 创建 `run_id`，写入事件 `run_started`。
4) `orchestrator` 调用外部 agent：`POST {agent.endpoint}/invoke`（携带 `run_id/session_id/traceparent`）。
5) agent SSE 返回 `delta/state/done` 事件，`orchestrator` 记录 events，并将事件交给 `ingress` 投递回外部通道。

### 4.2 Agent 调用 LLM（Agent → orchestrator(OpenAI proxy) → 外部路由系统）

- agent 使用 OpenAI 兼容 SDK，将 `base_url` 指向 `orchestrator`。
- `orchestrator` 将请求透传到外部多模型路由系统，并记录 `llm_call_*` 事件（含 latency、token、错误等）。

### 4.3 Agent 调用 Tool（阻塞节点）

- agent 调用 `orchestrator`：`POST /v1/tools/{tool_name}:invoke`。
- `orchestrator` 对该 tool_call 执行 policy：
  - `allow`：执行 tool（server 或 client）。
  - `require_approval`：创建 approval，使 run 进入 `WAITING_APPROVAL`。
  - `block`：直接失败并记录。

### 4.4 Client Tool 下发与回收（orchestrator ↔ ingress ↔ 外部通道）

- `orchestrator` 产生 `tool_request` 事件并交给 `ingress`。
- `ingress` 将 `tool_request` 投递到外部通道（例如 IM/WebSocket）并到达终端。
- 终端执行后将 `tool_result` 回传到 `ingress`，`ingress` 再提交给 `orchestrator`。
- `orchestrator` 将结果返回给 agent（见 §7 的 tool invoke 语义），并记录事件。

### 4.5 审批节点（WAITING_APPROVAL → approve → 继续执行）

- 当 policy 决策为 `require_approval`：
  - 创建 `approval_id`，tool_call 状态置为 `WAITING_APPROVAL`。
  - 平台推送 `approval_required` 给客户端。
- 客户端回传 `approval_decision`：approve/reject。
- approve：平台继续执行该 tool_call（dispatch），成功后 run 才能继续推进。

## 5. 接入层消息协议（外部通道 ↔ 平台）

本节定义 `ingress` 与外部通道之间承载的“业务消息载荷（payload）”。外部通道可以是 IM、WebSocket、HTTP Webhook、MQ 消息等。

本协议只描述业务层消息；具体的连接管理、投递、ACK、重连、离线补偿等通信问题由 `ingress` 负责，`orchestrator` 不承担。

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

客户端仅发送本次输入的单条消息（历史消息由服务端存储维护；agent 侧如需完整上下文，可通过平台接口拉取 transcript）。

```json
{"type":"agent_invoke","ts":0,"request_id":"r1","session_id":"s1","agent_id":"agentA","message":{"role":"user","content":"hi"}}
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
    - 平台可选择向 agent 传递：
      - `input_message`：本次用户输入（必选）
      - `messages[]`：完整 transcript（可选；由平台从服务端存储拼装）
    - 这允许客户端保持极简，而 agent 仍可拿到所需上下文。

```json
{
  "agent_id":"agentA",
  "session_id":"s1",
  "run_id":"run1",
  "input_message":{"role":"user","content":"hi"},
  "messages":[{"role":"system","content":"..."},{"role":"user","content":"..."}],
  "context":{"user_id":"u1"}
}
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

> 说明：以下接口由 `orchestrator` 对外提供，agent 侧通过 SDK/HTTP 访问；对用户侧的推送与交互由 `ingress` 通过外部通道承载。

### 7.1 LLM Proxy（OpenAI 兼容）

- `POST /v1/chat/completions`（支持 streaming）
- 由 `orchestrator` 透传到外部多模型路由系统
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

### 7.4 Session Transcript（给 agent 拉取上下文）

平台不负责“上下文管理”，但需要提供服务端消息存储与读取能力，用于：
- 客户端极简（只上报单条输入）
- agent 自主决定是否需要历史消息/如何组织上下文

建议提供：
- `GET /v1/sessions/{session_id}/messages?limit=...&before=...`
  - 返回服务端保存的 transcript（messages[]）
- `POST /v1/sessions/{session_id}/messages`
  - 允许 `ingress` 或 `orchestrator` 追加一条消息（例如用户输入、最终 assistant 输出摘要）


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
- 回放客户端按事件序列重建 UI（或平台通过 `ingress` 重放为外部通道事件）

## 10. 可靠性与默认策略（MVP）

- 鉴权：外部通道到 `ingress` 的 `hello` 使用静态 `api_key`（MVP）
- client tool 离线：立即失败 `client_offline`
- 超时：tool 默认 60s，审批默认 10min（可配置）
- 幂等：tool invoke 支持 `idempotency_key`

## 11. 平台运行数据与上下文存储

### 11.1 平台不管理 Agent 上下文（仅提供存储封装）

结论定稿：**平台不负责管理/加工 agent 的上下文，只提供“上下文存储能力（storage）”**。

- 平台必须持久化的平台运行数据（不等同于 agent 上下文）

  - Run/ToolCall/Approval 的状态机状态与推进点（保证“工具/审批阻塞节点”可恢复、可推进）
  - Trace/回放事件流（events）：用户输入、agent 调用、LLM 调用、tool 调用、审批节点
  - 这些属于平台自身能力的数据面，必须落库，否则无法回放、也难以保证执行可靠性
- 平台不做的事（明确不在边界内）

  - 不做 memory 选择、压缩、总结、检索、对齐等“智能上下文管理”
  - 不托管 agent 的长期记忆/向量库/私有状态（由 agent 自己决定形态与存储）
- 平台提供的能力（仅存储封装，agent 自愿使用）

  - `context store`（KV/Blob/版本化对象）：agent 可将任意上下文片段写入并获取 `context_key`
  - 平台事件中仅记录 `context_key` 或摘要引用，避免把 agent 私有状态强绑定到平台数据模型

### 11.2 数据模型（Postgres，建议 JSONB）

- `sessions(session_id, user_id, created_at, metadata_jsonb)`
- `runs(run_id, session_id, root_agent_id, status, started_at, ended_at, error_jsonb)`
- `events(event_id, run_id, ts, type, payload_jsonb)`
- `agents(agent_id, name, endpoint, capabilities_jsonb, status, last_heartbeat_at)`
- `tools(tool_name, kind, policy_jsonb, timeout_ms, metadata_jsonb)`
- `tool_calls(tool_call_id, run_id, tool_name, kind, status, args_jsonb, result_jsonb, error_jsonb, approval_id)`
- `approvals(approval_id, run_id, tool_call_id, status, created_at, decided_at, decided_by, reason)`

（可选）`context_blobs(context_key, owner_type, owner_id, version, data_jsonb, created_at)`：给 agent 自愿存取的通用上下文存储（仅存储，不做管理/加工）

## 12. 实施里程碑（建议顺序）

- M0：IM 调用与 agent SSE 透传（`agent_invoke → delta → done`），events 落库可回放
- M1：agent 动态注册/发现（`agents/register`, `GET /agents`）
- M2：OpenAI 兼容（含 streaming）+ LLM 事件
- M3：server tool（同步）+ tool_call 状态机 + 事件
- M4：client tool（IM 下发/回收）+ 超时/离线处理
- M5：审批节点（policy → approval_required → decision → 继续执行 tool）
- M6：基础监控指标（run/tool 成功率、p95、等待审批时长）

> 说明：以上里程碑默认实现于单体 `orchestrator` 内部模块；后续如需扩展，再按 §3.2 进行服务拆分。
