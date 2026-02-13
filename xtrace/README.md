## XTrace模块

### 1. 模块简介

XTrace 是 XOne 框架的分布式追踪模块，基于 OpenTelemetry 封装，提供：
- 自动 Trace 初始化和生命周期管理
- 与 xhttp、xgorm 等模块无缝集成
- 支持 W3C Trace Context 和 B3 两种传播格式
- 支持控制台打印 Trace 信息（调试用）
- 线程安全的 shutdown 机制

> 当前只是本地记录 trace，暂不支持上报到远程服务端。

### 2. 配置参数

```yaml
XTrace:
  Enable: true    # Trace 是否开启(optional default true)
  Console: false  # 是否在控制台打印 trace 内容(optional default false)
```

### 3. API 接口

```go
// 检查 Trace 是否启用
xtrace.EnableTrace() bool

// 获取 Tracer，用于创建自定义 Span
xtrace.GetTracer(name string, opts ...trace.TracerOption) trace.Tracer

// 设置 shutdown 超时时间（默认 5 秒）
xtrace.SetShutdownTimeout(timeout time.Duration)
```

### 4. 使用示例

```go
package main

import (
    "context"
    "github.com/xiaoshicae/xone/v2/xtrace"
)

func main() {
    // 检查 Trace 是否启用
    if xtrace.EnableTrace() {
        // 创建自定义 Span
        tracer := xtrace.GetTracer("my-service")
        ctx, span := tracer.Start(context.Background(), "my-operation")
        defer span.End()

        // 添加属性
        span.SetAttributes(
            attribute.String("key", "value"),
        )

        // 记录事件
        span.AddEvent("something happened")

        // 业务逻辑...
        doSomething(ctx)
    }
}
```

### 5. 自动集成

通过 XOne 运行的应用会自动初始化 Trace，无需手动调用。以下模块已自动集成：

| 模块 | 集成方式 |
|-----|---------|
| xhttp | HTTP 请求自动创建 Span |
| xgorm | 数据库操作自动创建 Span |
| xlog | 日志自动关联 TraceID/SpanID |

### 6. 注意事项

- `Enable=false` 时会使用 NoopTracerProvider，不产生任何开销
- `Console=true` 仅用于本地调试，生产环境建议关闭
- shutdown 超时可通过 `SetShutdownTimeout()` 调整
