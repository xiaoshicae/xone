# 架构规范

## 模块分层

- **基础层**：xerror、xutil、xhook（**零第三方依赖**，只用标准库）
- **核心层**：xconfig、xtrace、xlog（依赖基础层）
- **服务层**：xhttp、xgorm（依赖核心层）
- **生命周期层**：xserver（Server 接口 + 信号处理）
- **应用层**：xgin（依赖 xserver + 核心层/服务层配置）

上层可以依赖下层，下层不得依赖上层。同层模块之间无直接编译依赖。

### 基础层的零依赖约束

xerror / xutil / xhook 只要 import 了 xone 基本都会被编进去，因此它们**不得引入任何第三方依赖**。
需要下层用到上层能力时，用「上层注入、下层持有扩展点」的方式，而不是让下层 import 上层：

| 扩展点 | 注入方 | 作用 |
|--------|--------|------|
| `xutil.SetTraceContextExtractor` | xtrace | 让日志/指标能读到 TraceID，而基础层不必依赖 OpenTelemetry |
| `xlog.AddObserver` | xmetric、xgin | 观察日志事件，而不必依赖全局日志实例 |
| `xtrace.AddSpanProcessor` | 业务服务 | 自带 exporter 上报 Span，框架不引入 OTLP/gRPC |

新增基础层代码前跑一遍：

```bash
# 期望输出为空
go list -deps ./xerror ./xutil ./xhook | grep -E "^[^/]*\." | grep -v xiaoshicae
```

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

详细规范见 `config-conventions.md`。基本骨架：

```go
const XModuleConfigKey = "XModule"

type Config struct {
    Timeout string `mapstructure:"Timeout"`
    Enable  *bool  `mapstructure:"Enable"` // 默认 true 用 *bool
}

func configMergeDefault(c *Config) *Config {
    if c == nil { c = &Config{} }
    if c.Timeout == "" { c.Timeout = "60s" }
    if c.Enable == nil { c.Enable = xutil.ToPtr(true) }
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

XOne 各模块通过 `init()` 函数调用 `xhook.BeforeStart()` / `xhook.BeforeStop()` 注册 Hook，因此注册顺序就是包的初始化顺序。

**Go 的 init 顺序是「拓扑排序（被依赖的包先）+ 就绪集合内按 import path 字典序」，与 import 的书写顺序无关**（`gofmt` 本来也会重排同组 import）。所以不要试图靠调整 import 顺序控制生命周期顺序。xone 各模块之间顺序正确，靠的是真实依赖边：每个模块都 import xconfig，xgorm / xhttp / xtrace 都 import xlog。

对于相同 Order 值的 Hook，`xhook` 使用**稳定排序**（`slices.SortStableFunc`），不改变注册时的相对顺序。

`Order` 表示**资源层级**而非阶段内优先级：值越小越底层，BeforeStart 越先执行、BeforeStop 越后执行。BeforeStop 是 BeforeStart 顺序的整体镜像，同 Order 内再按注册顺序逆序。这样一个资源只需声明一个 Order 就能做到「先启动、后关闭」。

**BeforeStart 正序执行，BeforeStop 反序执行**，确保与启动顺序对称（LIFO）。后初始化的模块先关闭，先初始化的模块最后关闭。

### 不推荐普通模块使用 Order

`xhook.Order()` 选项虽然可用，但**普通模块与业务资源应保持默认值（100）**。原因：

1. **越少越好把控**：Order 值分散在各模块中，声明得越多越难全局把控
2. **避免 Order 冲突**：多个模块各自声明 Order 值，容易产生冲突或不一致
3. **全部默认时行为即是所需**：所有模块保持默认 Order 时，行为恰好是「启动按 init 顺序、关闭按其逆序」

**Order 分两个区间，且是强制隔离的：**

| 区间 | 成员 | Order |
|------|------|-------|
| 框架保留区（负值） | xconfig / xlog | -100 / -50 |
| 业务区（>= 0） | 其余模块与用户资源 | 默认 100 |

`xhook.Order(n)` 对 `n < 0` 直接 panic；负值只能经 `xhook.ReservedOrder` 设置，其参数类型在 `internal/hookorder` 中，外部模块 import 会编译失败。所以业务 Hook 不可能先于 xconfig 启动、也不可能晚于 xlog 关闭。

这两层都不可省：Go 按 import path 字典序决定 init 顺序，用户模块叫 `acme/...` 还是 `myapp/...` 就决定了它排在 xone 之前还是之后，使用者无法通过 import 纪律控制。没有配置层级，路径靠前的用户包会在配置加载完成前执行 BeforeStart；没有日志层级，它会在日志写入器关闭之后才关闭，其关闭日志直接丢失。

```go
// 正确 - 使用默认 Order，依靠 import 顺序
func init() {
    xhook.BeforeStart(initXLog)
    xhook.BeforeStop(closeXLog)
}

