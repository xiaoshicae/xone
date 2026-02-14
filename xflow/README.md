## XFlow 模块

### 1. 模块简介

XFlow 是 XOne 框架的流程编排模块，提供：
- 按顺序执行多个处理器（Processor），支持 Request/Response/临时上下文 三层数据模型
- 强依赖 / 弱依赖区分：强依赖失败中断流程并自动回滚，弱依赖失败跳过并继续
- 已成功处理器逆序回滚，Process / Rollback 均捕获 panic
- `ExecuteResult[Resp]` 自包含返回值，调用方无需回到原始变量取结果
- 可选 Monitor 监控，零开销关闭

### 2. 核心概念

| 概念 | 说明 |
|------|------|
| `FlowData[Req, Resp]` | 流程数据容器，包含 Request（入参）、Response（出参）和 Extra（临时数据） |
| `Processor[Req, Resp]` | 处理器接口，定义 `Process` 和 `Rollback` 方法 |
| `Flow[Req, Resp]` | 流程编排器，按序执行 Processor 列表 |
| `ExecuteResult[Resp]` | 执行结果，携带 Response 数据和错误信息 |
| `Dependency` | 依赖类型：`Strong`（强依赖）/ `Weak`（弱依赖） |
| `Monitor` | 监控接口，观测每步执行和回滚耗时 |

### 3. 三层数据模型

```
┌─────────────────────────────────────────┐
│           FlowData[Req, Resp]           │
├─────────────┬───────────────┬───────────┤
│  Request    │  Response     │  Extra    │
│  (入参)     │  (出参)       │  (临时)   │
│  语义不可变 │  Processor    │  Processor│
│             │  逐步填充     │  间传递   │
└─────────────┴───────────────┴───────────┘
```

- **Request**：调用方传入的入参，语义上不可变
- **Response**：Processor 逐步填充的出参，流程成功后通过 `result.Data` 返回
- **Extra**：Processor 间传递的临时数据，支持类型安全存取

### 4. 使用示例

#### 定义入参和出参

```go
package main

import (
    "context"
    "fmt"
    "github.com/xiaoshicae/xone/v2/xflow"
)

// 入参
type CreateOrderReq struct {
    UserID    int
    ProductID int
}

// 出参
type CreateOrderResp struct {
    OrderID string
    Total   float64
}
```

#### 定义处理器

```go
// 类型安全的临时数据键
var UserLevelKey = xflow.NewKey[string]("user_level")

// ValidateProcessor 校验处理器
type ValidateProcessor struct{}

func (p *ValidateProcessor) Name() string                { return "validate" }
func (p *ValidateProcessor) Dependency() xflow.Dependency { return xflow.Strong }

func (p *ValidateProcessor) Process(ctx context.Context, data *xflow.FlowData[CreateOrderReq, CreateOrderResp]) error {
    if data.Request.UserID == 0 {
        return fmt.Errorf("invalid user")
    }
    // 设置临时数据供下游使用
    xflow.SetExtra(data, UserLevelKey, "VIP")
    return nil
}

func (p *ValidateProcessor) Rollback(ctx context.Context, data *xflow.FlowData[CreateOrderReq, CreateOrderResp]) error {
    return nil
}

// PayProcessor 支付处理器
type PayProcessor struct{}

func (p *PayProcessor) Name() string                { return "pay" }
func (p *PayProcessor) Dependency() xflow.Dependency { return xflow.Strong }

func (p *PayProcessor) Process(ctx context.Context, data *xflow.FlowData[CreateOrderReq, CreateOrderResp]) error {
    // 读取上游临时数据
    level, _ := xflow.GetExtra(data, UserLevelKey)
    _ = level
    // 填充出参
    data.Response.OrderID = "ORD-123"
    data.Response.Total = 99.9
    return nil
}

func (p *PayProcessor) Rollback(ctx context.Context, data *xflow.FlowData[CreateOrderReq, CreateOrderResp]) error {
    // 退款逻辑...
    return nil
}
```

#### 构建并执行流程

```go
func main() {
    flow := xflow.New[CreateOrderReq, CreateOrderResp]("create-order",
        &ValidateProcessor{},
        &PayProcessor{},
    )

    result := flow.Execute(context.Background(), CreateOrderReq{UserID: 1, ProductID: 100})

    if result.Success() {
        fmt.Printf("订单创建成功: OrderID=%s, Total=%.2f\n", result.Data.OrderID, result.Data.Total)
    } else {
        fmt.Printf("订单创建失败: %v\n", result)
    }
}
```

#### 监控

监控默认开启，使用 xlog 打印日志。可通过 YAML 配置禁用：

```yaml
XFlow:
  DisableMonitor: true
```

自定义全局 Monitor 实现：

```go
xflow.SetDefaultMonitor(myMonitor)
```

### 5. 强依赖 vs 弱依赖

| 类型 | 失败行为 | 适用场景 |
|------|----------|----------|
| `Strong` | 中断流程，逆序回滚所有已成功的处理器 | 核心逻辑（支付、库存扣减） |
| `Weak` | 记录错误，继续执行后续处理器 | 非关键逻辑（发通知、记日志） |

```
[p1:Strong ✓] → [p2:Weak ✗ 跳过] → [p3:Strong ✓] → [p4:Strong ✗ 中断]
                                                         ↓
                                          回滚: p3 → p2 → p1（逆序）
```

### 6. 执行结果

```go
result := flow.Execute(ctx, req)

result.Success()          // 是否成功（无强依赖失败）
result.Data               // 自包含的 Response 数据（类型安全）
result.Err                // 致命错误（强依赖失败）
result.Rolled             // 是否触发了回滚
result.IsRolled()         // 同上（ResultSummary 接口方法）
result.SkippedErrors      // 弱依赖跳过的错误列表
result.RollbackErrors     // 回滚过程中的错误列表
result.HasSkippedErrors() // 是否存在弱依赖错误
result.HasRollbackErrors()// 是否存在回滚错误
```

### 7. 临时数据传递

Processor 间传递临时数据有两种方式：

```go
// 方式一：非类型安全（any 类型）
data.Set("key", "value")
v, ok := data.Get("key")

// 方式二：类型安全（推荐）
var LevelKey = xflow.NewKey[string]("level")
xflow.SetExtra(data, LevelKey, "VIP")
level, ok := xflow.GetExtra(data, LevelKey)  // level 自动推导为 string
```

### 8. 注意事项

- `Process` 和 `Rollback` 中的 panic 会被自动捕获，转为错误返回
- 弱依赖失败后也会加入回滚列表，后续强依赖失败时会一并回滚
- `Monitor` 默认开启（使用 xlog 打印），可通过配置 `DisableMonitor: true` 关闭（零开销）
- `Flow` 的字段赋值非并发安全，必须在 `Execute` 前完成
- 流程失败时 `result.Data` 保持零值，成功时自动从 `FlowData.Response` 填充
