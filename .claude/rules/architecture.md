# 架构规范

## 模块分层

- **基础层**：xutil、xhook（无外部依赖）
- **核心层**：xconfig、xtrace、xlog（依赖基础层）
- **服务层**：xhttp、xgorm（依赖核心层）
- **生命周期层**：xserver（Server 接口 + 信号处理）
- **应用层**：xgin（依赖 xserver + 核心层/服务层配置）

上层可以依赖下层，下层不得依赖上层。同层模块之间无直接编译依赖。

## 全局状态管理

```go
// 单实例存储（推荐）
var (
    defaultClient *Client
    clientMu      sync.RWMutex
)

// 对外查询 API
func C() *Client {
    clientMu.RLock()
    defer clientMu.RUnlock()
    return defaultClient
}

// 带 Context 版本（仅当底层 client 需要在获取时绑定 context 才提供）
// 例：xgorm 需要 CWithCtx，因为 GORM 要 db.WithContext(ctx)
// 例：xredis 不需要，因为 redis.Client 每个操作方法本身接受 ctx
func CWithCtx(ctx context.Context) *Client { ... }
```

## 配置结构体模式

```go
const XModuleConfigKey = "XModule"

type Config struct {
    Timeout string `mapstructure:"Timeout"`
}

func configMergeDefault(c *Config) *Config {
    if c == nil {
        c = &Config{}
    }
    // 时间配置使用 xutil.ToDuration()，不用 cast.ToDuration()
    // 默认值使用 xutil.GetOrDefault()
    return c
}
```

## 错误处理

- 优先使用 `xerror.XOneError` 统一错误类型，禁止直接使用 `fmt.Errorf`
- 创建错误：`xerror.New(module, op, err)` 或 `xerror.Newf(module, op, format, args...)`
- 判断模块错误：`xerror.Is(err, "xconfig")`
- 提取模块名：`xerror.Module(err)`
- 不要忽略错误，必须处理或向上传递
- 使用 `errors.Is()` 和 `errors.As()` 进行错误判断
- 关键操作失败时记录日志：`xutil.ErrorIfEnableDebug()`

## xhook 使用规范

### 执行顺序机制

XOne 各模块通过 `init()` 函数调用 `xhook.BeforeStart()` / `xhook.BeforeStop()` 注册 Hook。Go 的 import 机制保证了 `init()` 按包导入顺序依次执行，因此 Hook 的注册顺序由**用户 import 各模块的顺序**决定。

对于相同 Order 值的 Hook，`xhook` 使用**稳定排序**（`slices.SortStableFunc`），即不改变注册时的相对顺序。这意味着只要 import 顺序一致，执行顺序就是确定的。

**BeforeStart 正序执行，BeforeStop 反序执行**，确保与启动顺序对称（LIFO）。后初始化的模块先关闭，先初始化的模块最后关闭。

### 不推荐使用 Order

`xhook.Order()` 选项虽然可用，但**不推荐普通模块使用**，应保持默认值（100）。原因：

1. **依赖 import 顺序更直观**：Go 开发者天然理解 import 顺序，而显式 Order 值分散在各模块中，难以全局把控
2. **避免 Order 冲突**：多个模块各自声明 Order 值，容易产生冲突或不一致
3. **框架内部已有保障**：xconfig 使用 `Order(1)` 确保最先初始化，其余模块无需干预顺序

```go
// 正确 - 使用默认 Order，依靠 import 顺序
func init() {
    xhook.BeforeStart(initXLog)
    xhook.BeforeStop(closeXLog)
}

// 不推荐 - 除 xconfig 外，其他模块不应使用 Order
func init() {
    xhook.BeforeStart(initXLog, xhook.Order(30))
}
```

### 用户侧控制顺序的方式

用户在 `main.go` 中通过 import 顺序控制各模块的初始化顺序：

```go
import (
    _ "github.com/xiaoshicae/xone/v2/xconfig" // 1. 配置（Order=1，始终最先）
    _ "github.com/xiaoshicae/xone/v2/xtrace"  // 2. 链路追踪
    _ "github.com/xiaoshicae/xone/v2/xlog"    // 3. 日志
    _ "github.com/xiaoshicae/xone/v2/xhttp"   // 4. HTTP 客户端
    _ "github.com/xiaoshicae/xone/v2/xgorm"   // 5. 数据库
)
```

