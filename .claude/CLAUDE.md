# XOne 项目说明

## 项目概述

XOne 是 Go 三方库集成框架，提供配置管理、日志、HTTP 客户端、数据库、链路追踪等开箱即用的功能模块。

## 语言规定

- 对话、代码注释、文档：简体中文
- commit message：英文

## 模块地图

```
xone/
├── xutil/       # 工具函数（基础层，无外部依赖）
├── xhook/       # 生命周期钩子（基础层）
├── xconfig/     # 配置管理（核心层，基于 Viper）
├── xtrace/      # 链路追踪（核心层，基于 OpenTelemetry）
├── xlog/        # 日志（核心层，基于 Logrus）
├── xhttp/       # HTTP 客户端（服务层，基于 Resty）
├── xgorm/       # 数据库（服务层，基于 GORM，MySQL/PostgreSQL）
├── xserver/     # 服务运行和生命周期管理（生命周期层）
├── xgin/        # Gin Web 框架集成（应用层，Builder 模式 + 内置中间件）
│   ├── middleware/   # 中间件（Log/Trace/Recover/Session）
│   ├── options/      # 选项
│   ├── swagger/      # Swagger 集成
│   └── trans/        # 中文翻译
└── test/        # 集成测试
```

## Hook 生命周期

**BeforeStart 初始化顺序**：xconfig → xtrace → xlog → xhttp → xgorm
**BeforeStop 关闭顺序**：倒序执行（xgorm → xhttp → xlog → xtrace）

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
- `/build-fix` - 快速修复编译错误

## 关键依赖

- **Web**: gin, swaggo/gin-swagger, swaggo/swag
- **HTTP**: go-resty
- **数据库**: gorm, pgx, mysql-driver
- **链路追踪**: opentelemetry, otelhttp
- **日志**: logrus, file-rotatelogs
- **配置**: viper, godotenv
- **验证**: go-playground/validator
- **测试**: bytedance/mockey, goconvey
