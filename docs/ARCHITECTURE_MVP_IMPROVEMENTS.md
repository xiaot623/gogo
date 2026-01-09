# 架构设计评估与改进建议

## 1. 总体评估

*   **优点**：
    *   **边界清晰**：Ingress 与 Orchestrator 分离，业务逻辑与通信协议解耦，非常利于后续支持多协议（HTTP/WebSocket/gRPC）。
    *   **标准兼容**：Agent 侧采用 OpenAI 兼容协议，极大降低了算法侧接入成本，也便于利用现有的 LLM 生态工具链。
    *   **Human-in-the-loop 设计完善**：将“审批”作为状态机的一等公民，并设计了 Server/Client Tool 的异步模型，这是企业级 Agent 平台的刚需。
    *   **数据模型合理**：Trace 事件流采用 Append-only 模式，不仅用于调试，还直接支撑了“回放”功能，设计很有前瞻性。

*   **潜在风险**：
    *   **长连接超时问题**：设计中 Agent 调用 Tool 遇到审批时，虽然 SDK 封装了 `wait`，但在 HTTP 层面长时间 hold 连接（如审批耗时 10 分钟）极易被中间件（Nginx/Load Balancer）切断。
    *   **Agent 注册安全**：动态注册接口缺乏明确的安全管控，可能导致恶意服务注册。

## 2. 具体改进建议

### 2.1 核心交互协议优化 (Reliability & Protocols)

**问题**：关于 Tool 调用中的“审批等待”机制。
文档中提到 SDK 内部调用 `invoke + wait`。如果审批需要 10 分钟，而 HTTP 网关超时通常是 60s。

**建议**：
*   **SDK 轮询机制**：SDK 不应单纯依赖长连接等待。应实现 **"Polling (轮询)"** 或 **"Long-polling with timeout"** 模式。
    *   流程：`invoke` -> 返回 `pending` -> SDK sleep -> SDK 定期调用 `GET /tool_calls/{id}` 查询状态 -> 直到 `succeeded`。
    *   这样可以避免长连接被网络设施切断的问题。
    *   **SDK 实现细节**：
        1. 发起 `invoke`，若返回 `pending`，则进入轮询循环。
        2. 循环调用 `wait`（设置合理的 timeout，如 30s）。
        3. 若 `wait` 超时返回（状态仍为 pending），SDK 捕获超时并继续下一轮 `wait`，直到状态变为终态。

### 2.2 安全性增强 (Security)

**问题**：`POST /v1/agents/register` 接口目前是开放的。

**建议**：
*   **注册鉴权**：增加 `X-Admin-Token` 或只允许通过内部管理后台/CI CD 流程注册 Agent。
    *   Header: `x-admin-key`: 管理员密钥
*   **工具权限隔离 (ACL)**：在“策略引擎”部分补充 ACL 设计。明确 **哪个 Agent 有权限调用哪个 Tool**。

### 2.3 状态机与并发控制 (State Machine)

**问题**：`tool_call` 状态流转可能存在并发更新风险（例如用户审批的同时，超时检测任务也在运行）。

**建议**：
*   **乐观锁 (Optimistic Locking)**：所有状态更新必须采用 **CAS (Compare-And-Swap)** 机制。
    *   SQL 示例：
        ```sql
        UPDATE tool_calls 
        SET status = 'APPROVED', result = '...', updated_at = NOW()
        WHERE tool_call_id = 'tc_001' AND status = 'WAITING_APPROVAL';
        ```

### 2.4 可观测性细化 (Observability)

**问题**：目前的 Trace 比较偏向“功能性”回放。

**建议**：
*   **Cost 归因**：在 `run_done` 或 `llm_call_done` 事件中，不仅记录 token 数，建议直接计算并记录 **预估成本 (USD/RMB)**。
*   **结构化日志上下文**：强制要求 Agent 在日志中注入 `trace_id` (即 `run_id`)。

### 2.5 架构微调建议 (Refined Architecture)

针对 **Section 3.3 后续演进**，在 MVP 阶段可以引入轻量级的 **"Async Buffer"**：
*   Ingress 对 Orchestrator 的调用设置严格超时（如 5s）。
*   Orchestrator 异步启动 Agent 调用后立即返回，而不是等待 Agent 的首字生成。
