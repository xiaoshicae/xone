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

## 新增模块指南

1. 创建 `x{模块名}/` 目录
2. 必须包含文件：`config.go`、`client.go`、`x{模块名}_init.go`、`x{模块名}_test.go`、`README.md`
3. 配置 key 统一为 `X{模块名}ConfigKey = "X{模块名}"`
4. 在 `init()` 中通过 `xhook.BeforeStart()` / `xhook.BeforeStop()` 注册 Hook
5. 初始化函数先检查 `xconfig.ContainKey(key)`，无配置则跳过

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