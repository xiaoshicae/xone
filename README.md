# XOne

开箱即用的 Golang 三方库集成 SDK

## 功能特性

- 统一集成三方包，降低维护成本
- 通过配置启用能力，开箱即用
- 提供最佳实践的默认参数配置
- 支持 Hook 机制，灵活扩展
- 集成 OpenTelemetry 链路追踪

## 环境要求

- Go >= 1.24

## 快速开始

### 1. 安装

```bash
go get github.com/xiaoshicae/xone
```

### 2. 配置文件

创建 `application.yml`（支持放置在 `./`、`./conf/`、`./config/` 目录下）：

```yaml
Server:
  Name: "my-service"
  Version: "v1.1.5"
  Profiles:
    Active: "dev"
  Gin:
    Port: 8000

XLog:
  Level: "info"
  Console: true

XGorm:
  Driver: "mysql"
  DSN: "user:password@tcp(127.0.0.1:3306)/dbname?charset=utf8mb4&parseTime=True"

XHttp:
  Timeout: "30s"
  MaxIdleConns: 100
  MaxIdleConnsPerHost: 10

XCache:
  MaxCost: 100000
  DefaultTTL: "5m"
```

### 3. 启动服务

```go
package main

import (
	"github.com/gin-gonic/gin"
	"github.com/xiaoshicae/xone"
)

func main() {
	engine := gin.Default()

	engine.GET("/ping", func(c *gin.Context) {
		c.JSON(200, gin.H{"message": "pong"})
	})

	// 启动服务，自动初始化所有模块
	xone.RunGin(engine)
}
```

### 4. 使用模块

```go
package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/xiaoshicae/xone/xgorm"
	"github.com/xiaoshicae/xone/xhttp"
	"github.com/xiaoshicae/xone/xconfig"
	"github.com/xiaoshicae/xone/xlog"
)

func Handler(c *gin.Context) {
	ctx := c.Request.Context()

	// 数据库操作（推荐使用 CWithCtx 传递 context）
	var user User
	xgorm.CWithCtx(ctx).First(&user, 1)

	// HTTP 请求（推荐使用 RWithCtx 传递 context）
	resp, _ := xhttp.RWithCtx(ctx).Get("https://api.example.com/data")

	// 日志记录
	xlog.Info(ctx, "request handled, user_id=%d", user.ID)

	// 读取自定义配置
	customValue := xconfig.GetString("MyConfig.Key")
}
```

## 模块清单

