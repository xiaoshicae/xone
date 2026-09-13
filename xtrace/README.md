## XTrace模块

### 1. 模块简介

XTrace 是 XOne 框架的分布式追踪模块，基于 OpenTelemetry 封装，提供：
- 自动 Trace 初始化和生命周期管理
- 与 xhttp、xgorm 等模块无缝集成
- 支持 W3C Trace Context 和 B3 两种传播格式
- 支持控制台打印 Trace 信息（调试用）
- 链路关闭时仍保留 Header 透传能力
- 线程安全的 shutdown 机制

> **当前不上报到远程服务端**。`EnableConsole=false` 时不注册任何 SpanProcessor，
> Span 创建后直接丢弃 —— 此时模块提供的是 TraceID / SpanID（供 xlog 关联日志）
> 与 Header 透传能力，而不是可查询的链路数据。

### 2. 配置参数

```yaml
XTrace:
  Enable: true               # Trace 是否开启 (optional, default true)
  EnableConsole: false       # 是否在控制台打印 trace 内容，仅用于本地调试 (optional, default false)
  SampleRatio: 1.0           # 采样率，取值 (0,1]，>=1 全采样 (optional, default 1.0)
  ForwardHeaders:            # 全局透传的自定义 HTTP Header 列表，向所有下游服务透传 (optional, default nil)
    - X-Request-Id
  ForwardHeaderRules:        # 按域名透传的 Header 规则列表 (optional, default nil)
    - Domains:               # 域名模式，支持精确匹配和通配符前缀（如 *.example.com），忽略端口和大小写
        - "*.svc.cluster.local"
      Headers:               # 仅当目标域名匹配时才透传的 Header
        - X-Auth-Token
        - X-Tenant-Id
```

### 3. API 接口

```go
// 检查 Trace 是否启用（需在 xtrace 的 BeforeStart Hook 执行之后调用）
xtrace.EnableTrace() bool

// 检查是否配置了自定义 Header 透传
xtrace.EnableForwardHeader() bool

// 获取 Tracer，用于创建自定义 Span
xtrace.GetTracer(name string, opts ...trace.TracerOption) trace.Tracer

// 从 context 读取透传的 Header
xtrace.ForwardHeadersFromContext(ctx) map[string]string
xtrace.ForwardHeaderFromContext(ctx, key) string
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

### 6. Header 透传的边界

- **`ForwardHeaders`（全局）向所有下游域名注入**，敏感 Header 请放进 `ForwardHeaderRules`
- 同一个 Header 同时出现在两处属于误配：**以更严格的域名规则为准**，不会全局透传
- `ForwardHeaderRules` 的域名匹配依赖请求目标 Host 被写入 context，这由 `xtrace.HostAwareTransport`
  完成，而它只在 **xhttp 创建的 client** 上自动挂载。用户自建 `http.Client` 时规则不生效，
  需自行包一层 `&xtrace.HostAwareTransport{Next: ...}`
- 只有从上游请求 Extract 到的值才会向下游 Inject；框架不提供手动写入透传值的 API

### 7. 注意事项

- `Enable=false` 时使用 NoopTracerProvider，不产生链路开销；**但已配置的 Header 透传仍然生效**
- `EnableConsole=true` 仅用于本地调试，生产环境建议关闭
- 关闭链路请用 `Enable=false`，不要把 `SampleRatio` 设为 0（0 视为未配置，回落到全采样）
- shutdown 超时由 xhook 的 Hook 超时统一控制（默认 10 秒），可用 `xhook.SetStopTimeout()` 调整整体预算
