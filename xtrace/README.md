## XTrace模块

### 1. 模块简介

XTrace 是 XOne 框架的分布式追踪模块，基于 OpenTelemetry 封装，提供：
- 自动 Trace 初始化和生命周期管理
- 与 xhttp、xgorm 等模块无缝集成
- 支持 W3C Trace Context 和 B3 两种传播格式
- 支持控制台打印 Trace 信息（调试用）
- 链路关闭时仍保留 Header 透传能力
- 通过 `AddSpanProcessor` 接入任意上报后端
- 线程安全的 shutdown 机制

> **框架不内置上报 exporter**。OTLP / Jaeger 等 exporter 会带进上百个构建依赖
> （实测 OTLP 为 **+143 个包**，引入 grpc、protobuf、genproto 等模块树），
> 不应由所有使用者承担。需要上报的服务通过 `xtrace.AddSpanProcessor` 自行接入，
> 见第 4 节；不接入时 Span 创建后直接丢弃，模块提供的是 TraceID / SpanID
> （供 xlog 关联日志）与 Header 透传能力。

### 2. 配置参数

```yaml
XTrace:
  Enable: true               # Trace 是否开启 (optional, default true)
  EnableConsole: false       # 是否在控制台打印 trace 内容，仅用于本地调试 (optional, default false)
  SampleRatio: 1.0           # 采样率，取值 (0,1]，>=1 全采样 (optional, default 1.0)
  ShutdownTimeout: "5s"      # 关闭时等待 Span 导出完成的上限 (optional, default "5s")
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

// 注册自定义 SpanProcessor，用于把 Span 上报到远端
xtrace.AddSpanProcessor(sp sdktrace.SpanProcessor)
```

### 4. 上报到远端（OTLP / Jaeger / 自研）

框架只提供接入口，exporter 由服务自行引入 —— 这样不上报的服务不必承担相关依赖。
在服务的 `main` 包中注册即可：

```go
import (
    "context"

    "github.com/xiaoshicae/xone/v2/xconfig"
    "github.com/xiaoshicae/xone/v2/xtrace"
    "go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
    sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func init() {
    exp, err := otlptracegrpc.New(context.Background(),
        otlptracegrpc.WithEndpoint(xconfig.GetString("MyApp.Otlp.Endpoint")),
        otlptracegrpc.WithInsecure(),
    )
    if err != nil {
        panic(err)
    }
    // 网络导出必须用 Batch 处理器，避免每个 Span 结束时同步阻塞
    xtrace.AddSpanProcessor(sdktrace.NewBatchSpanProcessor(exp))
}
```

要点：

- **随时可注册**：初始化前注册的会在初始化时装上，初始化后注册的立即生效，重复初始化会自动装回
- **生命周期由框架管**：注册的处理器在 BeforeStop 阶段被统一关闭，等待上限是 `ShutdownTimeout`
- **导出端不可达不影响启动与请求**：`otlptracegrpc.New` 不做连接握手，Span 结束也不阻塞；
  但**关闭时会一直等到超时**，所以 `ShutdownTimeout` 要配一个你能接受的值
- **配置放业务自己的 key 下**（如上面的 `MyApp.Otlp.Endpoint`），框架不定义上报相关配置项
- `Enable=false` 时注册的处理器收不到任何 Span，初始化会打一条 debug 告警

### 5. 使用示例

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

### 6. 自动集成

通过 XOne 运行的应用会自动初始化 Trace，无需手动调用。以下模块已自动集成：

| 模块 | 集成方式 |
|-----|---------|
| xhttp | HTTP 请求自动创建 Span |
| xgorm | 数据库操作自动创建 Span |
| xlog | 日志自动关联 TraceID/SpanID |

### 7. Header 透传的边界

- **`ForwardHeaders`（全局）向所有下游域名注入**，敏感 Header 请放进 `ForwardHeaderRules`
- 同一个 Header 同时出现在两处属于误配：**以更严格的域名规则为准**，不会全局透传
- `ForwardHeaderRules` 的域名匹配依赖请求目标 Host 被写入 context，这由 `xtrace.HostAwareTransport`
  完成，而它只在 **xhttp 创建的 client** 上自动挂载。用户自建 `http.Client` 时规则不生效，
  需自行包一层 `&xtrace.HostAwareTransport{Next: ...}`
- 只有从上游请求 Extract 到的值才会向下游 Inject；框架不提供手动写入透传值的 API

### 8. 注意事项

- `Enable=false` 时使用 NoopTracerProvider，不产生链路开销；**但已配置的 Header 透传仍然生效**
- `EnableConsole=true` 仅用于本地调试，生产环境建议关闭
- 关闭链路请用 `Enable=false`，不要把 `SampleRatio` 设为 0（0 视为未配置，回落到全采样）
- 关闭等待上限由 `XTrace.ShutdownTimeout`（默认 5 秒）控制，取值应小于 xhook 单个 Hook 的超时（默认 10 秒）：
  否则是 xhook 先放弃等待，留下一个仍在阻塞的 goroutine
