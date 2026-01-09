# 架构设计评估与改进建议（基于 `ARCHITECTURE_MVP.md` v1.0）

> 目标：在不破坏 MVP 边界（Ingress 只管连接与投递、Orchestrator 只管路由与状态机）的前提下，补齐“可落地的可靠性/安全/并发/可观测”细节，使协议与状态机在真实网络与生产约束下依然稳定。

---

## 0. 结论摘要（TL;DR）

- 你的 MVP 分层（Ingress/Orchestrator/外部 Agent）和“Tool 异步对象模型 + 审批一等公民”的方向是对的，能支撑企业级 Human-in-the-loop。
- 最大的落地风险集中在：**长耗时等待（审批/客户端工具/Agent 首包）导致连接超时**、**Agent 动态注册与 Tool 权限缺口**、以及 **状态机并发推进的一致性**。
- 建议按优先级分三档推进：
  - **P0（MVP 必须）**：Tool wait/轮询语义、注册鉴权、状态迁移 CAS、事件幂等与顺序、关键指标与 trace 关联。
  - **P1（强烈建议）**：Ingress↔Orchestrator 解耦（最小 async buffer / 事件拉取）、取消/超时统一、速率限制、审计与敏感数据策略。
  - **P2（后续演进）**：多租户、策略 DSL、MQ worker、弹性扩缩容、Agent 市场等。

---

## 1. 总体评估（Design Review）

### 1.1 设计亮点

- **边界清晰**：Ingress 与 Orchestrator 分工明确，利于后续多协议接入（WebSocket/HTTP/gRPC）与水平扩展。
- **Agent 接入成本低**：Agent 侧采用 OpenAI 兼容代理（`/v1/chat/completions`），能直接复用成熟 SDK 与生态。
- **审批/Client Tool 作为状态机节点**：把“未来完成”的阻塞节点建模为 `ToolCall`/`Approval`，且有 `GET /v1/tool_calls/{id}` + `:wait`，这是非常正确的抽象。
- **Trace 事件流 append-only**：天然支持回放、审计、调试与问题定位，是平台级能力的核心资产。

### 1.2 主要风险点（按影响排序）

1. **长等待导致连接断开**：审批默认 10min，但 HTTP 网关常见超时 60s；同类风险也出现在 Ingress↔Orchestrator 内部链路、以及 Agent SSE 的“首包等待”。
2. **Agent 注册安全与供应链风险**：`POST /v1/agents/register` 如果开放，等价于允许外部注入执行入口（并间接获得调用 LLM/Tool 的通道）。
3. **状态机并发推进冲突**：审批决策、超时任务、客户端 tool_result 回收、以及重试逻辑可能并发更新同一 `tool_calls`/`runs`。
4. **事件投递的幂等/顺序/断线恢复细节不足**：WebSocket 重连、重复投递、乱序到达、以及“客户端本地缓存与服务端事件源”需要明确契约。
5. **可观测性更偏回放而非运营**：目前有事件类型，但缺少“指标化 + 成本归因 + SLO 视角”的最小闭环。

---

## 2. P0（MVP 必须）改进项

### 2.1 Tool 调用的等待机制：避免长连接超时（Reliability & Protocol）

#### 问题
- `POST /v1/tools/{tool_name}:invoke` 返回 `pending` 后，如果 SDK 使用一次性 `:wait(timeout_ms=600000)`，在网关/Nginx/LB 的 60s/120s 超时下会被切断。

#### 建议（保持你现有 API 不变，仅补齐语义与 SDK 行为）

1. **明确 `:wait` 的定位：长轮询（long-poll），且必须“短超时”**
   - 建议 SDK 默认 `wait_timeout_ms=30000`（30s），并在 pending 时循环调用 `:wait`。
   - `:wait` 返回只要不是终态，就继续下一轮（可带轻微 jitter，避免羊群效应）。