| 模块      | 底层库                                                                 | 文档                            | Log | Trace | 说明                    |
|---------|---------------------------------------------------------------------|-------------------------------|-----|-------|-----------------------|
| xconfig | [viper](https://github.com/spf13/viper)                             | [README](./xconfig/README.md) | -   | -     | 配置管理                  |
| xlog    | [logrus](https://github.com/sirupsen/logrus)                        | [README](./xlog/README.md)    | -   | -     | 日志记录                  |
| xtrace  | [opentelemetry](https://github.com/open-telemetry/opentelemetry-go) | [README](./xtrace/README.md)  | -   | -     | 链路追踪                  |
| xgorm   | [gorm](https://gorm.io/)                                            | [README](./xgorm/README.md)   | ✅   | ✅     | 数据库(MySQL/PostgreSQL) |
| xhttp   | [go-resty](https://github.com/go-resty/resty)                       | [README](./xhttp/README.md)   | -   | ✅     | HTTP 客户端              |
| xcache  | [ristretto](https://github.com/dgraph-io/ristretto)                 | [README](./xcache/README.md)  | -   | -     | 本地缓存（支持 TTL/泛型）     |

## 多数据库配置

支持配置多个数据库实例：

```yaml
XGorm:
  - Name: "master"
    Driver: "mysql"
    DSN: "user:pass@tcp(127.0.0.1:3306)/master_db"
    MaxOpenConns: 100
  - Name: "slave"
    Driver: "postgres"
    DSN: "host=127.0.0.1 user=postgres password=pass dbname=slave_db port=5432 sslmode=disable"
    MaxOpenConns: 50
```

```go
// 获取指定数据库
masterDB := xgorm.CWithCtx(ctx, "master")
slaveDB := xgorm.CWithCtx(ctx, "slave")

// 获取默认数据库（第一个配置）
defaultDB := xgorm.CWithCtx(ctx)
```

## 服务启动方式

```go
package main

import (
	"github.com/gin-gonic/gin"
	"github.com/xiaoshicae/xone"
	"github.com/xiaoshicae/xone/xhook"
)

func init() {
	// 注册启动前钩子
	xhook.BeforeStart(func() error {
		// 自定义初始化逻辑
		return nil
	})

	// 注册停止前钩子
	xhook.BeforeStop(func() error {
		// 自定义清理逻辑
		return nil
	})
}

func main() {
	// 方式一：Gin Web 服务
	xone.RunGin(gin.Default())

	// 方式二：Gin HTTPS 服务
	xone.RunGinTLS(gin.Default(), "/path/to/cert.pem", "/path/to/key.pem")

	// 方式三：自定义 Server（需实现 xone.Server 接口）
	xone.RunServer(myServer)

	// 方式四：阻塞服务（适用于 consumer、job 等）
	xone.RunBlockingServer()

	// 方式五：单次执行（适用于调试）
	xone.R()
}
```

## 环境变量

| 环境变量                     | 说明           | 示例                |
|--------------------------|--------------|-------------------|
| `SERVER_ENABLE_DEBUG`    | 启用 XOne 调试日志 | `true`            |
| `SERVER_PROFILES_ACTIVE` | 指定激活的配置环境    | `dev`, `prod`     |
| `SERVER_CONFIG_LOCATION` | 指定配置文件路径     | `/app/config.yml` |

配置文件支持环境变量占位符：

```yaml
XGorm:
  DSN: "${DB_DSN:-user:pass@tcp(localhost:3306)/db}"
```

## IDE Schema 配置

为 YAML 配置文件启用智能提示：

**GoLand**: Settings → Languages & Frameworks → Schemas and DTDs → JSON Schema Mappings

添加映射：

- Schema: `${GOPATH}/pkg/mod/github.com/xiaoshicae/xone@{version}/config_schema.json`
- File pattern: `application*.yml`

## 更新日志

- **v1.3.1** (2026-02-06) - feat: 新增 xcache 本地缓存模块，基于 ristretto，支持 TTL、泛型 API、多实例
- **v1.2.1** (2026-02-06) - feat: xhttp 新增 DialKeepAlive 配置，支持自定义 TCP keep-alive 探测间隔
- **v1.2.0** (2026-02-05) - feat: xhttp 新增 DialTimeout 配置，支持自定义 TCP 连接超时时间
- **v1.1.5** (2026-02-02) - feat: xtrace 新增 B3 传播格式支持，兼容 W3C Trace Context
- **v1.1.4** (2026-01-30) - fix: xlog ConsoleFormatIsRaw=true 时控制台输出纯 JSON，去除颜色前缀
- **v1.1.3** (2026-01-30) - perf: xlog 文件写入改为异步，避免磁盘 I/O 阻塞调用方
- **v1.1.2** (2026-01-29) - fix: SERVER_PROFILES_ACTIVE 指定的配置文件不存在时忽略并回落到 application.yml
- **v1.1.1** (2026-01-29) - fix: xlog 日志定位文件名误指向 hook 文件
- **v1.1.0** (2026-01-27) - feat: 新增 RunGinTLS 支持 HTTPS 启动; fix: xlog RawLog 增加 ctx nil 检查
- **v1.0.4** (2026-01-27) - fix xconfig 环境变量展开
- **v1.0.3** (2026-01-26) - xtrace支持W3C Trace Context propagator
- **v1.0.2** (2026-01-26) - 稳定性修复与测试补充
- **v0.0.8** (2026-01-21) - xhttp支持重试
- **v0.0.7** (2026-01-21) - 修复xconfig bug
- **v0.0.6** (2026-01-21) - xhttp模块优化
- **v0.0.5** (2026-01-04) - 删除gin支持debug mode
- **v0.0.4** (2026-01-04) - gin支持debug mode
- **v0.0.3** (2026-01-04) - config新增parent目录检测
- **v0.0.2** (2026-01-04) - 优化IP获取
- **v0.0.1** (2026-01-04) - 初始版本
