# Tool Proxy MVP 实现差距分析

本文档记录 `TOOL_IMPLEMENTATION.md` 规划与当前代码实现之间的差距，包括尚未实现的功能和不符合预期的实现。

---

## 一、尚未实现的功能

### 1.1 GET /v1/tools - 获取工具列表 API

**优先级**: 高

**规划描述**: Agent 需要获取可用工具列表，包括服务端内置工具和客户端提供的工具。

**API 规范**:
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

**实现位置**:
- `internal/service/tool.go` - 添加 `ListTools()` 方法
- `internal/transport/http/v1/tools.go` - 添加 `ListTools()` handler
- `internal/transport/http/v1/handler.go` - 注册路由 `e.GET("/v1/tools", h.ListTools)`

**依赖**: 存储层 `store.ListTools()` 已实现

---

### 1.2 POST /internal/tools/register - 客户端工具注册 API

**优先级**: 高

**规划描述**: 客户端启动时上报可用工具列表到 orchestrator。

**API 规范**:
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
        }
    ]
}

Response:
{
    "ok": true,
    "registered_count": 1
}
```

**实现位置**:
- `internal/domain/tool.go` - 添加 `ToolRegistrationRequest` 结构体
- `internal/service/tool.go` - 添加 `RegisterTools()` 方法
- `internal/transport/http/internalapi/tools.go` - 添加 `RegisterTools()` handler
- `internal/transport/http/internalapi/handler.go` - 注册路由

**依赖**: 存储层 `store.CreateTool()` 已实现

---

### 1.3 Tool 模型缺少 Schema 和 ClientID 字段

**优先级**: 高

**规划描述**: Tool 模型应包含 JSON Schema 定义和客户端标识。

**当前实现** (`internal/domain/tool.go`):
```go
type Tool struct {
    Name      string          `json:"name"`
    Kind      ToolKind        `json:"kind"`
    Policy    json.RawMessage `json:"policy"`
    TimeoutMs int             `json:"timeout_ms"`
    Metadata  json.RawMessage `json:"metadata,omitempty"`
}
```

**需要添加**:
```go
type Tool struct {
    Name      string          `json:"name"`
    Kind      ToolKind        `json:"kind"`
    Schema    json.RawMessage `json:"schema"`              // 新增
    ClientID  string          `json:"client_id,omitempty"` // 新增
    Policy    json.RawMessage `json:"policy"`
    TimeoutMs int             `json:"timeout_ms"`
    Metadata  json.RawMessage `json:"metadata,omitempty"`
}
```

**实现位置**:
- `internal/domain/tool.go` - 修改 Tool 结构体
- `internal/repository/sqlite.go` - 修改 tools 表 schema 和 CRUD 方法

---

### 1.4 Service 层 ListTools() 方法

**优先级**: 高

**规划描述**: 服务层需要封装存储层的 ListTools 方法。

**实现位置**:
- `internal/service/tool.go`

**代码示例**:
```go
func (s *Service) ListTools(ctx context.Context) ([]domain.Tool, error) {
    return s.store.ListTools(ctx)
}
```

---

## 二、不符合预期的实现

### 2.1 服务端工具同步执行（应改为异步）

**严重程度**: 🔴 高

**规划描述**: 文档规划所有工具（包括服务端工具）统一采用异步执行模型，调用后立即返回凭证，Agent 通过轮询获取结果。

**当前实现** (`internal/service/tool.go:181-200`):
```go
// 服务端工具同步执行，直接返回结果
result := `{"status":"executed"}`
s.store.UpdateToolCallResult(ctx, toolCallID, domain.ToolCallStatusSucceeded, []byte(result), nil)

return &domain.ToolInvokeResponse{
    Status:     "succeeded",   // 直接返回成功状态
    ToolCallID: toolCallID,
    Result:     json.RawMessage(result),  // 直接包含结果
}, nil
```

**期望实现**:
```go
// 服务端工具异步执行
go s.executeServerToolAsync(context.Background(), toolCall, tool)

return &domain.ToolInvokeResponse{
    Status:     "pending",
    ToolCallID: toolCallID,
    Message:    "tool call created, use tool_call_id to poll result",
}, nil

