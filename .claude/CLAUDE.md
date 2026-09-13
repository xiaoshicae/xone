# XOne 项目说明

## 项目概述

XOne 是 Go 三方库集成框架，提供配置管理、日志、HTTP 客户端、数据库、链路追踪等开箱即用的功能模块。

## 语言规定

- 对话、代码注释、文档：简体中文
- commit message：英文

## 模块地图

```
xone/
├── xerror/      # 统一错误类型（基础层，零第三方依赖）
├── xutil/       # 工具函数（基础层，零第三方依赖）
├── xhook/       # 生命周期钩子（基础层，零第三方依赖）
├── internal/
│   └── hookorder/    # 框架保留 Order，外部 import 编译失败
├── xconfig/     # 配置管理（核心层，基于 Viper）
├── xlog/        # 日志（核心层，基于标准库 log/slog）
├── xtrace/      # 链路追踪（核心层，基于 OpenTelemetry）
├── xmetric/     # 指标采集（核心层，基于 Prometheus）
├── xflow/       # 流程编排（核心层，强弱依赖 + 自动回滚）
├── xhttp/       # HTTP 客户端（服务层，基于 Resty）
├── xgorm/       # 数据库（服务层，基于 GORM，MySQL/PostgreSQL）
├── xredis/      # Redis（服务层，基于 go-redis）
├── xcache/      # 本地缓存（服务层，基于 ristretto）
├── xserver/     # 服务运行和生命周期管理（生命周期层）
├── xgin/        # Gin Web 框架集成（应用层，Builder 模式 + 内置中间件）
│   ├── middleware/   # 中间件（Session/Trace/Log/Metric/Recover）
│   ├── options/      # 选项
│   ├── swagger/      # Swagger 集成
│   └── trans/        # 中文翻译
└── test/        # 集成测试
```

## Hook 生命周期

**BeforeStart 初始化顺序**：xconfig（Order -100）→ xlog（Order -50）→ 其余模块（默认 Order 100，同 Order 内按 init 顺序）
**BeforeStop 关闭顺序**：BeforeStart 的整体镜像，业务资源 → xlog → xconfig，同 Order 内 LIFO
**注意**：只有两个保留层级是框架承诺的；业务区内部的先后由 import path 字典序决定，不要依赖它
**Order 语义**：资源层级，值越小越底层 —— 启动越早、关闭越晚。负值为框架保留区（xconfig=-100、xlog=-50，经 `xhook.ReservedOrder` + `internal/hookorder` 强制隔离），业务 Hook 传负值会 panic，保持默认 100 即可
**注意**：Go 的 init 顺序是「拓扑序 + import path 字典序」，**与 import 书写顺序无关**，不要靠调整 import 控制生命周期顺序

## 核心设计模式

### 全局状态管理

```go
var (
    defaultClient *Client
    clientMu      sync.RWMutex
)
```

### 幂等 Hook 注册

```go
func init() {
    xhook.BeforeStart(initModule)
    xhook.BeforeStop(closeModule)
}
```

### 标准模块文件职责

- `config.go` - 配置结构体 + `configMergeDefault()` + 配置读取 API
- `client.go` - 全局状态 + 对外 API（`C()` / `CWithCtx(ctx)`）
- `x{模块名}_init.go` - 初始化/关闭逻辑 + Hook 注册
- `x{模块名}_test.go` - 单元测试

## 常用命令

```bash
go test -gcflags="all=-N -l" ./...          # 运行所有测试（必须禁用内联以支持 Mockey）
go test -gcflags="all=-N -l" ./xhttp/... -v # 运行单个模块测试
go build ./...                               # 构建
gofmt -w .                                   # 格式化
go vet ./...                                 # 静态检查
```

## 自定义 Skills

- `/commit` - 规范提交（测试 + 覆盖率 + 版本号 + 提交）
- `/test` - 运行测试
- `/new-module` - 创建新模块
- `/review` - 模块代码审查
- `/perf` - 性能瓶颈分析
- `/build-fix` - 快速修复编译错误
- `/integration-test` - 集成测试（自动启动 Docker 依赖 + 运行 test/ 下的测试）

## 关键依赖

- **Web**: gin, swaggo/gin-swagger, swaggo/swag
- **HTTP**: go-resty
- **数据库**: gorm, pgx, mysql-driver
- **链路追踪**: opentelemetry, otelhttp
- **日志**: log/slog（标准库，无第三方依赖）
- **配置**: viper, godotenv
- **验证**: go-playground/validator
- **测试**: bytedance/mockey, goconvey
