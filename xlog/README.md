## XLog 模块

### 1. 模块简介

基于 [logrus](https://github.com/sirupsen/logrus) 封装的日志模块，提供：

- 结构化 JSON 日志输出
- 默认仅打印到标准输出，开箱适配 K8s 等容器环境
- 可选的文件落盘与自动轮转（基于 [file-rotatelogs](https://github.com/lestrrat-go/file-rotatelogs)）
- 文件写入异步化，日志 I/O 不阻塞业务调用
- OpenTelemetry TraceID / SpanID 自动关联
- 彩色控制台输出
- 自定义 KV 字段与 Context 透传

### 2. 配置参数

```yaml
XLog:
  Level: "info"               # 日志级别（optional, default "info"），可选 debug/info/warn/error/fatal，大小写不敏感
  EnableFile: false           # 是否写入日志文件（optional, default false）
  EnableConsole: true         # 是否打印到控制台（标准输出）（optional, default true）
  ConsoleFormatIsRaw: false   # 控制台是否输出原始 JSON 格式（optional, default false）
  Name: "app"                 # 日志文件名称（optional, default "app"），仅 EnableFile 为 true 时生效
  Path: "./log"               # 日志文件夹路径（optional, default "./log"），仅 EnableFile 为 true 时生效
  MaxAge: "7d"                # 日志保存最大时长（optional, default "7d"），仅 EnableFile 为 true 时生效
  RotateTime: "1d"            # 日志切割周期（optional, default "1d"），仅 EnableFile 为 true 时生效
  Timezone: "Asia/Shanghai"   # 日志时间时区（optional, default "Asia/Shanghai"），加载失败时回退系统本地时区
```

时间类配置支持 `d`（天）前缀及 Go duration 单位，如 `7d`、`1d12h`、`500ms`。
所有配置项均支持环境变量占位符，如 `EnableFile: "${XLOG_ENABLE_FILE:-false}"`。

#### 输出目标

日志输出由 `EnableFile` 和 `EnableConsole` 两个开关共同决定：

| EnableFile | EnableConsole | 输出结果 | 典型场景 |
|------------|---------------|---------|---------|
| `false`（默认） | `true`（默认） | 仅标准输出 | K8s / 容器 |
| `true` | `true` | 文件 + 标准输出 | 本地开发调试 |
| `true` | `false` | 仅文件 | 物理机 / 虚拟机部署 |
| `false` | `false` | 强制回退为仅标准输出 | 配置错误保护 |

`EnableFile` 为 `false` 时不会创建日志目录，也不会创建轮转文件，`Name` / `Path` / `MaxAge` / `RotateTime` 均不生效。

#### K8s / 容器环境

容器环境中日志一般由采集器（Filebeat / Fluent Bit / Loki 等）从容器标准输出收集，无需落盘，
这正是本模块的默认行为，不配置即可使用。建议开启原始 JSON 输出，便于采集器解析与日志平台检索：

```yaml
XLog:
  Level: "info"
  ConsoleFormatIsRaw: true
```

#### 日志落盘

需要写入文件时显式开启 `EnableFile`，按如下配置日志保存到 `/a/b/c/xxx.log`：

```yaml
XLog:
  EnableFile: true
  Path: "/a/b/c"
  Name: "xxx"
  MaxAge: "10d"
  RotateTime: "2d"
  EnableConsole: false        # 可选：关闭控制台输出，仅写文件
```

### 3. API 接口

```go
// 日志输出，ctx 为必传参数，用于关联 TraceID/SpanID 及 Context 中的 KV
func Debug(ctx context.Context, msg string, args ...any)
func Info(ctx context.Context, msg string, args ...any)
func Warn(ctx context.Context, msg string, args ...any)
func Error(ctx context.Context, msg string, args ...any)

// 指定级别输出
func RawLog(ctx context.Context, level logrus.Level, msg string, args ...any)

// 添加自定义 KV，作为 args 传入上述日志函数
func KV(k string, v any) Option
func KVMap(m map[string]any) Option

// 在 Context 中注入 KV（后续日志自动携带）
func CtxWithKV(ctx context.Context, kvs map[string]any) context.Context

// 获取当前日志级别
func XLogLevel() string
```

`args` 中的 `Option` 会被提取为 JSON 字段，其余参数用于 `msg` 的格式化占位符。

### 4. 使用示例

```go
package main

import (
    "context"

    "github.com/xiaoshicae/xone/v2/xlog"
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
    kvs := map[string]any{"userId": "u001", "action": "purchase"}
    xlog.Info(ctx, "user action", xlog.KVMap(kvs))

    // 格式化参数与 KV 混用
    xlog.Error(ctx, "pay failed, orderId=[%s]", "12345", xlog.KV("errCode", 500))

    // Context 注入 KV（后续所有日志自动携带）
    ctx = xlog.CtxWithKV(ctx, map[string]any{"requestId": "req-123"})
    xlog.Info(ctx, "processing request") // 自动包含 requestId
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

- `ConsoleFormatIsRaw: false`（默认）：`[INFO][2024-10-15 19:45:05.136] main.go:44 trace-id some info`
- `ConsoleFormatIsRaw: true`：原始 JSON 格式，字段同上，容器环境推荐