// 新增异步执行方法
func (s *Service) executeServerToolAsync(ctx context.Context, toolCall *domain.ToolCall, tool *domain.Tool) {
    // 1. 更新状态为 RUNNING
    s.store.UpdateToolCallStatus(ctx, toolCall.ToolCallID, domain.ToolCallStatusRunning)

    // 2. 执行工具逻辑
    result, err := s.executeServerTool(ctx, tool.Name, toolCall.Args)

    // 3. 更新结果
    if err != nil {
        s.store.UpdateToolCallResult(ctx, toolCall.ToolCallID, domain.ToolCallStatusFailed, nil, errJSON)
    } else {
        s.store.UpdateToolCallResult(ctx, toolCall.ToolCallID, domain.ToolCallStatusSucceeded, result, nil)
    }
}
```

**修改位置**:
- `internal/service/tool.go` - 修改 `InvokeTool()` 中服务端工具的执行逻辑
- `internal/service/tool.go` - 添加 `executeServerToolAsync()` 方法

---

### 2.2 字段命名差异：Source vs Kind

**严重程度**: 🟡 低

**规划描述**: 文档使用 `Source` 和 `ToolSource` 命名。

**当前实现**: 使用 `Kind` 和 `ToolKind` 命名。

| 规划 | 实际 |
|------|------|
| `Source` | `Kind` |
| `ToolSourceServer` | `ToolKindServer` |
| `ToolSourceClient` | `ToolKindClient` |

**建议**: 由于语义相同，可保持现状，但需更新文档以保持一致性。或者修改代码以符合文档。

**修改位置** (如需修改):
- `internal/domain/tool.go`
- `internal/domain/enums.go`
- `internal/service/tool.go`
- `internal/repository/sqlite.go`

---

### 2.3 v1/tools.go 中存在未使用的 SubmitToolResult 方法

**严重程度**: 🟡 低

**问题描述**: `internal/transport/http/v1/tools.go` 中实现了 `SubmitToolResult` 方法，但未注册路由，属于死代码。

**当前状态**:
- v1/tools.go 有 SubmitToolResult 实现
- v1/handler.go 未注册该路由
- internalapi 中已有正确的实现

**建议**: 删除 v1/tools.go 中的 SubmitToolResult 方法，保持 API 边界清晰。

**修改位置**:
- `internal/transport/http/v1/tools.go` - 删除 SubmitToolResult 方法

---

### 2.4 响应格式差异

**严重程度**: 🟡 低

**规划的 InvokeTool 响应**:
```json
{
    "tool_call_id": "tc_xyz789",
    "status": "pending",
    "message": "tool call created, use tool_call_id to poll result"
}
```

**当前实现的响应**:
```go
type ToolInvokeResponse struct {
    Status     string          `json:"status"`
    ToolCallID string          `json:"tool_call_id"`
    Result     json.RawMessage `json:"result,omitempty"`
    Error      *ToolError      `json:"error,omitempty"`
    Reason     string          `json:"reason,omitempty"`  // 额外字段
}
```

**差异**:
- 使用 `Reason` 替代 `Message`
- 响应可能包含 `Result` 和 `Error`（同步执行时）

**建议**: 统一异步模型后，响应格式将自然对齐。可考虑添加 `Message` 字段或保持 `Reason`。

---

### 2.5 缺少独立的 tool_proxy.go 文件

**严重程度**: 🟡 低

**规划描述**: 文档规划 Tool Proxy 作为独立层存在于 `internal/service/tool_proxy.go`。

**当前实现**: 所有逻辑合并在 `internal/service/tool.go` 中。

**建议**: 可保持现状（功能已实现），或拆分以提高代码可读性。

---

## 三、实现优先级排序

| 优先级 | 任务 | 类型 |
|--------|------|------|
| P0 | 服务端工具改为异步执行 | 修复 |
| P0 | Tool 模型添加 Schema/ClientID 字段 | 新增 |
| P0 | POST /internal/tools/register API | 新增 |
| P0 | GET /v1/tools API | 新增 |
| P1 | Service 层 ListTools() 方法 | 新增 |
| P2 | 删除 v1/tools.go 中的死代码 | 清理 |
| P2 | 统一字段命名 Source/Kind | 可选 |
| P2 | 统一响应格式 | 可选 |

---

## 四、数据库迁移

如果添加 Schema 和 ClientID 字段，需要更新数据库表结构：

```sql
-- 添加新字段
ALTER TABLE tools ADD COLUMN schema TEXT;
ALTER TABLE tools ADD COLUMN client_id TEXT;

-- 添加索引
CREATE INDEX idx_tools_client ON tools(client_id);
```

或在 `internal/repository/sqlite.go` 的初始化逻辑中更新表创建语句。

---

## 五、测试验证

完成实现后，需验证以下场景：

1. **服务端工具异步执行**
   - 调用 `POST /v1/tools/weather.query/invoke`
   - 验证返回 `status: "pending"`
   - 轮询 `GET /v1/tool_calls/:id` 直到完成

2. **客户端工具注册**
   - 调用 `POST /internal/tools/register` 注册工具
   - 调用 `GET /v1/tools` 验证工具已注册

3. **工具列表查询**
   - 调用 `GET /v1/tools`
   - 验证返回服务端和客户端工具
