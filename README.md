# XOne

开箱即用的 Golang 三方库集成 SDK

## 功能特性

- 统一集成三方包，降低维护成本
- 通过配置启用能力，开箱即用
- 提供最佳实践的默认参数配置
- 支持 Hook 机制，灵活扩展
- 集成 OpenTelemetry 链路追踪

## 环境要求

- Go >= 1.24.0

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
  Version: "v1.0.0"
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

	// 方式二：自定义 Server（需实现 xone.Server 接口）
	xone.RunServer(myServer)

	// 方式三：阻塞服务（适用于 consumer、job 等）
	xone.RunBlockingServer()

	// 方式四：单次执行（适用于调试）
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

- **v1.0.1** (2026-01-24) - Bug 修复与测试增强
  - xtrace: 修复 sync.Once 重置并发问题，改用 atomic.Bool
  - xconfig: 添加正则匹配结果边界检查，防止 panic
  - xgorm: 使用 errors.Join() 合并多个关闭错误
  - xutil: 修正 strToDuration 错误日志描述
  - 测试覆盖率大幅提升（xhttp 98%, xconfig 92%, xlog 85%, xtrace 82%）

- **v1.0.0** (2026-01-24) - 正式版本发布
  - 全模块代码优化：性能提升、并发安全、代码重构
  - xhook: 延迟排序、新增 SetStopTimeout API
  - xconfig: 正则预编译、路径检测重构
  - xlog: 内存预分配、缓存优化
  - xtrace: 并发保护、新增 GetTracer/SetShutdownTimeout API
  - xhttp: 并发保护、警告日志、支持 "d" 时间格式
  - xgorm: 修复 GetDriver 默认值、并发保护、日志优化
  - xutil: 统一错误处理、sync.Once 保护
  - 添加 Claude Code 项目配置

