## XLog 模块

### 1. 模块简介

基于标准库 [log/slog](https://pkg.go.dev/log/slog) 的日志模块，**不引入任何第三方日志依赖**，提供：

- 结构化 JSON 日志输出
- 默认仅打印到标准输出，开箱适配 K8s 等容器环境
- 可选的文件落盘与按时间自动轮转（模块内实现，无第三方依赖）
- 文件写入异步化，日志 I/O 不阻塞业务调用
- 每条日志最多只做一次 JSON 序列化，控制台使用可读格式时不做序列化
- 可作为 `slog.Handler` / `*slog.Logger` 交给第三方库复用同一套输出配置
- 调用方定位按程序计数器缓存，避免每条日志重复做符号解析
- OpenTelemetry TraceID / SpanID 自动关联
- 彩色控制台输出
- 自定义 KV 字段与 Context 透传

### 2. 配置参数

`Level` 与 `Timezone` 为两路输出共用，控制台与文件各自的配置分别位于 `Console` 与 `File` 子块：

```yaml
XLog:
  Level: "info"               # 日志级别（optional, default "info"），可选 debug/info/warn/error/fatal，大小写不敏感
  Timezone: "Asia/Shanghai"   # 日志时间时区（optional, default "Asia/Shanghai"），加载失败时回退系统本地时区

  Console:
    Enable: true              # 是否输出到控制台（optional, default true）
    Format: "text"            # 输出格式（optional, default "text"），可选 text / json

  File:
    Enable: false             # 是否写入日志文件（optional, default false）
    Path: "./log"             # 日志文件夹路径（optional, default "./log"）
    Name: "app"               # 日志文件名称（optional, default "app"）
    MaxAge: "7d"              # 日志保留时长（optional, default "7d"），设为 0 表示不清理
    RotateTime: "1d"          # 日志切割周期（optional, default "1d"），最小 1m
```

时间类配置支持 `d`（天）前缀及 Go duration 单位，如 `7d`、`1d12h`、`6h`。
所有配置项均支持环境变量占位符，如 `Enable: "${XLOG_FILE_ENABLE:-false}"`。

#### 输出目标

两路输出各自独立开关：

| File.Enable | Console.Enable | 输出结果 | 典型场景 |
|-------------|----------------|---------|---------|
| `false`（默认） | `true`（默认） | 仅标准输出 | K8s / 容器 |
| `true` | `true` | 文件 + 标准输出 | 本地开发调试 |
| `true` | `false` | 仅文件 | 物理机 / 虚拟机部署 |
| `false` | `false` | 强制回退为仅标准输出 | 配置错误保护 |

`File.Enable` 为 `false` 时不会创建日志目录，也不会创建轮转文件，`File` 子块的其余配置均不生效。

#### K8s / 容器环境

容器环境中日志一般由采集器（Filebeat / Fluent Bit / Loki 等）从容器标准输出收集，无需落盘，
这正是本模块的默认行为。建议将控制台格式设为 `json`，便于采集器解析与日志平台检索：

```yaml
XLog:
  Level: "info"
  Console:
    Format: "json"
```

#### 日志落盘

需要写入文件时显式开启 `File.Enable`。日志按 `RotateTime` 周期轮转，文件名形如
`xxx.log.20260912`，并维护一个指向当前文件的符号链接 `xxx.log` 便于 tail 跟随；
超过 `MaxAge` 的历史文件会在轮转时自动清理。

```yaml
XLog:
  File:
    Enable: true
    Path: "/a/b/c"            # 日志保存到 /a/b/c/ 目录下
    Name: "xxx"
    MaxAge: "10d"
    RotateTime: "2d"
  Console:
    Enable: false             # 可选：关闭控制台输出，仅写文件
```

`RotateTime` 支持小于一天的周期（如 `"6h"`、`"30m"`），文件名后缀会自动使用更细的
时间粒度：`20260912` → `2026091206` → `202609120630`。由于最细只到分钟，
**小于 `1m` 的周期无法兑现，会回退到默认值**——注意无单位的数字会被解析为纳秒（`"5"` 即 5ns）。

### 3. API 接口

```go
// 日志输出，ctx 为必传参数，用于关联 TraceID/SpanID 及 Context 中的 KV
func Debug(ctx context.Context, msg string, args ...any)
func Info(ctx context.Context, msg string, args ...any)
func Warn(ctx context.Context, msg string, args ...any)
func Error(ctx context.Context, msg string, args ...any)

// 指定级别输出
func RawLog(ctx context.Context, level Level, msg string, args ...any)

// 日志级别，不暴露底层日志库类型
type Level uint32

const (
    PanicLevel Level = iota
    FatalLevel
    ErrorLevel
    WarnLevel
    InfoLevel
    DebugLevel
    TraceLevel
)

// 解析级别名称，大小写不敏感，兼容 "warning" 别名
func ParseLevel(s string) (Level, bool)

// 添加自定义 KV，作为 args 传入上述日志函数
func KV(k string, v any) Option
func KVMap(m map[string]any) Option

// 在 Context 中注入 KV（后续日志自动携带），以及读取已注入的 KV
func CtxWithKV(ctx context.Context, kvs map[string]any) context.Context
func KVFromCtx(ctx context.Context) map[string]any

// 获取当前生效的日志级别
func CurrentLevel() Level
func XLogLevel() string
```

`args` 中的 `Option` 会被提取为 JSON 字段，其余参数用于 `msg` 的格式化占位符。

日志级别未开启时（如线上配置 info 却调用 `Debug`），调用会立即返回，不产生格式化与内存分配开销。

#### 日志观察者

需要在日志写出时做旁路处理（指标上报、告警等）可注册观察者，它与具体日志库解耦：

```go
// Record 只包含元信息，不依赖任何日志库类型
type Record struct {
    Level   Level
    Message string
    File    string // 文件名（不含路径）
    Line    int
    TraceID string
    SpanID  string
}

type Observer func(ctx context.Context, r Record)

func AddObserver(o Observer)
```

观察者对所有级别的日志生效，需自行按 `Record.Level` 过滤。注意两点：

- 必须快速返回，耗时操作自行异步化，否则会拖慢日志写入
- 不得在其中调用 xlog 的日志函数，否则会无限递归
- panic 会被捕获并隔离，不会影响日志输出与其他观察者，但仍应自行处理错误

`xmetric` 的错误指标自动上报即基于该扩展点实现。

#### 与 slog 生态互操作

底层为标准库 `log/slog`，可把本模块的处理器交给任何接受 slog 的第三方库，
使其日志与业务日志共用同一套格式、输出目标与 traceid 注入：

```go
func Handler() slog.Handler   // 当前生效的处理器
func Logger() *slog.Logger    // 基于当前配置的 logger

// 例：让第三方库的日志并入本模块
thirdparty.SetLogger(xlog.Logger())
```

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

- `Console.Format: "text"`（默认）：`[INFO][2024-10-15 19:45:05.136] main.go:44 trace-id some info`
- `Console.Format: "json"`：结构化 JSON 格式，字段同上，容器环境推荐