2. **SDK 参考算法（建议写入 SDK 设计/文档）**

   - 伪流程：
     - `invoke()`
       - `status == succeeded/failed` → 直接返回
       - `status == pending` → 进入等待循环
     - loop:
       - `POST /v1/tool_calls/{id}:wait?timeout_ms=30000`
       - 若返回 `succeeded/failed/timeout/rejected/blocked` → 返回/抛错
       - 若仍为 `pending/waiting_*` → sleep(200~800ms 带 jitter) → 继续

3. **补齐 ToolCall 状态与 `status` 字段映射**
   - 文档层建议规定：API `status` 与内部 `tool_calls.status` 的映射表（例如：
     - `WAITING_APPROVAL`/`WAITING_CLIENT` → API `pending`
     - `SUCCEEDED` → API `succeeded`
     - `FAILED`/`REJECTED`/`BLOCKED`/`TIMEOUT` → API `failed`，并在 `error.code` 区分）
   - 这样 Agent SDK 的异常分类更稳定，也利于重试策略。

4. **幂等与重复调用约束（MVP 必须写清楚）**
   - 你已经有 `idempotency_key`：建议明确语义：
     - 相同 `idempotency_key` 在 TTL 内必须返回 **相同 `tool_call_id`**（无论 pending/succeeded/failed）。
     - `args` 变化但 key 相同 → 直接 409（或返回 failed: invalid_request），避免“错绑结果”。


### 2.2 Agent 动态注册的安全收口（Security）

#### 问题
- `POST /v1/agents/register` 本质是“把执行入口加入平台路由表”，一旦开放会带来：SSRF、恶意 endpoint、滥用平台 LLM/Tool 代理等。

#### 建议（MVP 可实现的最小闭环）

1. **注册鉴权**
   - 在 `POST /v1/agents/register` 强制 `x-admin-key`（或 `Authorization: Bearer <admin_token>`）。
   - 将注册接口仅暴露在内网，或仅允许来自管理面/CI 流水线网络段。

2. **Endpoint 校验与 allowlist**
   - 校验 `endpoint` 必须是允许的 scheme（仅 `http/https`），禁止 link-local、metadata IP 段等（SSRF 防护）。
   - MVP 可先做“域名/网段 allowlist”。

3. **Agent 访问平台的鉴权一致性**
   - 当前外部通道用静态 `api_key`；建议补齐 Agent→平台也要携带可校验身份（至少区分 agent_id）。
   - MVP 可采用：Agent 注册时下发 `agent_api_key`，后续 Agent 调用平台代理接口必须携带。


### 2.3 状态机推进：并发一致性与 CAS（State Machine Consistency）

#### 问题
- 并发来源：审批回调、client tool_result、超时扫描、重试、重复提交。
- 若没有“状态前置条件”，会出现：重复执行、覆盖结果、或 run 卡死。

#### 建议（DB 层即可落地）

1. **所有状态更新必须带前置状态（CAS）**
   - `UPDATE ... WHERE id=? AND status IN (...)`，更新行数为 0 视为冲突或已处理。

2. **对关键行使用事务与行锁（MVP 版本够用）**
   - 在执行 `tool_call`（从 `DISPATCHED/APPROVED` → `RUNNING`）时使用 `SELECT ... FOR UPDATE` 锁定该 `tool_call`，保证只执行一次。

3. **建议补齐字段以支持冲突检测**（若你会实现 DB schema，可在后续代码阶段做）
   - `tool_calls.updated_at`、`tool_calls.version`（或仅 `updated_at` + CAS）。
   - `approvals.decided_at` + 决策幂等（重复 decision 返回已决策结果）。

4. **Run 推进规则要与 ToolCall 终态一致**
   - 你已有规则：“只有 `SUCCEEDED` 才允许 run 继续”。建议补充：
     - `REJECTED/BLOCKED/FAILED/TIMEOUT` → run 进入 `FAILED`（或 `CANCELLED`，视业务），并写入 `run_failed` 事件。
     - 避免 run 在 `PAUSED_*` 下悬挂。


### 2.4 事件与连接可靠性：断线恢复与幂等（Ingress & Replay）

#### 问题
- WebSocket 断线重连必然发生；同时服务端可能重复推送同一事件（重试/重放/网络抖动）。

#### 建议（不改变你已定义的回放 API）

