# XOne

开箱即用的 Golang 三方库集成框架。

一份 YAML 配置启用全部能力，日志 / 链路 / 指标自动串联，无需在业务代码里重复初始化。

<img src="doc/architecture.svg" alt="XOne 模块架构图" width="100%">

## 特性

- **统一集成三方库** —— 配置管理、日志、链路、指标、HTTP、数据库、缓存各有一套经过取舍的默认参数
- **配置驱动** —— 能力通过 YAML 开关，不写初始化代码
- **生命周期托管** —— 启动按依赖顺序初始化，退出按逆序关闭，启动失败自动回滚
- **链路自动关联** —— 日志带 TraceID，出站请求与 SQL 自动串进同一条链路
- **可扩展** —— Hook 机制接管启动与关闭，日志观察者、Span 处理器等扩展点对外开放

## 环境要求

Go >= 1.25

## 快速开始

### 1. 安装

```bash
go get github.com/xiaoshicae/xone/v2
```

> 模块路径带 `/v2`，所有 import 也必须带，例如 `github.com/xiaoshicae/xone/v2/xlog`。

### 2. 创建配置文件

在 `./`、`./conf/` 或 `./config/` 下放一个 `application.yml`：

```yaml
Server:
  Name: "my-service"
  Version: "v1.0.0"

XGin:
  Port: 8000

XLog:
  Level: "info"

XGorm:
  Driver: "mysql"
  DSN: "user:password@tcp(127.0.0.1:3306)/dbname?charset=utf8mb4&parseTime=True"
```

只配置需要的模块即可 —— 没有对应顶层 key 的模块会跳过初始化。
完整配置项见 [配置参考](./doc/configuration.md)。

### 3. 启动服务

```go
package main

import (
	"github.com/gin-gonic/gin"

	"github.com/xiaoshicae/xone/v2/xgin"
	"github.com/xiaoshicae/xone/v2/xgin/options"
)

func main() {
	xgin.New(
		options.EnableLogMiddleware(true),
		options.EnableTraceMiddleware(true),
	).WithRouteRegister(func(e *gin.Engine) {
		e.GET("/ping", func(c *gin.Context) {
			c.JSON(200, gin.H{"message": "pong"})
		})
	}).Build().Start()
}
```

`Start()` 会先按顺序初始化各模块（xconfig → xlog → 其余模块），再启动 HTTP 服务，
并接管 `SIGINT` / `SIGTERM` 做优雅退出。

## 模块清单

