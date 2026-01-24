## XLog模块

### 1. 模块简介

XLog 是 XOne 框架的日志模块，基于 logrus 封装，提供：
- 结构化 JSON 日志输出
- 自动日志轮转（基于 file-rotatelogs）
- OpenTelemetry TraceID/SpanID 自动关联
- 彩色控制台输出
- 自定义 KV 字段支持

### 2. 配置参数

```yaml
# 按如下配置，日志会保存到 /a/b/c/xxx.log
# 如果没有任何配置，日志会默认保存到 ./log/app.log
XLog:
  Level: "debug"            # 日志级别(optional default "info")，支持 debug/info/warn/error/fatal
  Name: "xxx"               # 日志文件名称(optional default "app")
  Path: "/a/b/c"            # 日志文件夹路径(optional default "./log/")
  Console: true             # 日志内容是否需要在控制台打印(optional default false)
  ConsoleFormatIsRaw: true  # 控制台打印原始JSON格式(optional default false)
  MaxAge: "10d"             # 日志保存最大天数(optional default "7d")
  RotateTime: "2d"          # 日志切割周期(optional default "1d")
  Timezone: "Asia/Shanghai" # 时区设置(optional default "Asia/Shanghai")
```

### 3. API 接口

```go
// 日志输出
xlog.Debug(ctx context.Context, msg string, args ...interface{})
xlog.Info(ctx context.Context, msg string, args ...interface{})
xlog.Warn(ctx context.Context, msg string, args ...interface{})
xlog.Error(ctx context.Context, msg string, args ...interface{})

// 添加自定义 KV
xlog.KV(k string, v interface{}) Option
xlog.KVMap(m map[string]interface{}) Option

// 在 Context 中注入 KV（后续日志自动携带）
xlog.CtxWithKV(ctx context.Context, kvs map[string]interface{}) context.Context

// 获取当前日志级别
xlog.XLogLevel() string
```

### 4. 使用示例

```go
package main

import (
    "context"
    "github.com/xiaoshicae/xone/xlog"
)

func main() {
    ctx := context.Background()

    // 基础用法
    xlog.Info(ctx, "some info")

    // 格式化参数
    xlog.Info(ctx, "user %s login success", "alice")

    // 自定义 KV
    xlog.Info(ctx, "order created", xlog.KV("orderId", "12345"), xlog.KV("amount", 99.9))

    // KVMap 批量添加
    kvs := map[string]interface{}{"userId": "u001", "action": "purchase"}
    xlog.Info(ctx, "user action", xlog.KVMap(kvs))

    // Context 注入 KV（后续所有日志自动携带）
    ctx = xlog.CtxWithKV(ctx, map[string]interface{}{"requestId": "req-123"})
    xlog.Info(ctx, "processing request")  // 自动包含 requestId
}
```

### 5. 日志 JSON 字段说明

```json
{
  "msg": "some info",                 // 日志内容
  "time": "2024-10-15 19:45:05.136",  // 日志时间
  "level": "info",                    // 日志级别
  "filename": "main.go",              // 文件名
  "lineid": "44",                     // 行号
  "ip": "10.10.10.10",                // 服务器 IP
  "pid": "123",                       // 进程 ID
  "servername": "my-app",             // 服务名
  "traceid": "xxxxx",                 // OpenTelemetry TraceID
  "spanid": "xxxxx",                  // OpenTelemetry SpanID
  "k1": "v1"                          // 自定义 KV
}
```

### 6. 控制台输出格式

- **ConsoleFormatIsRaw=false**（默认）: `[INFO][2024-10-15 19:45:05.136] main.go:44 trace-id some info`
- **ConsoleFormatIsRaw=true**: 原始 JSON 格式
