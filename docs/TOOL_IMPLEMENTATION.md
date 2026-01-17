# Orchestrator Tool 功能实现文档 (MVP 版本)

## 概述

Orchestrator 提供了一个简洁的工具编排系统，支持服务端内置工具和客户端上报工具的统一管理和调用。Agent 通过 tool_proxy 机制透明地使用工具，无需关心工具的权限和执行细节。本文档详细描述了 MVP 版本的 tool 相关功能实现。

## 目录

1. [核心概念](#核心概念)
2. [架构设计](#架构设计)
3. [工具来源与注册](#工具来源与注册)
4. [工具调用流程](#工具调用流程)
5. [Tool Proxy 机制](#tool-proxy-机制)
6. [异步执行模型](#异步执行模型)
7. [数据模型](#数据模型)
8. [API 接口](#api-接口)

---

## 核心概念

### 工具来源（Tool Source）

系统支持两种工具来源:

- **Server Built-in Tools**: 服务端内置工具
  - 由 orchestrator 直接执行
  - 启动时自动注册
  - 示例: `weather.query`, `database.query`

- **Client Provided Tools**: 客户端上报工具
  - 由客户端实现和执行
  - 客户端启动时上报可用工具列表
  - 示例: `browser.screenshot`, `file.read`

### Agent 工具列表

Agent 可用的工具列表由以下两部分组成:

```
Agent ToolCall List = Server Built-in Tools + Client Provided Tools
```

- Agent 无需区分工具来源
- Tool Proxy 机制自动路由到正确的执行器
- 对 Agent 透明，统一调用接口

### 工具调用状态（ToolCall Status）

简化的工具调用生命周期状态:

```
PENDING (调用已创建)
  ↓
RUNNING (执行中)
  ↓
SUCCEEDED / FAILED / TIMEOUT
```

### Tool Proxy 机制

Tool Proxy 是工具调用的代理层，负责:

- **权限检查**: 自动处理工具权限验证（对 Agent 透明）
- **路由分发**: 根据工具来源路由到服务端或客户端执行
- **结果聚合**: 统一的结果返回格式
- **超时控制**: 自动处理执行超时

---

## 架构设计

### 简化的目录结构

```
orchestrator/
├── main.go                              # 应用入口
├── internal/
│   ├── domain/                          # 领域模型
│   │   ├── tool.go                      # Tool & ToolCall 模型
│   │   └── enums.go                     # 状态枚举
│   ├── service/                         # 业务逻辑
│   │   ├── tool_proxy.go                # Tool Proxy 核心逻辑
│   │   └── tool.go                      # 工具调用与执行
│   ├── repository/                      # 数据持久化
│   │   ├── store.go                     # 存储接口
│   │   └── sqlite.go                    # SQLite 实现
│   ├── transport/http/
│   │   ├── v1/                          # Agent API
│   │   │   └── tools.go                 # 工具调用接口
│   │   └── internalapi/                 # 内部 API
│   │       ├── tools.go                 # 客户端工具注册
│   │       └── results.go               # 工具结果提交
│   └── adapter/
│       ├── llm/                         # LLM 代理客户端
│       └── ingress/                     # 事件推送
```

### 核心组件

1. **Tool Proxy Layer** (`internal/service/tool_proxy.go`)
   - 工具调用路由
   - 权限验证（对 Agent 透明）
   - 执行器选择

2. **Service Layer** (`internal/service/tool.go`)
   - 工具调用编排
   - 结果轮询
   - 超时处理

3. **Repository Layer** (`internal/repository/`)
   - 工具注册表
   - 调用记录持久化
   - SQLite 存储

4. **Transport Layer** (`internal/transport/http/`)
   - Agent API: 工具调用接口
   - Internal API: 客户端工具注册和结果提交

### 架构图

```
┌─────────────────────────────────────────────────────────┐
│                        Agent                            │
│  (无需关心工具来源、权限、执行细节)                       │
└───────────────────────┬─────────────────────────────────┘
                        │ ToolCall Request
                        ↓
┌─────────────────────────────────────────────────────────┐
│                   Tool Proxy Layer                       │
│  - 权限检查 (透明)                                       │
│  - 路由分发                                              │
│  - 结果聚合                                              │
└────────────┬────────────────────────┬───────────────────┘
             │                        │
             ↓                        ↓
┌────────────────────┐    ┌──────────────────────────────┐
│  Server Built-in   │    │   Client Provided Tools      │
│      Tools         │    │   (via ingress)              │
│  - weather.query   │    │   - browser.screenshot       │
│  - database.query  │    │   - file.read                │
└────────────────────┘    └──────────────────────────────┘
```

---

## 工具来源与注册

### 数据模型

**Tool 结构** (`internal/domain/tool.go`):

```go
type Tool struct {
    Name      string          // 工具唯一标识 (e.g., "weather.query")
    Source    ToolSource      // "server" 或 "client"
    Schema    json.RawMessage // 工具参数 schema (JSON Schema)
    TimeoutMs int             // 执行超时时间（毫秒）
    Metadata  json.RawMessage // 附加元数据
    ClientID  string          // 客户端 ID（仅 client 工具）
}

type ToolSource string
const (
    ToolSourceServer ToolSource = "server"  // 服务端内置
    ToolSourceClient ToolSource = "client"  // 客户端提供
)
```

### 1. 服务端内置工具

**启动时自动注册** (`internal/repository/sqlite.go`):

```go
func (r *SQLiteStore) initBuiltinTools(ctx context.Context) error {
    builtinTools := []domain.Tool{
        {
            Name:      "weather.query",
            Source:    domain.ToolSourceServer,
            TimeoutMs: 5000,
            Schema:    json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"}}}`),
        },
        {
            Name:      "database.query",
            Source:    domain.ToolSourceServer,
            TimeoutMs: 10000,
            Schema:    json.RawMessage(`{"type":"object","properties":{"sql":{"type":"string"}}}`),
        },
    }

    for _, tool := range builtinTools {
        r.CreateTool(ctx, &tool)
    }
    return nil
}
```

**内置工具示例**:

| 工具名 | 超时 | 说明 |
|--------|------|------|
| `weather.query` | 5s | 天气查询 |
| `database.query` | 10s | 数据库查询 |
| `calculation.eval` | 3s | 数学计算 |

### 2. 客户端工具注册

**注册流程**:

1. 客户端启动
2. 调用 `/internal/tools/register` API 上报可用工具列表
3. Orchestrator 存储客户端工具信息
4. 将工具加入 Agent 可用工具列表

**API 接口** (`internal/transport/http/internalapi/tools.go`):

```http
POST /internal/tools/register
Content-Type: application/json

Request:
{
    "client_id": "client_abc123",
    "tools": [
        {
            "name": "browser.screenshot",
            "schema": {
                "type": "object",
                "properties": {
                    "url": {"type": "string"},
                    "width": {"type": "integer"},
                    "height": {"type": "integer"}
                },
                "required": ["url"]
            },
            "timeout_ms": 30000
        },
        {
            "name": "file.read",
            "schema": {
                "type": "object",
                "properties": {
                    "path": {"type": "string"}
                },
                "required": ["path"]
            },
            "timeout_ms": 5000
        }
    ]
}

Response:
{
    "ok": true,
    "registered_count": 2
}
```

**实现代码**:

```go
func (h *InternalAPIHandler) RegisterTools(c *gin.Context) {
    var req domain.ToolRegistrationRequest
    c.BindJSON(&req)

    for _, toolSchema := range req.Tools {
        tool := &domain.Tool{
            Name:      toolSchema.Name,
            Source:    domain.ToolSourceClient,
            Schema:    toolSchema.Schema,
            TimeoutMs: toolSchema.TimeoutMs,
            ClientID:  req.ClientID,
        }
        h.service.RegisterTool(ctx, tool)
    }

    c.JSON(200, gin.H{"ok": true, "registered_count": len(req.Tools)})
}
```

### 3. 获取 Agent 工具列表

**API 接口**:

```http
GET /v1/tools

Response:
{
    "tools": [
        {
            "name": "weather.query",
            "source": "server",
            "schema": {...},
            "timeout_ms": 5000
        },
        {
            "name": "browser.screenshot",
            "source": "client",
            "schema": {...},
            "timeout_ms": 30000
        }
    ]
}
```

Agent 无需区分工具来源，统一调用即可。

### 数据库表结构

```sql
CREATE TABLE tools (
    name TEXT PRIMARY KEY,
    source TEXT NOT NULL,           -- "server" 或 "client"
    schema TEXT NOT NULL,            -- JSON Schema
    timeout_ms INTEGER NOT NULL,
    metadata TEXT,
    client_id TEXT,                  -- 客户端 ID（仅 client 工具）
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_tools_source ON tools(source);
CREATE INDEX idx_tools_client ON tools(client_id);
```

---

## 工具调用流程

### MVP 统一流程图

```
┌──────────────────────────────────────────────────────────┐
│ 1. Agent 调用工具                                         │
│    POST /v1/tools/:tool_name/invoke                      │
│    {run_id, args}                                        │
└────────────────┬─────────────────────────────────────────┘
                 ↓
┌──────────────────────────────────────────────────────────┐
│ 2. Tool Proxy Layer                                      │
│    - 获取工具定义                                         │
│    - 权限检查 (透明)                                      │
│    - 创建 ToolCall 记录 (PENDING)                        │
│    - 立即返回凭证                                         │
└────────────────┬─────────────────────────────────────────┘
                 │
                 ↓
         ┌───────┴───────┐
         │               │
    Server Tool      Client Tool
         │               │
         ↓               ↓
┌────────────────┐   ┌──────────────────┐
│ 3a. 服务端调度 │   │ 3b. 客户端调度    │
│                │   │                  │
│ - 后台执行     │   │ - 推送到 ingress │
│ - 更新状态     │   │ - 等待客户端执行  │
└────────────────┘   └──────────────────┘
         │                    │
         └──────────┬─────────┘
                    ↓
         ┌────────────────────┐
         │ 4. 返回凭证 (立即)  │
         │                    │
         │ tool_call_id       │
         │ status: pending    │
         └──────────┬─────────┘
                    │
                    ↓
         ┌────────────────────┐
         │ 5. Agent 轮询结果   │
         │ GET /v1/tool_calls/ │
         │     :tool_call_id   │
         │                    │
         │ 返回: PENDING /    │
         │      RUNNING /     │
         │      SUCCEEDED /   │
         │      FAILED        │
         └────────────────────┘
```

**关键特性**:
- 所有工具（服务端/客户端）统一返回凭证
- Agent 无需区分工具类型，统一使用轮询模式
- 简化 Agent 端代码逻辑

### 核心代码实现

**调用入口** (`internal/service/tool_proxy.go`):

```go
func (s *ToolProxy) InvokeTool(ctx context.Context, req domain.ToolInvokeRequest) (*domain.ToolInvokeResponse, error) {
    // 1. 获取工具定义
    tool, err := s.store.GetTool(ctx, req.ToolName)
    if err != nil {
        return nil, errors.New("tool not found")
    }

    // 2. 权限检查 (对 Agent 透明)
    if err := s.checkPermission(ctx, req); err != nil {
        return nil, err
    }

    // 3. 创建 ToolCall 记录
    toolCall := &domain.ToolCall{
        ToolCallID: generateID("tc_"),
        RunID:      req.RunID,
        ToolName:   req.ToolName,
        Source:     tool.Source,
        Status:     domain.ToolCallStatusPending,
        Args:       req.Args,
    }
    s.store.CreateToolCall(ctx, toolCall)

    // 4. 根据工具来源异步调度
    if tool.Source == domain.ToolSourceServer {
        // 服务端工具: 后台执行
        go s.executeServerToolAsync(context.Background(), toolCall, tool)
    } else {
        // 客户端工具: 推送到 ingress
        s.dispatchClientTool(ctx, toolCall, tool)
    }

    // 5. 立即返回凭证（统一行为）
    return &domain.ToolInvokeResponse{
        ToolCallID: toolCall.ToolCallID,
        Status:     "pending",
        Message:    "tool call created, use tool_call_id to poll result",
    }, nil
}
```

**服务端工具后台执行**:

```go
func (s *ToolProxy) executeServerToolAsync(ctx context.Context,
    toolCall *domain.ToolCall, tool *domain.Tool) {

    // 1. 标记为执行中
    toolCall.Status = domain.ToolCallStatusRunning
    s.store.UpdateToolCallStatus(ctx, toolCall.ToolCallID, toolCall.Status)

    // 2. 执行工具 (调用内置工具处理器)
    result, err := s.toolExecutor.Execute(ctx, tool.Name, toolCall.Args)

    // 3. 更新结果
    if err != nil {
        toolCall.Status = domain.ToolCallStatusFailed
        toolCall.Error = json.RawMessage(fmt.Sprintf(`{"message":"%s"}`, err.Error()))
    } else {
        toolCall.Status = domain.ToolCallStatusSucceeded
        toolCall.Result = result
    }

    // 4. 保存最终状态
    now := time.Now()
    toolCall.CompletedAt = &now
    s.store.UpdateToolCallResult(ctx, toolCall.ToolCallID, toolCall)
}
```

**客户端工具调度**:

```go
func (s *ToolProxy) dispatchClientTool(ctx context.Context,
    toolCall *domain.ToolCall, tool *domain.Tool) error {

    // 推送到 ingress (客户端会收到工具调用请求)
    err := s.ingressClient.PushToolRequest(ctx, domain.ToolRequest{
        ToolCallID: toolCall.ToolCallID,
        ToolName:   tool.Name,
        Args:       toolCall.Args,
        TimeoutMs:  tool.TimeoutMs,
    })

    return err
}
```

---

## Tool Proxy 机制

### 权限检查算法

Tool Proxy 对 Agent 透明地处理权限验证:

```go
func (s *ToolProxy) checkPermission(ctx context.Context, req domain.ToolInvokeRequest) error {
    // 1. 从上下文获取 Agent 身份
    agentID := ctx.Value("agent_id").(string)

    // 2. 检查 Agent 是否有权限使用该工具
    // MVP 版本: 简单的白名单检查
    allowed := s.store.CheckToolPermission(ctx, agentID, req.ToolName)

    if !allowed {
        return errors.New("permission denied")
    }

    return nil
}
```

**特点**:
- Agent 无需在调用时提供任何权限凭证
- 权限信息从请求上下文自动提取
- 支持基于 Agent 身份的细粒度权限控制
- MVP 版本使用简单的白名单机制，后续可扩展为 RBAC 或 ABAC

### 路由策略

根据工具来源自动路由:

```go
func (s *ToolProxy) routeTool(tool *domain.Tool) ToolExecutor {
    switch tool.Source {
    case domain.ToolSourceServer:
        return s.serverExecutor
    case domain.ToolSourceClient:
        return s.clientDispatcher
    default:
        return nil
    }
}
```

---

## 异步执行模型

### 统一的异步轮询模型

MVP 版本采用统一的异步执行模型，无论是服务端工具还是客户端工具，都使用相同的调用和结果获取流程。

**核心设计**:
- 所有工具调用立即返回凭证（tool_call_id）
- Agent 通过凭证轮询获取结果
- 无需区分工具类型

**执行流程**:

1. **调用阶段**:
   - Agent 调用工具 API
   - Orchestrator 创建 ToolCall 记录，状态为 PENDING
   - 根据工具来源调度执行（服务端后台执行 / 客户端推送到 ingress）
   - 立即返回 tool_call_id 给 Agent

2. **执行阶段**:
   - **服务端工具**: 后台 goroutine 执行，更新状态
   - **客户端工具**: 推送到 ingress，客户端收到后执行

3. **获取结果阶段**:
   - Agent 使用 tool_call_id 轮询结果
   - 调用 `GET /v1/tool_calls/:id`
   - 获取实时状态和结果（PENDING / RUNNING / SUCCEEDED / FAILED）

### 对比传统混合模式

**传统混合模式的问题**:
```python
# Agent 需要区分工具类型
response = call_tool(tool_name, args)
if response["status"] == "pending":
    # 客户端工具，需要轮询
    result = poll_until_complete(response["tool_call_id"])
else:
    # 服务端工具，直接返回结果
    result = response["result"]
```

**统一异步模型的优势**:
```python
# Agent 无需区分工具类型
response = call_tool(tool_name, args)
tool_call_id = response["tool_call_id"]
# 统一轮询获取结果
result = poll_until_complete(tool_call_id)
```

### 客户端结果提交

**API 接口** (`internal/transport/http/internalapi/results.go`):

```http
POST /internal/tool_calls/:tool_call_id/submit
Content-Type: application/json

Request:
{
    "status": "SUCCEEDED",  // 或 "FAILED"
    "result": {
        "screenshot_url": "https://example.com/screenshot.png"
    },
    "error": null
}

Response:
{
    "ok": true,
    "tool_call_id": "tc_abc123",
    "status": "SUCCEEDED"
}
```

**实现代码**:

```go
func (h *InternalAPIHandler) SubmitToolCallResult(c *gin.Context) {
    toolCallID := c.Param("tool_call_id")

    var req domain.ToolCallResultRequest
    c.BindJSON(&req)

    // 更新工具调用结果
    err := h.service.UpdateToolCallResult(ctx, toolCallID, &req)
    if err != nil {
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }

    c.JSON(200, gin.H{
        "ok": true,
        "tool_call_id": toolCallID,
        "status": req.Status,
    })
}
```

### Agent 结果轮询

**API 接口**:

```http
GET /v1/tool_calls/:tool_call_id

Response:
{
    "tool_call_id": "tc_abc123",
    "run_id": "run_001",
    "tool_name": "browser.screenshot",
    "source": "client",
    "status": "SUCCEEDED",      // PENDING, RUNNING, SUCCEEDED, FAILED, TIMEOUT
    "args": {...},
    "result": {...},
    "error": null,
    "created_at": 1234567890,
    "completed_at": 1234567900
}
```

**轮询逻辑示例** (Agent 端):

```python
def invoke_tool_and_wait(tool_name, args, timeout=30):
    # 1. 调用工具（所有工具统一返回凭证）
    response = requests.post(
        f"http://orchestrator/v1/tools/{tool_name}/invoke",
        json={"run_id": "run_001", "args": args}
    )
    tool_call_id = response.json()["tool_call_id"]

    # 2. 轮询结果（无需区分工具类型）
    start_time = time.time()
    while time.time() - start_time < timeout:
        result = requests.get(
            f"http://orchestrator/v1/tool_calls/{tool_call_id}"
        ).json()

        # 检查是否完成
        if result["status"] in ["SUCCEEDED", "FAILED", "TIMEOUT"]:
            if result["status"] == "SUCCEEDED":
                return result["result"]
            else:
                raise Exception(f"Tool execution failed: {result.get('error')}")

        time.sleep(0.5)  # 轮询间隔

    raise TimeoutError("Tool execution timeout")
```

**优势**:
- 统一的调用模式，无需区分工具类型
- 简化 Agent 代码逻辑
- 服务端工具和客户端工具行为一致

---

## 数据模型

### ToolCall 完整模型

```go
type ToolCall struct {
    ToolCallID  string          // 唯一标识 (tc_xxx)
    RunID       string          // 关联的运行 ID
    ToolName    string          // 工具名称
    Source      ToolSource      // "server" 或 "client"
    Status      ToolCallStatus  // 当前状态
    Args        json.RawMessage // 执行参数（JSON）
    Result      json.RawMessage // 执行结果（JSON）
    Error       json.RawMessage // 错误信息（JSON）
    CreatedAt   time.Time       // 创建时间
    CompletedAt *time.Time      // 完成时间
}
```

### 状态枚举

```go
type ToolCallStatus string

const (
    ToolCallStatusPending   ToolCallStatus = "PENDING"    // 等待执行
    ToolCallStatusRunning   ToolCallStatus = "RUNNING"    // 执行中
    ToolCallStatusSucceeded ToolCallStatus = "SUCCEEDED"  // 成功
    ToolCallStatusFailed    ToolCallStatus = "FAILED"     // 失败
    ToolCallStatusTimeout   ToolCallStatus = "TIMEOUT"    // 超时
)
```

### 数据库表结构

```sql
CREATE TABLE tool_calls (
    tool_call_id TEXT PRIMARY KEY,
    run_id TEXT NOT NULL,
    tool_name TEXT NOT NULL,
    source TEXT NOT NULL,           -- "server" 或 "client"
    status TEXT NOT NULL,            -- PENDING, RUNNING, SUCCEEDED, FAILED, TIMEOUT
    args TEXT,                       -- JSON 参数
    result TEXT,                     -- JSON 结果
    error TEXT,                      -- JSON 错误
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    completed_at DATETIME,
    FOREIGN KEY (run_id) REFERENCES runs(run_id),
    FOREIGN KEY (tool_name) REFERENCES tools(name)
);

CREATE INDEX idx_tool_calls_run ON tool_calls(run_id);
CREATE INDEX idx_tool_calls_status ON tool_calls(status);
CREATE INDEX idx_tool_calls_created ON tool_calls(created_at);
```

---

## API 接口

### Agent API (v1)

**1. 获取工具列表**

```http
GET /v1/tools

Response:
{
    "tools": [
        {
            "name": "weather.query",
            "source": "server",
            "schema": {...},
            "timeout_ms": 5000
        },
        {
            "name": "browser.screenshot",
            "source": "client",
            "schema": {...},
            "timeout_ms": 30000
        }
    ]
}
```

**2. 调用工具**

```http
POST /v1/tools/:tool_name/invoke
Content-Type: application/json

Request:
{
    "run_id": "run_abc123",
    "args": {
        "query": "weather in Beijing"
    }
}

Response (统一返回凭证):
{
    "tool_call_id": "tc_xyz789",
    "status": "pending",
    "message": "tool call created, use tool_call_id to poll result"
}
```

**说明**:
- 所有工具（服务端/客户端）统一返回凭证
- Agent 通过 `tool_call_id` 轮询结果
- 不再区分同步/异步响应

**3. 获取工具调用结果**

```http
GET /v1/tool_calls/:tool_call_id

Response:
{
    "tool_call_id": "tc_xyz789",
    "run_id": "run_abc123",
    "tool_name": "browser.screenshot",
    "source": "client",
    "status": "SUCCEEDED",
    "args": {...},
    "result": {...},
    "error": null,
    "created_at": 1234567890,
    "completed_at": 1234567900
}
```

### Internal API

**1. 注册客户端工具**

```http
POST /internal/tools/register
Content-Type: application/json

Request:
{
    "client_id": "client_abc123",
    "tools": [
        {
            "name": "browser.screenshot",
            "schema": {...},
            "timeout_ms": 30000
        }
    ]
}

Response:
{
    "ok": true,
    "registered_count": 1
}
```

**2. 提交工具执行结果**

```http
POST /internal/tool_calls/:tool_call_id/submit
Content-Type: application/json

Request:
{
    "status": "SUCCEEDED",
    "result": {...},
    "error": null
}

Response:
{
    "ok": true,
    "tool_call_id": "tc_xyz789",
    "status": "SUCCEEDED"
}
```

---

## 完整执行示例

### 示例 1: 服务端工具（异步轮询）

```bash
# 1. 调用天气查询工具
curl -X POST http://localhost:8080/v1/tools/weather.query/invoke \
  -H "Content-Type: application/json" \
  -d '{
    "run_id": "run_001",
    "args": {"query": "Beijing weather"}
  }'

# 响应 (返回凭证)
{
    "tool_call_id": "tc_001",
    "status": "pending",
    "message": "tool call created, use tool_call_id to poll result"
}

# 2. 轮询结果 (执行中)
curl http://localhost:8080/v1/tool_calls/tc_001

# 响应
{
    "tool_call_id": "tc_001",
    "status": "RUNNING",
    ...
}

# 3. 轮询结果 (已完成)
curl http://localhost:8080/v1/tool_calls/tc_001

# 响应
{
    "tool_call_id": "tc_001",
    "status": "SUCCEEDED",
    "result": {
        "weather": "Sunny",
        "temperature": 25
    },
    "completed_at": 1234567890
}
```

**内部流程**:
1. 创建 ToolCall 记录，状态为 PENDING
2. 后台 goroutine 执行工具
3. 更新状态为 RUNNING
4. 执行完成，更新状态为 SUCCEEDED
5. Agent 轮询获取结果

### 示例 2: 客户端工具（异步轮询）

```bash
# 1. Agent 调用浏览器截图工具
curl -X POST http://localhost:8080/v1/tools/browser.screenshot/invoke \
  -H "Content-Type: application/json" \
  -d '{
    "run_id": "run_002",
    "args": {"url": "https://example.com"}
  }'

# 响应 (返回凭证)
{
    "tool_call_id": "tc_002",
    "status": "pending",
    "message": "tool call created, use tool_call_id to poll result"
}

# 2. Agent 轮询结果 (等待中)
curl http://localhost:8080/v1/tool_calls/tc_002

# 响应
{
    "tool_call_id": "tc_002",
    "status": "PENDING",
    ...
}

# 3. 客户端提交结果 (内部 API)
curl -X POST http://localhost:8080/internal/tool_calls/tc_002/submit \
  -H "Content-Type: application/json" \
  -d '{
    "status": "SUCCEEDED",
    "result": {
        "screenshot_url": "https://cdn.example.com/screenshot.png"
    }
  }'

# 4. Agent 再次轮询 (已完成)
curl http://localhost:8080/v1/tool_calls/tc_002

# 响应
{
    "tool_call_id": "tc_002",
    "status": "SUCCEEDED",
    "result": {
        "screenshot_url": "https://cdn.example.com/screenshot.png"
    },
    "completed_at": 1234567890
}
```

**内部流程**:
1. 创建 ToolCall 记录，状态为 PENDING
2. 推送工具请求到 ingress
3. 客户端收到请求并执行
4. 客户端提交结果，更新状态为 SUCCEEDED
5. Agent 轮询获取结果

**与服务端工具的区别**:
- 服务端工具：orchestrator 后台执行
- 客户端工具：推送到 ingress，由客户端执行
- 对 Agent 而言：调用方式完全相同
