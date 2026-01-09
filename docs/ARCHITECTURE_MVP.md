# 多 Agent 平台（MVP）架构设计

> **版本**: 1.0
> **更新日期**: 2024-01
> **状态**: Draft

---

## 目录

- [1. 背景与目标](#1-背景与目标)
  - [1.1 MVP 范围（In-scope）](#11-mvp-范围in-scope)
  - [1.2 MVP 非目标（Out-of-scope）](#12-mvp-非目标out-of-scope)
- [2. 核心概念与实体](#2-核心概念与实体)
- [3. 总体架构](#3-总体架构组件与职责)
  - [3.1 MVP 形态](#31-mvp-形态协调者--统一接入ingress)
  - [3.2 架构图](#32-架构图)
  - [3.3 后续演进](#33-后续演进同步异步双链路非-mvp)
- [4. 关键链路时序](#4-关键链路时序)
- [5. 接入层消息协议](#5-接入层消息协议外部通道--平台)
- [6. Agent 接入协议](#6-agent-接入协议平台--agenthttps--sse)
- [7. 平台代理接口](#7-平台代理接口agent--平台)
- [8. 状态机设计](#8-状态机设计)
- [9. Trace 与回放](#9-trace-与回放)
- [10. 错误处理与重试策略](#10-错误处理与重试策略)
- [11. 可靠性与默认策略](#11-可靠性与默认策略mvp)
- [12. 平台运行数据与上下文存储](#12-平台运行数据与上下文存储)
- [13. 部署配置](#13-部署配置)
- [14. 实施里程碑](#14-实施里程碑建议顺序)

---

## 1. 背景与目标

本项目的核心目的：**解耦 Agent 的实现（算法侧外部服务）与工程链路（平台侧治理/转发/可观测/审批）**。

- Agent 以"外部 HTTP 服务"的形态存在（通常 Python 实现），通过实现平台定义的最小接入协议接入。
- Agent 在开发时"以为自己在直连 LLM/Tool/其他 Agent"，实际统一通过平台代理转发，从而实现全链路 trace、回放、审计与审批。
- 用户与平台的交互由 **IM 系统承载**（双向通信体验）。客户端保持极简：只发送"单条用户输入"，渲染服务端推送的事件；会话消息与执行事件由服务端维护（客户端可做本地缓存）。MVP 阶段为了便于落地，可先用原生 **WebSocket** 实现 IM 链路的最小子集。

### 1.1 MVP 范围（In-scope）

| 链路           | 描述                                                                                                      |
| -------------- | --------------------------------------------------------------------------------------------------------- |
| 用户 ↔ 平台   | 由 IM 承载双向交互（MVP 可用 WebSocket 实现），客户端只发送单条输入消息；服务端负责会话消息存储与事件推送 |
| 平台 ↔ Agent  | HTTP 调用 `POST /invoke`，Agent 用 **SSE** 流式返回                                               |
| Agent → LLM   | 平台提供 OpenAI 兼容代理（`/v1/chat/completions`，支持 streaming），透传到外部多模型路由系统            |
| Agent → Tool  | 平台提供 tool invoke 代理（server tool 由平台执行；client tool 由平台通过 IM 下发给用户客户端执行）       |
| Agent → Agent | Agent-to-Agent 必须走平台转发，形成 parent/child 调用链                                                   |
| Trace          | 会话级链路追踪 + 回放（记录用户输入、agent 调用、LLM 调用、tool 调用、审批节点）                          |
| 审批           | 作为工作流阻塞节点（`WAITING_APPROVAL`），审批通过后平台自动继续执行该 tool 节点                        |

**支持的客户端操作**：

- 发起 run
- 接收流式输出
- 接收 tool/审批节点
- 提交 tool_result
- 提交审批决策

### 1.2 MVP 非目标（Out-of-scope）

- 多租户/用户隔离、AB 实验、生态市场、合规治理（保留接口/扩展点即可）
- IM 链路侧的高性能优化（MVP 先用原生 WebSocket 复刻最小能力）

---

## 2. 核心概念与实体

| 实体         | 说明                                                                                   |
| ------------ | -------------------------------------------------------------------------------------- |
| `Agent`    | 外部服务（HTTP endpoint），由算法侧实现                                                |
| `Session`  | 一次对话/任务会话（用户维度）                                                          |
| `Run`      | 用户调用某个 root agent 的一次执行                                                     |
| `Tool`     | 可调用工具，分为 `server_tool`（平台直接执行）和 `client_tool`（下发给客户端执行） |
| `ToolCall` | 一次具体工具调用（工作流节点）                                                         |
| `Approval` | 一次审批任务，与某个 ToolCall 绑定                                                     |
| `Event`    | 用于 trace/回放的事件流记录（append-only）                                             |
| `Message`  | 会话中的单条消息（用户输入或 assistant 输出）                                          |

---

## 3. 总体架构（组件与职责）

### 3.1 MVP 形态：协调者 + 统一接入（Ingress）

为降低部署与心智成本，MVP 推荐采用两类逻辑组件（可同进程部署，但边界要清晰）：

**协调者服务（`orchestrator`）**：纯业务核心，负责三类路由与状态机推进

| 职责              | 说明                                  |
| ----------------- | ------------------------------------- |
| 跨 agent 调用路由 | Agent-to-Agent 转发                   |
| 工具调用路由      | server/client tool 分发               |
| 大模型调用路由    | OpenAI 兼容代理 → 外部多模型路由系统 |

**统一接入服务（`ingress`）**：对接外部通信与触发源

| 职责     | 说明                                               |
| -------- | -------------------------------------------------- |
| 协议转换 | 把外部消息/任务转成 orchestrator 可执行的命令      |
| 通道对接 | 可对接 IM、WebSocket、HTTP Webhook、内部事件总线等 |
| 连接管理 | 负责推送 ACK、重连、离线补偿等通信职责             |

> **关键边界**：`ingress` 负责接入与投递；`orchestrator` 负责路由与执行推进。`orchestrator` 不承担连接管理、推送 ACK、重连、离线补偿等通信职责。

### 3.2 架构图

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              External World                                  │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│   ┌──────────┐    ┌──────────┐    ┌──────────┐    ┌──────────────────┐     │
│   │  Client  │    │  Client  │    │  Agent   │    │  LLM Router      │     │
│   │  (Web)   │    │  (App)   │    │ Services │    │  (External)      │     │
│   └────┬─────┘    └────┬─────┘    └────┬─────┘    └────────┬─────────┘     │
│        │               │               │                   │               │
└────────┼───────────────┼───────────────┼───────────────────┼───────────────┘
         │               │               │                   │
         │  WebSocket    │  WebSocket    │  HTTP/SSE         │  HTTP
         │               │               │                   │
┌────────┼───────────────┼───────────────┼───────────────────┼───────────────┐
│        ▼               ▼               │                   │               │
│   ┌─────────────────────────┐          │                   │               │
│   │        Ingress          │          │                   │               │
│   │  ┌───────────────────┐  │          │                   │               │
│   │  │ WebSocket Handler │  │          │                   │               │
│   │  │ Protocol Adapter  │  │          │                   │               │
│   │  │ Connection Mgmt   │  │          │                   │               │
│   │  └─────────┬─────────┘  │          │                   │               │
│   └────────────┼────────────┘          │                   │               │
│                │                       │                   │               │
│                ▼                       ▼                   │               │
│   ┌──────────────────────────────────────────────────────────────────┐    │
│   │                        Orchestrator                               │    │
│   │  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐              │    │
│   │  │ Run Manager │  │ Tool Router │  │ LLM Proxy   │◄─────────────┼────┘
│   │  │             │  │             │  │             │              │
│   │  │ State Machine│  │ Policy Engine│ │ /v1/chat/  │              │
│   │  └──────┬──────┘  └──────┬──────┘  │ completions │              │
│   │         │                │         └─────────────┘              │
│   │         │                │                                       │
│   │  ┌──────┴──────┐  ┌──────┴──────┐  ┌─────────────┐              │
│   │  │ Agent       │  │ Approval    │  │ Trace/Event │              │
│   │  │ Registry    │  │ Manager     │  │ Recorder    │              │
│   │  └─────────────┘  └─────────────┘  └──────┬──────┘              │
│   └───────────────────────────────────────────┼──────────────────────┘    │
│                                               │                           │
│                                               ▼                           │
│   ┌─────────────────────────────────────────────────────────────────┐    │
│   │                         PostgreSQL                               │    │
│   │  sessions │ runs │ events │ messages │ tool_calls │ approvals   │    │
│   └─────────────────────────────────────────────────────────────────┘    │
│                                                                           │
│                              Platform                                     │
└───────────────────────────────────────────────────────────────────────────┘
```

**MVP Docker Compose 服务组成**：

| 服务             | 职责                                                                                     |
| ---------------- | ---------------------------------------------------------------------------------------- |
| `orchestrator` | Run/ToolCall/Approval 状态机、Agent registry、Agent invoke、LLM route、Tool route、Trace |
| `ingress`      | 外部消息转换、事件投递（MVP 用 WebSocket 实现）                                          |
| `postgres`     | 持久化 sessions/runs/messages/tool_calls/approvals/events                                |

### 3.3 后续演进：同步/异步双链路（非 MVP）

未来可在 `ingress` 扩展异步处理链路，用于"实时性要求不高"的任务：

```
同步链路（高优）:
  外部消息 → ingress → orchestrator（同步调用）→ 事件回推

异步链路（填谷）:
  外部消息 → ingress → MQ（task_enqueued）
                        ↓
  worker 消费 → orchestrator → events → ingress 订阅/拉取 → 回推结果
```

同步/异步切换可通过"业务规则/算法插件"决定（例如按 agent_id、消息标签、成本预算、预计耗时、用户配置）。

---

## 4. 关键链路时序

### 4.1 用户调用 Agent

```mermaid
sequenceDiagram
    participant C as Client
    participant I as Ingress
    participant O as Orchestrator
    participant A as Agent
  
    C->>I: agent_invoke (WebSocket)
    I->>O: invoke command
    O->>O: create run_id, write run_started event
    O->>A: POST /invoke (HTTP)
  
    loop SSE Stream
        A-->>O: event: delta
        O->>O: record event
        O-->>I: delta event
        I-->>C: delta (WebSocket)
    end
  
    A-->>O: event: done
    O->>O: write run_done event
    O-->>I: done event
    I-->>C: done (WebSocket)
```

**步骤说明**：

1. 外部通道（例如 IM/WebSocket）将 `agent_invoke` 投递给 `ingress`（含 `agent_id/session_id/message` 单条输入）
2. `ingress` 将请求转换为 orchestrator 命令并调用 `orchestrator`（同步链路）
3. `orchestrator` 创建 `run_id`，写入事件 `run_started`
4. `orchestrator` 调用外部 agent：`POST {agent.endpoint}/invoke`（携带 `run_id/session_id/traceparent`）
5. agent SSE 返回 `delta/state/done` 事件，`orchestrator` 记录 events，并将事件交给 `ingress` 投递回外部通道

### 4.2 Agent 调用 LLM

```mermaid
sequenceDiagram
    participant A as Agent
    participant O as Orchestrator (LLM Proxy)
    participant L as LLM Router (External)
  
    A->>O: POST /v1/chat/completions
    O->>O: record llm_call_started
    O->>L: forward request
  
    alt Streaming
        loop SSE chunks
            L-->>O: chunk
            O-->>A: chunk
        end
    else Non-streaming
        L-->>O: response
        O-->>A: response
    end
  
    O->>O: record llm_call_done (latency, tokens, error)
```

- Agent 使用 OpenAI 兼容 SDK，将 `base_url` 指向 `orchestrator`
- `orchestrator` 将请求透传到外部多模型路由系统，并记录 `llm_call_*` 事件（含 latency、token、错误等）

### 4.3 Agent 调用 Tool（阻塞节点）

```mermaid
sequenceDiagram
    participant A as Agent
    participant O as Orchestrator
    participant T as Tool (Server/Client)
  
    A->>O: POST /v1/tools/{tool_name}:invoke
    O->>O: policy check
  
    alt allow
        O->>T: execute tool
        T-->>O: result
        O-->>A: {status: "succeeded", result}
    else require_approval
        O->>O: create approval, set WAITING_APPROVAL
        O-->>A: {status: "pending", tool_call_id}
    else block
        O-->>A: {status: "failed", error: "blocked by policy"}
    end
```

### 4.4 Client Tool 下发与回收

```mermaid
sequenceDiagram
    participant A as Agent
    participant O as Orchestrator
    participant I as Ingress
    participant C as Client
  
    A->>O: POST /v1/tools/{tool_name}:invoke
    O->>O: identify as client_tool
    O->>I: tool_request event
    I->>C: tool_request (WebSocket)
  
    Note over C: Client executes tool locally
  
    C->>I: tool_result
    I->>O: submit tool_result
    O->>O: record event, update tool_call status
    O-->>A: result (via wait or callback)
```

### 4.5 审批节点

```mermaid
sequenceDiagram
    participant A as Agent
    participant O as Orchestrator
    participant I as Ingress
    participant C as Client
    participant U as User/Approver
  
    A->>O: POST /v1/tools/{tool_name}:invoke
    O->>O: policy = require_approval
    O->>O: create approval_id, status = WAITING_APPROVAL
    O->>I: approval_required event
    I->>C: approval_required (WebSocket)
    C->>U: show approval UI
  
    U->>C: approve/reject
    C->>I: approval_decision
    I->>O: submit decision
  
    alt approved
        O->>O: execute tool_call
        O-->>A: result
    else rejected
        O-->>A: {status: "failed", error: "rejected"}
    end
```

---

## 5. 接入层消息协议（外部通道 ↔ 平台）

本节定义 `ingress` 与外部通道之间承载的"业务消息载荷（payload）"。外部通道可以是 IM、WebSocket、HTTP Webhook、MQ 消息等。

> **注意**：本协议只描述业务层消息；具体的连接管理、投递、ACK、重连、离线补偿等通信问题由 `ingress` 负责，`orchestrator` 不承担。

### 5.1 通用字段

所有消息包含以下字段：

| 字段           | 类型   | 必填 | 说明                        |
| -------------- | ------ | ---- | --------------------------- |
| `type`       | string | Y    | 消息类型                    |
| `ts`         | int64  | Y    | 毫秒时间戳                  |
| `request_id` | string | N    | 请求侧生成（用于幂等/关联） |
| `session_id` | string | N    | 会话 ID                     |
| `run_id`     | string | N    | 执行 ID（平台生成）         |

### 5.2 客户端 → 平台

#### `hello` - 建立连接

```json
{
  "type": "hello",
  "ts": 1704067200000,
  "user_id": "u1",
  "api_key": "sk-xxx",
  "client_meta": {
    "app": "web",
    "version": "1.0.0"
  }
}
```

#### `agent_invoke` - 调用 Agent

客户端仅发送本次输入的单条消息（历史消息由服务端存储维护）。

```json
{
  "type": "agent_invoke",
  "ts": 1704067200000,
  "request_id": "req_abc123",
  "session_id": "sess_001",
  "agent_id": "agent_a",
  "message": {
    "role": "user",
    "content": "帮我查询今天的天气"
  }
}
```

#### `tool_result` - 提交工具执行结果

```json
{
  "type": "tool_result",
  "ts": 1704067200000,
  "run_id": "run_001",
  "tool_call_id": "tc_001",
  "ok": true,
  "result": {
    "file_path": "/tmp/screenshot.png"
  }
}
```

#### `approval_decision` - 提交审批决策

```json
{
  "type": "approval_decision",
  "ts": 1704067200000,
  "run_id": "run_001",
  "approval_id": "ap_001",
  "decision": "approve",
  "reason": "已确认转账信息正确"
}
```

| decision    | 说明     |
| ----------- | -------- |
| `approve` | 批准执行 |
| `reject`  | 拒绝执行 |

#### `cancel_run` - 取消执行

```json
{
  "type": "cancel_run",
  "ts": 1704067200000,
  "run_id": "run_001"
}
```

### 5.3 平台 → 客户端

#### `run_started` - 执行开始

```json
{
  "type": "run_started",
  "ts": 1704067200000,
  "request_id": "req_abc123",
  "run_id": "run_001",
  "session_id": "sess_001",
  "agent_id": "agent_a"
}
```

#### `delta` - 流式输出片段

```json
{
  "type": "delta",
  "ts": 1704067200000,
  "run_id": "run_001",
  "text": "今天北京的天气是"
}
```

#### `state` - 状态变更

```json
{
  "type": "state",
  "ts": 1704067200000,
  "run_id": "run_001",
  "state": "WAITING_APPROVAL",
  "detail": {
    "approval_id": "ap_001",
    "tool_call_id": "tc_001"
  }
}
```

#### `tool_request` - Client Tool 下发

```json
{
  "type": "tool_request",
  "ts": 1704067200000,
  "run_id": "run_001",
  "tool_call_id": "tc_001",
  "tool_name": "browser.screenshot",
  "args": {
    "url": "https://example.com"
  },
  "deadline_ts": 1704067260000
}
```

#### `approval_required` - 审批请求

```json
{
  "type": "approval_required",
  "ts": 1704067200000,
  "run_id": "run_001",
  "approval_id": "ap_001",
  "tool_call_id": "tc_001",
  "tool_name": "payments.transfer",
  "args_summary": "转账 ¥100 到账户 xxx"
}
```

#### `error` - 错误

```json
{
  "type": "error",
  "ts": 1704067200000,
  "run_id": "run_001",
  "code": "client_offline",
  "message": "客户端离线，无法执行 client tool"
}
```

**错误码表**：

| code                 | 说明           |
| -------------------- | -------------- |
| `client_offline`   | 客户端离线     |
| `tool_timeout`     | 工具执行超时   |
| `approval_timeout` | 审批超时       |
| `agent_error`      | Agent 执行出错 |
| `internal_error`   | 平台内部错误   |

#### `done` - 执行完成

```json
{
  "type": "done",
  "ts": 1704067200000,
  "run_id": "run_001",
  "usage": {
    "total_tokens": 1234,
    "prompt_tokens": 500,
    "completion_tokens": 734,
    "duration_ms": 3500
  }
}
```

---

## 6. Agent 接入协议（平台 → Agent，HTTP + SSE）

### 6.1 HTTP 调用

**`POST /invoke`**

Headers：

| Header                  | 说明                                                  |
| ----------------------- | ----------------------------------------------------- |
| `traceparent`         | W3C Trace Context                                     |
| `x-session-id`        | 会话 ID                                               |
| `x-run-id`            | 执行 ID                                               |
| `x-platform-base-url` | 平台代理接口基础 URL（Agent 用于调用 LLM/Tool/Agent） |

Request Body：

```json
{
  "agent_id": "agent_a",
  "session_id": "sess_001",
  "run_id": "run_001",
  "input_message": {
    "role": "user",
    "content": "帮我查询今天的天气"
  },
  "messages": [
    {"role": "system", "content": "你是一个有帮助的助手"},
    {"role": "user", "content": "你好"},
    {"role": "assistant", "content": "你好！有什么可以帮你的？"},
    {"role": "user", "content": "帮我查询今天的天气"}
  ],
  "context": {
    "user_id": "u1",
    "timezone": "Asia/Shanghai"
  }
}
```

| 字段              | 必填 | 说明                                      |
| ----------------- | ---- | ----------------------------------------- |
| `input_message` | Y    | 本次用户输入                              |
| `messages`      | N    | 完整 transcript（由平台从服务端存储拼装） |
| `context`       | N    | 上下文信息（用户信息、环境变量等）        |

### 6.2 SSE 事件

SSE 采用 JSON data 格式：

#### `delta` - 流式文本输出

```
event: delta
data: {"text": "今天", "run_id": "run_001"}

event: delta
data: {"text": "北京的天气", "run_id": "run_001"}
```

#### `state` - 状态变更

```
event: state
data: {"state": "thinking", "detail": {"step": "分析用户意图"}}
```

#### `done` - 执行完成

```
event: done
data: {"usage": {"tokens": 100}, "final_message": "今天北京晴，气温 25°C"}
```

#### `error` - 执行出错

```
event: error
data: {"code": "tool_failed", "message": "天气 API 调用失败"}
```

### 6.3 健康检查

**`GET /health`**

Response：`200 OK`

```json
{
  "status": "healthy",
  "version": "1.0.0",
  "capabilities": ["streaming", "tools"]
}
```

---

## 7. 平台代理接口（Agent → 平台）

> 以下接口由 `orchestrator` 对外提供，agent 侧通过 SDK/HTTP 访问。

### 7.1 LLM Proxy（OpenAI 兼容）

**`POST /v1/chat/completions`**

完全兼容 OpenAI API，支持 streaming。

Request：

```json
{
  "model": "gpt-4",
  "messages": [
    {"role": "user", "content": "Hello"}
  ],
  "stream": true
}
```

Headers（Agent 需携带）：

| Header       | 说明                          |
| ------------ | ----------------------------- |
| `x-run-id` | 当前执行 ID（用于关联 trace） |

平台记录：`request_id`、`model`、`latency`、`token`、`error`

### 7.2 Tool Proxy（异步对象模型）

为了支持审批/客户端执行的"未来完成"，Tool invoke 采用异步对象模型。

#### `POST /v1/tools/{tool_name}:invoke` - 调用工具

Request：

```json
{
  "run_id": "run_001",
  "args": {
    "query": "北京天气"
  },
  "idempotency_key": "idem_abc123",
  "timeout_ms": 30000
}
```

Response（立即成功）：

```json
{
  "status": "succeeded",
  "tool_call_id": "tc_001",
  "result": {
    "weather": "晴",
    "temperature": 25
  }
}
```

Response（需要等待）：

```json
{
  "status": "pending",
  "tool_call_id": "tc_001",
  "reason": "waiting_approval"
}
```

Response（失败）：

```json
{
  "status": "failed",
  "tool_call_id": "tc_001",
  "error": {
    "code": "blocked",
    "message": "该工具已被策略禁止"
  }
}
```

#### `GET /v1/tool_calls/{tool_call_id}` - 查询状态

Response：

```json
{
  "tool_call_id": "tc_001",
  "status": "succeeded",
  "result": {...},
  "timestamps": {
    "created_at": 1704067200000,
    "started_at": 1704067201000,
    "completed_at": 1704067205000
  }
}
```

#### `POST /v1/tool_calls/{tool_call_id}:wait` - 等待完成

Query params：

- `timeout_ms`：最大等待时间（默认 60000）

Response：同 `GET /v1/tool_calls/{tool_call_id}`

> **SDK 封装**：MVP 推荐提供 Python SDK，`tool.invoke()` 内部调用 invoke + wait，算法侧几乎无感知。

### 7.3 Agent Registry / Agent-to-Agent

#### `POST /v1/agents/register` - 注册 Agent

Request：

```json
{
  "agent_id": "weather_agent",
  "name": "天气查询 Agent",
  "endpoint": "http://weather-agent:8080",
  "capabilities": ["weather_query", "location_parse"],
  "auth": {
    "type": "bearer",
    "token": "xxx"
  }
}
```

Response：

```json
{
  "ok": true,
  "registered_at": 1704067200000
}
```

#### `GET /v1/agents` - 获取 Agent 列表

Response：

```json
{
  "agents": [
    {
      "agent_id": "weather_agent",
      "name": "天气查询 Agent",
      "status": "healthy",
      "last_heartbeat_at": 1704067200000
    }
  ]
}
```

#### `POST /v1/agents/{agent_id}:invoke` - 调用其他 Agent

Request：

```json
{
  "parent_run_id": "run_001",
  "input_message": {
    "role": "user",
    "content": "查询北京天气"
  }
}
```

Response：返回 SSE 流（同 Agent invoke）

### 7.4 Session Transcript

平台提供服务端消息存储与读取能力，用于：

- 客户端极简（只上报单条输入）
- Agent 自主决定是否需要历史消息/如何组织上下文

#### `GET /v1/sessions/{session_id}/messages` - 获取会话消息

Query params：

- `limit`：返回条数（默认 50）
- `before`：游标（返回此 message_id 之前的消息）

Response：

```json
{
  "messages": [
    {
      "message_id": "msg_001",
      "role": "user",
      "content": "你好",
      "created_at": 1704067200000
    },
    {
      "message_id": "msg_002",
      "role": "assistant",
      "content": "你好！有什么可以帮你的？",
      "created_at": 1704067201000
    }
  ],
  "has_more": false
}
```

#### `POST /v1/sessions/{session_id}/messages` - 追加消息

Request：

```json
{
  "role": "assistant",
  "content": "今天北京晴，气温 25°C",
  "run_id": "run_001"
}
```

---

## 8. 状态机设计

### 8.1 Run 状态机

```
                    ┌───────────────────────────────────────┐
                    │                                       │
                    ▼                                       │
┌─────────┐    ┌─────────┐    ┌──────────────────────┐     │
│ CREATED │───▶│ RUNNING │───▶│ PAUSED_WAITING_TOOL  │─────┤
└─────────┘    └────┬────┘    └──────────────────────┘     │
                    │                                       │
                    │         ┌────────────────────────┐    │
                    ├────────▶│ PAUSED_WAITING_APPROVAL│────┤
                    │         └────────────────────────┘    │
                    │                                       │
                    ▼                                       │
         ┌──────────────────┐                              │
         │   Terminal States │                              │
         │  ┌──────┐         │                              │
         │  │ DONE │         │◀─────────────────────────────┘
         │  └──────┘         │
         │  ┌────────┐       │
         │  │ FAILED │       │
         │  └────────┘       │
         │  ┌───────────┐    │
         │  │ CANCELLED │    │
         │  └───────────┘    │
         └──────────────────┘
```

| 状态                        | 说明                  | 可转移到                                          |
| --------------------------- | --------------------- | ------------------------------------------------- |
| `CREATED`                 | 初始状态              | `RUNNING`                                       |
| `RUNNING`                 | 执行中                | `PAUSED_*`, `DONE`, `FAILED`, `CANCELLED` |
| `PAUSED_WAITING_TOOL`     | 等待 client tool 执行 | `RUNNING`, `FAILED`, `CANCELLED`            |
| `PAUSED_WAITING_APPROVAL` | 等待审批              | `RUNNING`, `FAILED`, `CANCELLED`            |
| `DONE`                    | 成功完成（终态）      | -                                                 |
| `FAILED`                  | 失败（终态）          | -                                                 |
| `CANCELLED`               | 用户取消（终态）      | -                                                 |

### 8.2 ToolCall 状态机

```
┌─────────┐
│ CREATED │
└────┬────┘
     │
     ▼
┌────────────────┐
│ POLICY_CHECKED │
└───────┬────────┘
        │
   ┌────┼────────────────┐
   │    │                │
   ▼    ▼                ▼
┌───────┐ ┌────────────────┐ ┌────────────┐
│BLOCKED│ │WAITING_APPROVAL│ │ DISPATCHED │
└───────┘ └───────┬────────┘ └─────┬──────┘
    │             │                │
    │        ┌────┴────┐      ┌────┴────┐
    │        ▼         ▼      ▼         │
    │   ┌────────┐ ┌────────┐          │
    │   │APPROVED│ │REJECTED│          │
    │   └───┬────┘ └────┬───┘          │
    │       │           │              │
    │       ▼           │              │
    │  ┌─────────┐      │              │
    │  │ RUNNING │◀─────┼──────────────┘
    │  └────┬────┘      │
    │       │           │
    │  ┌────┼────┐      │
    │  ▼    ▼    ▼      ▼
    │┌─────────┐ ┌────────┐ ┌─────────┐
    ││SUCCEEDED│ │ FAILED │ │ TIMEOUT │
    │└─────────┘ └────────┘ └─────────┘
    │     │          ▲          ▲
    │     │          │          │
    └─────┴──────────┴──────────┘
```

| 状态                 | 说明                                                    |
| -------------------- | ------------------------------------------------------- |
| `CREATED`          | 初始状态                                                |
| `POLICY_CHECKED`   | 策略检查完成                                            |
| `BLOCKED`          | 被策略阻止（终态）                                      |
| `WAITING_APPROVAL` | 等待审批                                                |
| `APPROVED`         | 审批通过                                                |
| `REJECTED`         | 审批拒绝（终态）                                        |
| `DISPATCHED`       | 已分发执行                                              |
| `RUNNING`          | 执行中（server tool）/`WAITING_CLIENT`（client tool） |
| `SUCCEEDED`        | 成功（终态）                                            |
| `FAILED`           | 失败（终态）                                            |
| `TIMEOUT`          | 超时（终态）                                            |

> **推进规则**：只有 `SUCCEEDED` 才允许 run 从该节点继续。

### 8.3 Approval 状态机

```
┌─────────┐
│ PENDING │
└────┬────┘
     │
┌────┼────────────┐
▼    ▼            ▼
┌────────┐ ┌────────┐ ┌─────────┐
│APPROVED│ │REJECTED│ │ EXPIRED │
└────────┘ └────────┘ └─────────┘
```

| 状态         | 说明           |
| ------------ | -------------- |
| `PENDING`  | 等待审批       |
| `APPROVED` | 已批准（终态） |
| `REJECTED` | 已拒绝（终态） |
| `EXPIRED`  | 已过期（终态） |

---

## 9. Trace 与回放

### 9.1 事件类型（events 表）

事件采用 append-only，最小必存事件：

| 类别     | 事件类型                 | 说明                                     |
| -------- | ------------------------ | ---------------------------------------- |
| 用户     | `user_input`           | 用户输入消息                             |
| Run      | `run_started`          | 执行开始                                 |
| Run      | `run_done`             | 执行成功完成                             |
| Run      | `run_failed`           | 执行失败                                 |
| Run      | `run_cancelled`        | 执行取消                                 |
| Agent    | `agent_invoke_started` | Agent 调用开始                           |
| Agent    | `agent_stream_delta`   | Agent 流式输出片段                       |
| Agent    | `agent_invoke_done`    | Agent 调用完成                           |
| Agent    | `agent_called_child`   | Agent 调用子 Agent（A→B）               |
| LLM      | `llm_call_started`     | LLM 调用开始                             |
| LLM      | `llm_call_done`        | LLM 调用完成（含 latency/token/error）   |
| Tool     | `tool_call_created`    | 工具调用创建                             |
| Tool     | `policy_decision`      | 策略决策（allow/require_approval/block） |
| Tool     | `tool_dispatched`      | 工具已分发执行                           |
| Tool     | `tool_result`          | 工具执行结果                             |
| Approval | `approval_created`     | 审批任务创建                             |
| Approval | `approval_decision`    | 审批决策（approve/reject）               |

### 9.2 事件结构

```json
{
  "event_id": "evt_001",
  "run_id": "run_001",
  "ts": 1704067200000,
  "type": "llm_call_done",
  "payload": {
    "request_id": "llm_req_001",
    "model": "gpt-4",
    "latency_ms": 1500,
    "prompt_tokens": 100,
    "completion_tokens": 50,
    "error": null
  }
}
```

### 9.3 回放 API

**`GET /v1/runs/{run_id}/events`**

Query params：

- `after_ts`：返回此时间戳之后的事件
- `types`：事件类型过滤（逗号分隔）
- `limit`：返回条数（默认 100）

Response：

```json
{
  "events": [
    {"event_id": "evt_001", "ts": 1704067200000, "type": "run_started", ...},
    {"event_id": "evt_002", "ts": 1704067201000, "type": "llm_call_started", ...}
  ],
  "has_more": true,
  "next_cursor": "evt_003"
}
```

回放客户端按事件序列重建 UI（或平台通过 `ingress` 重放为外部通道事件）。

---

## 10. 错误处理与重试策略

### 10.1 错误分类

| 类型       | 错误码               | 是否可重试 | 处理策略                 |
| ---------- | -------------------- | ---------- | ------------------------ |
| 客户端错误 | `client_offline`   | N          | 立即失败，通知上游       |
| 客户端错误 | `invalid_request`  | N          | 返回 400，不重试         |
| 超时       | `tool_timeout`     | Y          | 按配置重试或失败         |
| 超时       | `approval_timeout` | N          | 标记过期，通知上游       |
| 服务端错误 | `agent_error`      | Y          | 指数退避重试             |
| 服务端错误 | `llm_error`        | Y          | 指数退避重试，可切换模型 |
| 服务端错误 | `internal_error`   | Y          | 指数退避重试             |

### 10.2 重试策略

```python
# 默认重试配置
RETRY_CONFIG = {
    "max_retries": 3,
    "initial_delay_ms": 1000,
    "max_delay_ms": 30000,
    "multiplier": 2.0,
    "jitter": 0.1
}
```

**指数退避公式**：

```
delay = min(initial_delay * (multiplier ^ attempt), max_delay) * (1 + random(-jitter, jitter))
```

### 10.3 幂等性

- Tool invoke 支持 `idempotency_key`，相同 key 在 24h 内返回相同结果
- `idempotency_key` 建议格式：`{run_id}:{tool_name}:{args_hash}`

---

## 11. 可靠性与默认策略（MVP）

| 配置项               | 默认值           | 说明                                |
| -------------------- | ---------------- | ----------------------------------- |
| 鉴权方式             | 静态 `api_key` | 外部通道到 `ingress` 的 `hello` |
| client tool 离线处理 | 立即失败         | 返回 `client_offline` 错误        |
| tool 超时            | 60s              | 可按 tool 配置                      |
| 审批超时             | 10min            | 超时后自动标记 `EXPIRED`          |
| 幂等 key TTL         | 24h              | tool invoke 幂等去重                |
| Agent 心跳间隔       | 30s              | Agent 健康检查                      |
| Agent 失联阈值       | 90s              | 超过此时间标记 `unhealthy`        |

---

## 12. 平台运行数据与上下文存储

### 12.1 平台不管理 Agent 上下文（仅提供存储封装）

**结论定稿**：平台不负责管理/加工 agent 的上下文，只提供"上下文存储能力（storage）"。

**平台必须持久化的运行数据**（不等同于 agent 上下文）：

| 数据                       | 说明                                                |
| -------------------------- | --------------------------------------------------- |
| Run/ToolCall/Approval 状态 | 保证"工具/审批阻塞节点"可恢复、可推进               |
| Trace/回放事件流           | 用户输入、agent 调用、LLM 调用、tool 调用、审批节点 |
| Session 消息               | 会话级的完整 transcript                             |

**平台不做的事**（明确不在边界内）：

- 不做 memory 选择、压缩、总结、检索、对齐等"智能上下文管理"
- 不托管 agent 的长期记忆/向量库/私有状态（由 agent 自己决定形态与存储）

**平台提供的能力**（仅存储封装，agent 自愿使用）：

- `context store`（KV/Blob/版本化对象）：agent 可将任意上下文片段写入并获取 `context_key`
- 平台事件中仅记录 `context_key` 或摘要引用，避免把 agent 私有状态强绑定到平台数据模型

### 12.2 数据模型（PostgreSQL）

```sql
-- 会话表
CREATE TABLE sessions (
    session_id      VARCHAR(64) PRIMARY KEY,
    user_id         VARCHAR(64) NOT NULL,
    created_at      TIMESTAMP NOT NULL DEFAULT NOW(),
    metadata        JSONB
);

-- 会话消息表（transcript 存储）
CREATE TABLE messages (
    message_id      VARCHAR(64) PRIMARY KEY,
    session_id      VARCHAR(64) NOT NULL REFERENCES sessions(session_id),
    run_id          VARCHAR(64),                    -- 关联的 run（可选）
    role            VARCHAR(16) NOT NULL,           -- user/assistant/system
    content         TEXT NOT NULL,
    created_at      TIMESTAMP NOT NULL DEFAULT NOW(),
    metadata        JSONB
);
CREATE INDEX idx_messages_session ON messages(session_id, created_at);

-- 执行表
CREATE TABLE runs (
    run_id          VARCHAR(64) PRIMARY KEY,
    session_id      VARCHAR(64) NOT NULL REFERENCES sessions(session_id),
    root_agent_id   VARCHAR(64) NOT NULL,
    parent_run_id   VARCHAR(64),                    -- 父 run（agent-to-agent）
    status          VARCHAR(32) NOT NULL,
    started_at      TIMESTAMP NOT NULL DEFAULT NOW(),
    ended_at        TIMESTAMP,
    error           JSONB
);
CREATE INDEX idx_runs_session ON runs(session_id, started_at);

-- 事件表（append-only trace）
CREATE TABLE events (
    event_id        VARCHAR(64) PRIMARY KEY,
    run_id          VARCHAR(64) NOT NULL REFERENCES runs(run_id),
    ts              BIGINT NOT NULL,                -- 毫秒时间戳
    type            VARCHAR(64) NOT NULL,
    payload         JSONB
);
CREATE INDEX idx_events_run ON events(run_id, ts);

-- Agent 注册表
CREATE TABLE agents (
    agent_id        VARCHAR(64) PRIMARY KEY,
    name            VARCHAR(256) NOT NULL,
    endpoint        VARCHAR(512) NOT NULL,
    capabilities    JSONB,
    status          VARCHAR(32) NOT NULL DEFAULT 'healthy',
    last_heartbeat  TIMESTAMP,
    created_at      TIMESTAMP NOT NULL DEFAULT NOW()
);

-- 工具注册表
CREATE TABLE tools (
    tool_name       VARCHAR(128) PRIMARY KEY,
    kind            VARCHAR(16) NOT NULL,           -- server/client
    policy          JSONB,                          -- allow/require_approval/block 规则
    timeout_ms      INT NOT NULL DEFAULT 60000,
    metadata        JSONB
);

-- 工具调用表
CREATE TABLE tool_calls (
    tool_call_id    VARCHAR(64) PRIMARY KEY,
    run_id          VARCHAR(64) NOT NULL REFERENCES runs(run_id),
    tool_name       VARCHAR(128) NOT NULL,
    kind            VARCHAR(16) NOT NULL,
    status          VARCHAR(32) NOT NULL,
    args            JSONB,
    result          JSONB,
    error           JSONB,
    approval_id     VARCHAR(64),
    created_at      TIMESTAMP NOT NULL DEFAULT NOW(),
    completed_at    TIMESTAMP
);
CREATE INDEX idx_tool_calls_run ON tool_calls(run_id);

-- 审批表
CREATE TABLE approvals (
    approval_id     VARCHAR(64) PRIMARY KEY,
    run_id          VARCHAR(64) NOT NULL REFERENCES runs(run_id),
    tool_call_id    VARCHAR(64) NOT NULL REFERENCES tool_calls(tool_call_id),
    status          VARCHAR(32) NOT NULL DEFAULT 'PENDING',
    created_at      TIMESTAMP NOT NULL DEFAULT NOW(),
    decided_at      TIMESTAMP,
    decided_by      VARCHAR(64),
    reason          TEXT
);

-- 上下文存储（可选，给 agent 自愿存取）
CREATE TABLE context_blobs (
    context_key     VARCHAR(128) PRIMARY KEY,
    owner_type      VARCHAR(32) NOT NULL,           -- agent/session/run
    owner_id        VARCHAR(64) NOT NULL,
    version         INT NOT NULL DEFAULT 1,
    data            JSONB,
    created_at      TIMESTAMP NOT NULL DEFAULT NOW()
);
```

---

## 13. 部署配置

### 13.1 Docker Compose（MVP）

```yaml
version: '3.8'

services:
  orchestrator:
    image: platform/orchestrator:latest
    ports:
      - "8080:8080"     # API
      - "8081:8081"     # Internal (for ingress)
    environment:
      - DATABASE_URL=postgres://postgres:password@postgres:5432/platform
      - LLM_ROUTER_URL=https://llm-router.example.com
      - LOG_LEVEL=info
    depends_on:
      - postgres
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:8080/health"]
      interval: 10s
      timeout: 5s
      retries: 3

  ingress:
    image: platform/ingress:latest
    ports:
      - "8090:8090"     # WebSocket
    environment:
      - ORCHESTRATOR_URL=http://orchestrator:8081
      - LOG_LEVEL=info
    depends_on:
      - orchestrator

  postgres:
    image: postgres:15
    ports:
      - "5432:5432"
    environment:
      - POSTGRES_USER=postgres
      - POSTGRES_PASSWORD=password
      - POSTGRES_DB=platform
    volumes:
      - postgres_data:/var/lib/postgresql/data

volumes:
  postgres_data:
```

### 13.2 环境变量

| 变量                            | 服务         | 说明                  | 默认值     |
| ------------------------------- | ------------ | --------------------- | ---------- |
| `DATABASE_URL`                | orchestrator | PostgreSQL 连接串     | -          |
| `LLM_ROUTER_URL`              | orchestrator | 外部 LLM 路由系统地址 | -          |
| `TOOL_TIMEOUT_MS`             | orchestrator | 工具执行超时          | `60000`  |
| `APPROVAL_TIMEOUT_MS`         | orchestrator | 审批超时              | `600000` |
| `AGENT_HEARTBEAT_INTERVAL_MS` | orchestrator | Agent 心跳间隔        | `30000`  |
| `ORCHESTRATOR_URL`            | ingress      | Orchestrator 内部地址 | -          |
| `WS_PING_INTERVAL_MS`         | ingress      | WebSocket ping 间隔   | `30000`  |
| `LOG_LEVEL`                   | all          | 日志级别              | `info`   |

---

## 14. 实施里程碑（建议顺序）

| 里程碑       | 目标                                                                             | 预计周期 |
| ------------ | -------------------------------------------------------------------------------- | -------- |
| **M0** | IM 调用与 agent SSE 透传（`agent_invoke → delta → done`），events 落库可回放 | 1 周     |
| **M1** | Agent 动态注册/发现（`agents/register`, `GET /agents`），健康检查            | 0.5 周   |
| **M2** | OpenAI 兼容代理（含 streaming）+ LLM 事件记录                                    | 1 周     |
| **M3** | Server tool（同步）+ tool_call 状态机 + 事件                                     | 1 周     |
| **M4** | Client tool（IM 下发/回收）+ 超时/离线处理                                       | 1 周     |
| **M5** | 审批节点（policy → approval_required → decision → 继续执行 tool）             | 1 周     |
| **M6** | 基础监控指标（run/tool 成功率、p95、等待审批时长）                               | 0.5 周   |

> **说明**：以上里程碑默认实现于单体 `orchestrator` 内部模块；后续如需扩展，再按 §3.3 进行服务拆分。

---

## 附录 A：Python SDK 示例

```python
from platform_sdk import PlatformClient, Tool

# 初始化客户端（Agent 侧使用）
client = PlatformClient(
    base_url="http://orchestrator:8080",
    run_id="run_001"  # 从请求 header 获取
)

# 调用 LLM（OpenAI 兼容）
response = client.chat.completions.create(
    model="gpt-4",
    messages=[{"role": "user", "content": "Hello"}],
    stream=True
)

for chunk in response:
    print(chunk.choices[0].delta.content, end="")

# 调用 Tool（同步封装，内部处理 pending/wait）
result = client.tools.invoke(
    tool_name="weather.query",
    args={"city": "北京"}
)
print(result)  # {"weather": "晴", "temperature": 25}

# 调用其他 Agent
child_response = client.agents.invoke(
    agent_id="weather_agent",
    message={"role": "user", "content": "查询北京天气"}
)
```

---

## 附录 B：术语表

| 术语     | 英文         | 说明                               |
| -------- | ------------ | ---------------------------------- |
| 协调者   | Orchestrator | 平台核心服务，负责路由与状态机推进 |
| 接入层   | Ingress      | 统一接入服务，对接外部通道         |
| 执行     | Run          | 用户调用 agent 的一次完整执行      |
| 工具调用 | ToolCall     | 单次工具调用的执行记录             |
| 审批     | Approval     | 敏感操作的人工审批任务             |
| 事件     | Event        | Trace 记录的最小单元               |
| 会话     | Session      | 用户维度的对话上下文               |
| 消息     | Message      | 会话中的单条消息（transcript）     |