1. **事件具备稳定的去重键**
   - 你已有 `event_id`（events 表主键）。建议在平台→客户端的所有消息里带上 `event_id`（或 `seq`）。
   - 客户端以 `event_id` 去重，服务端可以“至少一次投递”。

2. **定义“重连恢复协议”**
   - 客户端重连时上报 `last_event_id` 或 `after_ts`。
   - Ingress 通过 `GET /v1/runs/{run_id}/events?after_ts=...` 补发缺失事件。

3. **补齐 ACK 语义（轻量即可）**
   - MVP 里可以先不做复杂 ACK，但需要明确：
     - 平台对客户端是“尽力而为实时推送 + 可回放补偿”。
     - UI 必须可通过回放重建最终状态。


### 2.5 可观测性最小闭环：指标、成本、日志上下文（Observability）

1. **成本归因（建议写进事件 payload）**
   - 在 `llm_call_done` 事件里除了 tokens，增加 `estimated_cost`（按模型单价配置计算）。
   - 在 `run_done` 汇总 `total_estimated_cost`。

2. **强制日志上下文**
   - 平台侧：所有日志至少带 `run_id`、`session_id`，工具相关日志带 `tool_call_id`。
   - Agent 侧：建议在接收 `/invoke` 时把 `x-run-id` 写入日志 MDC。

3. **MVP 必备指标（建议在改进文档里列出来，方便后续实现）**
   - `run_success_rate`、`run_p95_duration_ms`
   - `tool_success_rate`、`tool_p95_duration_ms`
   - `approval_wait_p95_ms`
   - `client_tool_offline_count`

---

## 3. P1（强烈建议）改进项

### 3.1 Ingress ↔ Orchestrator 的最小解耦（避免内部链路也被“长连接”绑死）

你的主文档里已经在 §3.3 描述了“同步/异步双链路”的后续演进。建议在 MVP 阶段就引入一个 **极轻量的 Async Buffer**（不一定上 MQ）：

- Ingress 调 Orchestrator 只做“创建 run + 触发执行”，严格超时（例如 3~5s）并快速返回 `run_id`。
- Orchestrator 持续把事件写入 `events`（你已经有）。
- Ingress 对活跃连接用轮询/订阅的方式读取 `events` 并推送给客户端。

这样做的收益：
- 不需要让 Ingress↔Orchestrator 持续保持一条可能跑 10 分钟的内部连接。
- 与“回放即事实来源（events source of truth）”的理念一致。

### 3.2 取消/超时统一语义

- 你已有 `cancel_run`：建议明确 run 取消后对 in-flight 的 tool/agent 事件如何处理：
  - 平台继续接收但忽略（只写审计事件），或尝试向 Agent 发取消信号（可选）。
- `approval_timeout`/`tool_timeout` 建议统一为“状态机终态 + 事件”而不仅是错误码。

### 3.3 速率限制与滥用防护（基础版）

- 外部通道（hello/api_key）按 `user_id`/连接/分钟做限流。
- Agent→平台代理接口按 `agent_id` 限流，避免某个 agent 把平台打爆。

### 3.4 敏感数据与审计策略（最小约束）

- events/messages 中会存用户输入与输出：建议在文档中明确数据保留期与访问审计。
- 对 tool args/result：提供“摘要字段”（例如 `args_summary`），UI/审批界面默认展示摘要，原始参数按权限查看。

---

## 4. P2（后续演进）方向

- 多租户与权限体系（user/org/project），把策略引擎升级为可配置 DSL。
- MQ + worker 承载异步任务（你在 §3.3 已规划），并支持弹性扩缩容与任务优先级。
- Agent 市场/版本管理：灰度、回滚、健康度评分与自动摘除。

---

## 5. 建议补充到主文档的“需要明确的契约”（Checklist）

- Tool API `status` 与内部状态机映射表
- SDK 的 `invoke + wait(loop)` 默认行为与超时策略
- `idempotency_key` 的严格语义与冲突处理
- `event_id` 去重与断线恢复流程
- `agents/register` 的鉴权与网络暴露面
- 状态迁移 CAS 与冲突码（例如 409）

