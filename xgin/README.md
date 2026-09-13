## XGin 模块

### 1. 模块简介

* 对 [Gin](https://github.com/gin-gonic/gin) 进行了封装，提供 Builder 模式构建 Web 服务
* 内置中间件：日志（Log）、链路追踪（Trace）、异常恢复（Recover）、会话（Session）、指标采集（Metric）
* 支持 HTTP/2 (H2C) 和 TLS (HTTPS)
* 集成 [Swagger](https://github.com/swaggo/gin-swagger) 文档
* 支持中文验证错误翻译
* 通过 `Start()` 启动，走完整生命周期（BeforeStart Hook → 服务 → 退出信号 → BeforeStop Hook）

> **`Start()` 是唯一的启动入口。** 服务本身的启停（`xserver.Server` 接口）由内部类型实现，
> 不对外暴露。它曾经是 `XGin` 上的公开方法 `Run()`——和 `Start()` 是英文同义词，
> 职责却完全不同：它不跑任何 Hook，只有在初始化完成之后调用才成立。选错的代价是隐形的：
> xconfig 未初始化时读配置不报错、只返回零值，于是服务照常起在默认端口上，
> 而日志、链路、数据库客户端一个都没配置。现在公开 API 上没有这个入口了。

### 2. 配置参数

```yaml
XGin:
  Host: "0.0.0.0"        # 服务监听地址 (optional, default "0.0.0.0")
  Port: 8000              # 服务端口号 (optional, default 8000)
  UseH2C: false         # 非 TLS 下启用 h2c (optional, default false)
  CertFile: ""            # TLS 证书路径 (optional, default ""，配置后自动启用 HTTPS)
  KeyFile: ""             # TLS 私钥路径 (optional, default "")

  # 超时。零值是"永不超时"，慢客户端可以一直占着连接不放，连接数打满后服务整体不可用
  ReadHeaderTimeout: "10s"  # 读取请求头超时 (optional, default "10s")，slowloris 的主要防线
  ReadTimeout: ""           # 读取整个请求超时 (optional, default 不限制)，限制会打断大文件上传
  WriteTimeout: ""          # 写响应超时 (optional, default 不限制)，限制会打断 SSE / 长轮询 / 大文件下载
  IdleTimeout: "60s"        # keep-alive 空闲超时 (optional, default "60s")
  GracefulStopTimeout: "25s" # 优雅退出超时 (optional, default "25s")

  Swagger: # Swagger 相关配置 (optional)
    Host: ""              # Swagger API Host (optional)
    BasePath: ""          # API 公共前缀 (optional)
    Title: ""             # API 标题 (optional)
    Description: ""       # API 描述 (optional)
    Schemes: # 支持的协议 (optional, default ["https", "http"])
      - "https"
      - "http"
```

`ReadTimeout` / `WriteTimeout` 默认不限制，因为一刀切会打断大文件上传、SSE 和长轮询；
需要时按业务实际上限配置。`ReadHeaderTimeout` 和 `IdleTimeout` 有默认值，
它们只约束"连上来却不发完整请求"和"发完了还占着连接"，不影响正常业务。

`GracefulStopTimeout` 应当**小于**部署环境的进程终止宽限期
（如 K8s `terminationGracePeriodSeconds`，默认 30s）。两者相等意味着 Shutdown
还没走完 pod 就被 SIGKILL，等于没有优雅退出。

启动时配置解析失败会直接返回错误、服务起不来 —— 而不是安静地回退到默认端口，
让服务起在一个没人预期的端口上。

### 3. 使用 demo

* 配置:

```yaml
XGin:
  Port: 8080
```

* 快捷启动:

```go
package main

import (
	"net/http"

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
			c.JSON(http.StatusOK, gin.H{"message": "pong"})
		})
	}).Build().Start()
}
```

* Builder 完整用法:

```go
package main

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/xiaoshicae/xone/v2/xgin"
	"github.com/xiaoshicae/xone/v2/xgin/options"
	"your-project/docs" // swag init 生成的文档
)

func main() {
	gx := xgin.New(
		options.EnableLogMiddleware(true),
		options.EnableTraceMiddleware(true),
		options.EnableZHTranslations(true),
		options.LogSkipPaths("/health", "/ready"),
	).WithRouteRegister(registerRoutes).
		WithSwagger(docs.SwaggerInfo).
		WithRecoverFunc(customRecoverFunc).
		Build()

	gx.Start()
}

func registerRoutes(e *gin.Engine) {
	e.GET("/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "pong"})
	})
}
```

* 启用 HTTPS:

```yaml
XGin:
  Port: 8443
  CertFile: "/path/to/cert.pem"
  KeyFile: "/path/to/key.pem"
```

* 启用 HTTP/2:

```yaml
XGin:
  UseH2C: true
```

### 4. API 说明

| 方法                            | 说明                             |
|-------------------------------|--------------------------------|
| `xgin.New(opts...)`           | 创建 XGin Builder                |
| `.WithRouteRegister(f...)`    | 注册路由                           |
| `.WithMiddleware(m...)`       | 注册自定义中间件                       |
| `.WithSwagger(spec, opts...)` | 注入 Swagger 文档                  |
| `.WithRecoverFunc(f)`         | 自定义 panic 恢复处理                 |
| `.Build()`                    | 构建 XGin 实例                     |
| `.Start()`                    | 启动服务（唯一的启动入口）                  |
| `.Engine()`                   | 获取底层 `*gin.Engine`（自动触发 Build） |

### 5. 内置中间件

| 中间件     | 说明                                  | 默认   |
|---------|-------------------------------------|------|
| Session | 注入请求会话信息                            | 始终启用 |
| Trace   | 链路追踪，生成 TraceID                     | 默认启用 |
| Recover | panic 恢复，防止服务崩溃                     | 始终启用 |
| Log     | 请求/响应日志记录                           | 默认启用 |
| Metric  | Prometheus 入站请求指标（请求数 + 耗时），需配合 xmetric | 默认启用 |

Metric 中间件采集指标：
- `http_requests_total{method, path, status}` — 入站请求总数
- `http_request_duration_seconds{method, path, status}` — 入站请求耗时（秒）

关闭方式：

```go
xgin.New(options.EnableMetricMiddleware(false)).Build()
```

#### 中间件顺序

洋葱模型，自外向内：

```
Session → Trace → Log → Metric → Recover → 用户中间件 → handler
```

**Recover 是框架中间件里最内层的一个**，这一点决定了 panic 请求能不能被观测到。
panic 一路向外抛，在哪一层被 recover 住，比它更内层的中间件里 `c.Next()` 之后的代码就都不执行 ——
若 Recover 在 Log / Metric 之外，handler panic 的请求既不写访问日志，也不计入
`http_requests_total`，而 panic 导致的 500 恰恰是最需要计入错误率的那一类。

进程安全不依赖这个顺序：`net/http` 对每个连接本就有兜底 recover，
框架中间件自身 panic 不会拖垮进程。

`/metrics` 端点刻意注册在用户中间件**之前**，因此不经过业务鉴权、限流等中间件 ——
否则采集器会被 401 挡在外面。需要保护该端点时，用 `options.MetricsPath` 换一个
不对外暴露的路径，或在网关层限制来源。Swagger 路由则注册在用户中间件之后，会经过它们。