| 模块                             | 底层库                                                                 | 说明                               | Log | Trace |
|--------------------------------|---------------------------------------------------------------------|----------------------------------|-----|-------|
| [xconfig](./xconfig/README.md) | [viper](https://github.com/spf13/viper)                             | 配置管理（YAML + 环境变量 + Profile）      | -   | -     |
| [xlog](./xlog/README.md)       | [log/slog](https://pkg.go.dev/log/slog)（标准库）                    | 结构化日志（标准输出 + 可选文件轮转 + KV 注入）   | -   | -     |
| [xtrace](./xtrace/README.md)   | [opentelemetry](https://github.com/open-telemetry/opentelemetry-go) | 链路追踪（W3C + B3 传播格式）              | -   | -     |
| [xmetric](./xmetric/README.md) | [prometheus](https://github.com/prometheus/client_golang)           | Prometheus 指标采集（打点 + /metrics 端点） | -   | -     |
| [xflow](./xflow/README.md)     | -                                                                   | 流程编排（强弱依赖 + 自动回滚 + 监控）           | -   | -     |
| [xhttp](./xhttp/README.md)     | [go-resty](https://github.com/go-resty/resty)                       | HTTP 客户端（重试 + 连接池 + 出站指标）        | -   | ✅     |
| [xgorm](./xgorm/README.md)     | [gorm](https://gorm.io/)                                            | 数据库（MySQL / PostgreSQL，多数据源）     | ✅   | ✅     |
| [xredis](./xredis/README.md)   | [go-redis](https://github.com/redis/go-redis)                       | Redis（多实例 + 连接池指标）               | -   | ✅     |
| [xcache](./xcache/README.md)   | [ristretto](https://github.com/dgraph-io/ristretto)                 | 本地缓存（TTL + 泛型）                   | -   | -     |
| [xserver](./xserver/README.md) | -                                                                   | 服务运行与生命周期管理                      | -   | -     |
| [xgin](./xgin/README.md)       | [gin](https://github.com/gin-gonic/gin)                             | Gin Web 框架集成（Builder + 内置中间件）    | ✅   | ✅     |

基础层的 `xerror`（统一错误类型）、`xutil`（工具函数）、`xhook`（生命周期钩子）零第三方依赖，由上述模块内部使用。

## 常用能力

各模块的完整用法见上表中的 README 链接，这里只列最常用的调用方式。

**日志** —— 所有日志函数都要求传 `ctx`，TraceID 由此关联：

```go
import "github.com/xiaoshicae/xone/v2/xlog"

xlog.Info(ctx, "order created, amount=%d", 99)
xlog.Info(ctx, "request handled", xlog.KV("userId", "u-123"))

// 注入公共字段，后续日志自动携带
ctx = xlog.CtxWithKV(ctx, map[string]any{"requestId": "req-456"})
```

**HTTP 客户端**：

```go
import "github.com/xiaoshicae/xone/v2/xhttp"

resp, err := xhttp.RWithCtx(ctx).Get("https://api.example.com/users")

rawClient := xhttp.RawClient() // 原生 http.Client，用于 SSE 等流式场景
```

**数据库**（多数据源传 Name）：

```go
import "github.com/xiaoshicae/xone/v2/xgorm"

var user User
xgorm.CWithCtx(ctx).First(&user, 1)

masterDB := xgorm.CWithCtx(ctx, "master")
```

**Redis / 本地缓存**：

```go
import (
	"github.com/xiaoshicae/xone/v2/xcache"
	"github.com/xiaoshicae/xone/v2/xredis"
)

xredis.C().Set(ctx, "key", "value", time.Minute)

xcache.Set("user:1", user)
u, ok := xcache.Get[*User]("user:1")
```

**指标**：

```go
import "github.com/xiaoshicae/xone/v2/xmetric"

xmetric.CounterInc("order_created_total", xmetric.T("channel", "wechat"))
xmetric.ObserveDuration("db_query", elapsed, xmetric.T("table", "orders"))
defer xmetric.TrackInFlight("http_requests_in_flight")()
```

`/metrics` 端点由 xgin 自动注册（默认 `/metrics`）。

**自定义配置**：

```go
import "github.com/xiaoshicae/xone/v2/xconfig"

apiKey := xconfig.GetString("MyApp.ApiKey")

var cfg MyAppConfig
xconfig.UnmarshalConfig("MyApp", &cfg)
```

**链路追踪** —— xhttp / xgorm / xredis / xlog 会自动关联，一般无需手动操作：

```go
import "github.com/xiaoshicae/xone/v2/xtrace"

ctx, span := xtrace.GetTracer("my-service").Start(ctx, "my-operation")
defer span.End()
```

## 服务启动方式

```go
// 方式一：XGin 快捷启动（推荐）
xgin.New(opts...).WithRouteRegister(registerRoutes).Build().Start()

// 方式二：通过 xserver 启动，等价于方式一
xserver.Run(xgin.New(opts...).Build())

// 方式三：自定义 Server（实现 xserver.Server 接口）
xserver.Run(myServer)

// 方式四：无 HTTP 服务的常驻进程（consumer / job）
xserver.RunBlocking()

// 方式五：只初始化模块，不启动服务（调试用）
xserver.R()
```

TLS、HTTP/2、超时与优雅退出都通过 YAML 配置，见 [配置参考](./doc/configuration.md#常用配置项)。

## 生命周期 Hook

```go
import "github.com/xiaoshicae/xone/v2/xhook"

func init() {
	// 在所有框架模块初始化之后执行
	xhook.BeforeStart(func() error {
		return warmUpCache()
	})

	// 在框架模块关闭之前执行，此时日志仍可用
	xhook.BeforeStop(func() error {
		return flushBuffer()
	})
}
```

Hook 的执行顺序由 `Order` 决定，值越小越底层 —— 启动越早、关闭越晚。
`xconfig`（-100）与 `xlog`（-50）是框架保留层级，业务 Hook 保持默认值 100 即可，
这样它一定在配置就绪之后启动、在日志关闭之前结束。详见 [xserver README](./xserver/README.md)。

## 文档

- [配置参考](./doc/configuration.md) —— 配置文件位置、多环境、环境变量注入、IDE 补全、常用配置项
- [更新日志](./CHANGELOG.md)
- 各模块用法见上方[模块清单](#模块清单)中的链接
