# XOne 项目说明

## 项目概述

XOne 是一个 Go 语言微服务框架，提供配置管理、日志、HTTP 客户端、数据库、链路追踪等开箱即用的功能模块。

## 项目结构

```
xone/
├── xconfig/     # 配置管理模块 (基于 Viper)
├── xlog/        # 日志模块 (基于 Logrus)
├── xhttp/       # HTTP 客户端模块 (基于 Resty)
├── xgorm/       # 数据库模块 (基于 GORM，支持 MySQL/PostgreSQL)
├── xtrace/      # 链路追踪模块 (基于 OpenTelemetry)
├── xhook/       # 生命周期钩子模块
├── xutil/       # 工具函数模块
└── test/        # 集成测试
```

## 编码规范

- Go 版本: 1.24+
- 使用 `gofmt` 格式化代码
- 错误处理统一使用 `fmt.Errorf("XOne xxx failed, err=[%v]", err)` 格式
- 日志使用 `xutil.InfoIfEnableDebug()` / `xutil.ErrorIfEnableDebug()` 等函数
- 时间配置统一使用 `xutil.ToDuration()`，支持 "d"（天）格式
- 并发访问全局变量需要使用 `sync.RWMutex` 或 `sync.Once` 保护

## 常用命令

```bash
# 运行所有测试（需要 -gcflags 禁用内联以支持 Mockey）
go test -gcflags="all=-N -l" ./...

# 运行单个模块测试
go test -gcflags="all=-N -l" ./xhttp/... -v

# 构建
go build ./...

# 格式化代码
gofmt -w .
```

## 测试框架

- 使用 [bytedance/mockey](https://github.com/bytedance/mockey) 进行 mock
- 使用 [smartystreets/goconvey](https://github.com/smartystreets/goconvey) 进行断言
- 测试时必须添加 `-gcflags="all=-N -l"` 参数

## 模块配置示例

```yaml
# application.yml
XLog:
  Level: "info"
  Console: true

XHttp:
  Timeout: "60s"
  RetryCount: 3

XGorm:
  Driver: "postgres"
  DSN: "host=localhost user=test dbname=test"

XTrace:
  Enable: true
  Console: false
```

## 注意事项

- 默认数据库驱动是 PostgreSQL
- 修改代码后确保运行测试验证
- 新增 API 需要更新对应模块的 README.md
