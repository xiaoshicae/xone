## XGin 模块

### 1. 模块简介

* 对 [Gin](https://github.com/gin-gonic/gin) 进行了封装，提供 Builder 模式构建 Web 服务
* 内置中间件：日志（Log）、链路追踪（Trace）、异常恢复（Recover）、会话（Session）
* 支持 HTTP/2 (H2C) 和 TLS (HTTPS)
* 集成 [Swagger](https://github.com/swaggo/gin-swagger) 文档
* 支持中文验证错误翻译
* 实现 `xserver.Server` 接口，通过 `Start()` 或 `xserver.Run()` 启动

### 2. 配置参数

```yaml
XGin:
  Host: "0.0.0.0"        # 服务监听地址 (optional, default "0.0.0.0")
  Port: 8000              # 服务端口号 (optional, default 8000)
  UseH2C: false         # 非 TLS 下启用 h2c (optional, default false)
  CertFile: ""            # TLS 证书路径 (optional, default ""，配置后自动启用 HTTPS)
  KeyFile: ""             # TLS 私钥路径 (optional, default "")
  Swagger: # Swagger 相关配置 (optional)
    Host: ""              # Swagger API Host (optional)
    BasePath: ""          # API 公共前缀 (optional)
    Title: ""             # API 标题 (optional)
    Description: ""       # API 描述 (optional)
    Schemes: # 支持的协议 (optional, default ["https", "http"])
      - "https"
      - "http"
```

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
	"github.com/xiaoshicae/xone/v2/xserver"
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

	xserver.Run(gx)
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
| `.Start()`                    | 快捷启动（等价于 `xserver.Run(gx)`）    |
| `.Engine()`                   | 获取底层 `*gin.Engine`（自动触发 Build） |

### 5. 内置中间件

| 中间件     | 说明              | 默认   |
|---------|-----------------|------|
| Session | 注入请求会话信息        | 始终启用 |
| Trace   | 链路追踪，生成 TraceID | 默认启用 |
| Recover | panic 恢复，防止服务崩溃 | 始终启用 |
| Log     | 请求/响应日志记录       | 默认启用 |