// 不推荐 - 除框架保留层级外，其他模块不应使用 Order
func init() {
    xhook.BeforeStart(initModule, xhook.Order(30))
}
```

### 用户侧

用户在 `main.go` 中按需匿名 import 各模块即可，**书写顺序不影响执行顺序**：

```go
import (
    _ "github.com/xiaoshicae/xone/v3/xconfig" // 保留层级 -100，最先启动
    _ "github.com/xiaoshicae/xone/v3/xlog"    // 保留层级 -50，次先启动、最后关闭
    _ "github.com/xiaoshicae/xone/v3/xtrace"
    _ "github.com/xiaoshicae/xone/v3/xhttp"
    _ "github.com/xiaoshicae/xone/v3/xgorm"
)
```

BeforeStop 自动反序执行，无需额外配置：

```
BeforeStart 执行顺序：xconfig → xlog → xtrace → xhttp → xgorm
BeforeStop  执行顺序：xgorm → xhttp → xtrace → xlog → xconfig
```

用户自己的资源保持默认 Order 即可：它一定在 xlog 之前关闭，关闭逻辑里可以放心打日志。

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

- **共享数据用指针类型**：`Flow[T]` 只有一个泛型参数，建议使用指针（如 `*OrderData`）。入参、出参与各 Processor 间的中间数据都放进这一个结构体，Processor 的方法签名里因此只出现业务自己的类型，不必重复框架的泛型类型
- **Rollback 不能假设 Process 完全成功**：弱依赖 Process 失败后仍可能被回滚，Rollback 中应做幂等处理
- **Rollback 失败不中断回滚流程**：单个 Rollback 出错会记录到 `RollbackErrors`，但不会阻止其余 Processor 回滚
- **流程失败时 data 中已写入的内容依然保留**：数据由调用方持有，便于排查与补偿

### context 语义

- **Process 使用调用方的 context**，`ctx` 被取消后不再启动新的 Processor，已执行的部分照常回滚
- **Rollback 使用剥离了取消与超时的 context**（`context.WithoutCancel`），只保留其中的 value。
  补偿逻辑（退款、还库存、解冻额度）最需要执行的时机恰恰是请求超时之后，沿用已取消的 context
  会让每个补偿调用一进去就被拒绝，资源就真的漏掉了
- **回滚由 `XFlow.RollbackTimeout` 单独限时**（默认 30s），预算耗尽时未补偿的 Processor 会逐个记入
  `RollbackErrors`，调用方据此知道哪些资源还悬着

### 其它约束

- `xflow.New` 传入 nil Processor 直接 panic，不留到执行时才空指针
- `Monitor` 的各回调均被 panic 隔离，监控实现出错只丢一次观测，不会打断业务流程
- `Flow` 构建后字段不再变化，可被并发 `Execute`

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
xserver.Init()           // 仅执行 BeforeStart hook（调试用）
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

// 启动（唯一入口，内部走 xserver.Run）
// 服务本身的启停由内部类型实现 xserver.Server，不挂在 XGin 上：
// 那会让「跳过 BeforeStart 直接起服务」重新变成一次方法调用的距离
gx.Start()

// 获取原始 gin.Engine（自动调用 Build）
engine := gx.Engine()

// TLS 和 HTTP/2 通过 YAML 配置启用（XGin.CertFile / XGin.KeyFile / XGin.UseH2C）
```