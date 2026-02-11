## XFlow 模块

### 1. 模块简介

XFlow 是 XOne 框架的流程编排模块，提供：
- 按顺序执行多个处理器（Processor），支持泛型数据传递
- 强依赖 / 弱依赖区分：强依赖失败中断流程并自动回滚，弱依赖失败跳过并继续
- 已成功处理器逆序回滚，Process / Rollback 均捕获 panic
- 可选 Monitor 监控，零开销关闭

### 2. 核心概念

| 概念 | 说明 |
|------|------|
| `Processor[T]` | 处理器接口，定义 `Process` 和 `Rollback` 方法 |
| `Flow[T]` | 流程编排器，按序执行 Processor 列表 |
| `Dependency` | 依赖类型：`Strong`（强依赖）/ `Weak`（弱依赖） |
| `Monitor` | 监控接口，观测每步执行和回滚耗时 |
| `ExecuteResult` | 执行结果，包含致命错误、跳过错误、回滚错误 |

### 3. 使用示例

#### 定义处理器

```go
package main

import (
    "context"
    "fmt"
    "github.com/xiaoshicae/xone/v2/xflow"
)

// OrderData 流程共享数据（使用指针类型以支持处理器间数据传递）
type OrderData struct {
    OrderID   string
    Validated bool
    Paid      bool
}

// ValidateProcessor 校验处理器
type ValidateProcessor struct{}

func (p *ValidateProcessor) Name() string              { return "validate" }
func (p *ValidateProcessor) Dependency() xflow.Dependency { return xflow.Strong }

func (p *ValidateProcessor) Process(ctx context.Context, data *OrderData) error {
    // 校验逻辑...
    data.Validated = true
    return nil
}

func (p *ValidateProcessor) Rollback(ctx context.Context, data *OrderData) error {
    data.Validated = false
    return nil
}

// PayProcessor 支付处理器
type PayProcessor struct{}

func (p *PayProcessor) Name() string              { return "pay" }
func (p *PayProcessor) Dependency() xflow.Dependency { return xflow.Strong }

func (p *PayProcessor) Process(ctx context.Context, data *OrderData) error {
    // 支付逻辑...
    data.Paid = true
    return nil
}

func (p *PayProcessor) Rollback(ctx context.Context, data *OrderData) error {
    // 退款逻辑...
    data.Paid = false
    return nil
}
```

#### 构建并执行流程

```go
func main() {
    flow := xflow.New[*OrderData]("create-order",
        &ValidateProcessor{},
        &PayProcessor{},
    )

    data := &OrderData{OrderID: "ORD-001"}
    result := flow.Execute(context.Background(), data)

    if result.Success() {
        fmt.Printf("订单创建成功: %+v\n", data)
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

也可为单个 Flow 设置独立的 Monitor：

```go
flow := xflow.New[*OrderData]("create-order",
    &ValidateProcessor{},
    &PayProcessor{},
)
flow.SetMonitor(myMonitor)

result := flow.Execute(context.Background(), data)
```

### 4. 强依赖 vs 弱依赖

| 类型 | 失败行为 | 适用场景 |
|------|----------|----------|
| `Strong` | 中断流程，逆序回滚所有已成功的处理器 | 核心逻辑（支付、库存扣减） |
| `Weak` | 记录错误，继续执行后续处理器 | 非关键逻辑（发通知、记日志） |

```
[p1:Strong ✓] → [p2:Weak ✗ 跳过] → [p3:Strong ✓] → [p4:Strong ✗ 中断]
                                                         ↓
                                          回滚: p3 → p2 → p1（逆序）
```

### 5. 执行结果

```go
result := flow.Execute(ctx, data)

result.Success()          // 是否成功（无强依赖失败）
result.Err                // 致命错误（强依赖失败）
result.Rolled             // 是否触发了回滚
result.SkippedErrors      // 弱依赖跳过的错误列表
result.RollbackErrors     // 回滚过程中的错误列表
result.HasSkippedErrors() // 是否存在弱依赖错误
result.HasRollbackErrors()// 是否存在回滚错误
```

### 6. 数据传递

处理器间共享可变数据时，泛型参数 `T` 应使用**指针类型**：

```go
// ✅ 使用指针类型，处理器可修改共享数据
flow := xflow.New[*OrderData]("order-flow", ...)
data := &OrderData{}
flow.Execute(ctx, data)
// data 包含所有处理器的修改

// ⚠️ 使用值类型，处理器内修改不会传递到后续处理器
flow := xflow.New[OrderData]("order-flow", ...)
```

### 7. 注意事项

- `Process` 和 `Rollback` 中的 panic 会被自动捕获，转为错误返回
- 弱依赖失败后也会加入回滚列表，后续强依赖失败时会一并回滚
- `Monitor` 默认开启（使用 xlog 打印），可通过配置 `DisableMonitor: true` 关闭（零开销）
- `Flow` 的配置方法（`AddProcessor`、`SetMonitor` 等）非并发安全，必须在 `Execute` 前完成