BeforeStop 自动反序执行，无需额外配置：

```
BeforeStart 执行顺序：xconfig → xtrace → xlog → xhttp → xgorm
BeforeStop  执行顺序：xgorm → xhttp → xlog → xtrace → xconfig
```

## 新增模块指南

1. 创建 `x{模块名}/` 目录
2. 必须包含文件：`config.go`、`client.go`、`x{模块名}_init.go`、`x{模块名}_test.go`、`README.md`
3. 配置 key 统一为 `X{模块名}ConfigKey = "X{模块名}"`
4. 在 `init()` 中通过 `xhook.BeforeStart()` / `xhook.BeforeStop()` 注册 Hook
5. 初始化函数先检查 `xconfig.ContainKey(key)`，无配置则跳过

## xflow 使用规范

### Process 与 Rollback 对称原则

每个 Processor 的 `Rollback()` 必须与 `Process()` 放在同一个结构体中，保持正向逻辑和回滚逻辑的对称性。**谁做的事，谁负责回滚**。

```go
// 正确 - 扣券逻辑和回滚逻辑在同一个 Processor 中
type DeductCouponProcessor struct{}

func (p *DeductCouponProcessor) Name() string             { return "扣券" }
func (p *DeductCouponProcessor) Dependency() xflow.Dependency { return xflow.Strong }

func (p *DeductCouponProcessor) Process(ctx context.Context, data *OrderData) error {
    // 正向：扣减优惠券
    return deductCoupon(ctx, data.CouponID)
}

func (p *DeductCouponProcessor) Rollback(ctx context.Context, data *OrderData) error {
    // 回滚：归还优惠券
    return returnCoupon(ctx, data.CouponID)
}
```

### 回滚触发时机

当某个**强依赖** Processor 的 `Process()` 失败时，xflow 会**逆序回滚**所有已成功执行的 Processor（包括弱依赖）：

```
扣券(Strong) → 扣库存(Strong) → 扣款(Strong) → 发通知(Weak)
                                   ↑ 失败
回滚顺序：扣库存.Rollback() → 扣券.Rollback()
```

- 强依赖失败 → 中断流程，逆序回滚所有已成功的 Processor
- 弱依赖失败 → 跳过错误继续执行，但失败的弱依赖也会被纳入回滚列表

### 设计要点

- **Rollback 不能假设 Process 完全成功**：弱依赖 Process 失败后仍可能被回滚，Rollback 中应做幂等处理
- **Rollback 失败不中断回滚流程**：单个 Rollback 出错会记录到 `RollbackErrors`，但不会阻止其余 Processor 回滚
- **共享数据用指针类型**：`Flow[T]` 的泛型参数建议使用指针（如 `*OrderData`），确保各 Processor 间数据可共享修改

## xserver 包

```go
// Server 接口
type Server interface {
    Run() error
    Stop() error
}

// 启动方式
xserver.Run(server)      // 启动 Server，阻塞等待退出信号
xserver.RunBlocking()    // 启动阻塞式 Server（consumer/job 服务）
xserver.R()              // 仅执行 BeforeStart hook（调试用）
```

## xgin 包

```go
// XGin Builder（支持中间件、Swagger、HTTP/2、TLS）
gx := xgin.New(
    options.EnableLogMiddleware(true),
    options.EnableTraceMiddleware(true),
).
    WithRouteRegister(register).
    WithMiddleware(customMiddleware).
    WithRecoverFunc(customRecoveryFunc).
    WithSwagger(docs.SwaggerInfo, options.SwaggerUrlPrefix("/api")).
    Build()

// 启动方式一：通过 xserver.Run（推荐，gx 实现了 xserver.Server 接口）
xserver.Run(gx)

// 启动方式二：快捷启动（内部调用 xserver.Run）
gx.Start()

// 获取原始 gin.Engine（自动调用 Build）
engine := gx.Engine()

// TLS 和 HTTP/2 通过 YAML 配置启用（XGin.CertFile / XGin.KeyFile / XGin.UseH2C）
```