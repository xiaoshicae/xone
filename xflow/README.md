## XFlow 模块

### 1. 模块简介

XFlow 是 XOne 框架的流程编排模块，提供：

- 按顺序执行多个处理器（Processor），一个共享数据结构贯穿全程
- 强依赖 / 弱依赖区分：强依赖失败中断流程并自动回滚，弱依赖失败跳过并继续
- 已执行的处理器逆序回滚，Process / Rollback 均捕获 panic
- **回滚不受调用方 context 取消影响**，补偿逻辑在请求超时后依然能执行
- 可选 Monitor 监控，关闭时零开销

### 2. 核心概念

| 概念 | 说明 |
|------|------|
| `Flow[T]` | 流程编排器，按序执行 Processor 列表 |
| `Processor[T]` | 处理器接口，定义 `Process` 和 `Rollback` |
| `T` | 贯穿全程的共享数据，建议用指针（如 `*OrderData`） |
| `ExecuteResult` | 执行结果（非泛型），只携带错误与回滚信息 |
| `Dependency` | 依赖类型：`Strong`（强依赖）/ `Weak`（弱依赖） |
| `Monitor` | 监控接口，观测每步执行和回滚耗时 |

### 3. 数据模型：一个结构体装下全部

入参、出参与处理器间的中间数据都放进同一个结构体，由调用方持有：

```go
type OrderData struct {
    // 入参
    UserID    int
    ProductID int

    // 出参
    OrderID string
    Amount  int

    // 处理器间的中间数据，就是普通字段
    CouponID string
}
```

这样处理器的方法签名里**只出现业务自己的类型**，不必重复框架的泛型类型。中间数据是编译期类型安全的普通字段，不需要 map 存取和类型断言。

数据由调用方持有还带来一个好处：**流程失败时，此前处理器已写入的内容依然保留**，便于排查与补偿。

### 4. 使用示例

```go
package main

import (
    "context"
    "fmt"

    "github.com/xiaoshicae/xone/v2/xflow"
)

type OrderData struct {
    UserID   int
    CouponID string
    OrderID  string
}

// 扣券：Process 与 Rollback 成对放在同一个结构体中
type DeductCoupon struct{}

func (p *DeductCoupon) Name() string                 { return "扣券" }
func (p *DeductCoupon) Dependency() xflow.Dependency { return xflow.Strong }

func (p *DeductCoupon) Process(ctx context.Context, d *OrderData) error {
    d.CouponID = "coupon-001"
    return nil
}

func (p *DeductCoupon) Rollback(ctx context.Context, d *OrderData) error {
    // 归还优惠券，需保证幂等
    return nil
}

// 发通知：弱依赖，失败不影响主流程
type SendNotice struct{}

func (p *SendNotice) Name() string                 { return "发通知" }
func (p *SendNotice) Dependency() xflow.Dependency { return xflow.Weak }
func (p *SendNotice) Process(ctx context.Context, d *OrderData) error  { return nil }
func (p *SendNotice) Rollback(ctx context.Context, d *OrderData) error { return nil }

func main() {
    flow := xflow.New("创建订单", &DeductCoupon{}, &SendNotice{})

    data := &OrderData{UserID: 1}
    result := flow.Execute(context.Background(), data)

    if !result.Success() {
        fmt.Println(result) // flow failed: ..., rolled back
        return
    }
    fmt.Println(data.OrderID, result.HasSkippedErrors())
}
```

### 5. 执行语义

**强依赖失败** → 中断流程，逆序回滚所有已执行的处理器（含失败的弱依赖）：

```
扣券(Strong) → 扣库存(Strong) → 扣款(Strong) → 发通知(Weak)
                                   ↑ 失败
回滚顺序：扣库存.Rollback() → 扣券.Rollback()
```

**弱依赖失败** → 记入 `SkippedErrors` 后继续执行，但同样纳入回滚范围（所以 Rollback 必须做幂等，不能假设 Process 完全成功）。

**回滚自身失败** → 记入 `RollbackErrors`，不中断其余处理器的回滚。

### 6. context 语义

| 阶段 | 使用的 context |
|------|----------------|
| `Process` | 调用方传入的 ctx。ctx 被取消后不再启动新的处理器，已执行的部分照常回滚 |
| `Rollback` | **剥离了取消与超时**的 ctx（`context.WithoutCancel`），只保留其中的 value |

补偿逻辑（退款、还库存、解冻额度）最需要执行的时机恰恰是请求超时之后。沿用已取消的 context 会让每个补偿调用一进去就被拒绝，资源就真的漏掉了 —— 所以回滚改用独立的 context，由 `XFlow.RollbackTimeout` 单独限时。

回滚预算耗尽时，未补偿的处理器会**逐个记入 `RollbackErrors`**，调用方据此知道哪些资源还悬着。

### 7. 配置参数

```yaml
XFlow:
  EnableMonitor: true       # 是否开启流程监控 (optional, default true)
  RollbackTimeout: "30s"    # 回滚全部处理器的总超时 (optional, default "30s")
```

### 8. 监控

默认 Monitor 用 xlog 打印每步执行与回滚。可替换为自定义实现：

```go
xflow.SetDefaultMonitor(myMonitor)  // 传 nil 等同于关闭监控
```

各回调均被 panic 隔离 —— 监控实现出错只丢一次观测，不会打断业务流程。

### 9. 注意事项

- `xflow.New` 传入 nil Processor 会直接 panic，不留到执行时才空指针
- `Flow` 构建后字段不再变化，可被并发 `Execute`；共享数据 `T` 由每次调用各自传入，互不干扰
- `Rollback` 必须幂等：弱依赖 Process 失败后仍会被回滚
