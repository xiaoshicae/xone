## XHttp 模块

### 1. 模块简介

* 对 [go-resty](https://github.com/go-resty/resty) 进行了封装，版本参见 go.mod 文件
* 支持链路追踪（OpenTelemetry）
* 提供原生 `http.Client` 用于流式请求场景（如 SSE）
* 线程安全的客户端访问

### 2. 配置参数

```yaml
XHttp:
  Timeout: "60s"             # HTTP 请求超时时间 (optional, default "60s")，支持 "d" 格式如 "1d"
  DialTimeout: "30s"         # 建立 TCP 连接超时时间 (optional, default "30s")
  DialKeepAlive: "30s"       # TCP keep-alive 探测间隔 (optional, default "30s")
  MaxIdleConns: 100          # 最大空闲连接数 (optional, default 100)
  MaxIdleConnsPerHost: 10    # 每个 host 最大空闲连接数 (optional, default 10)
  IdleConnTimeout: "90s"     # 空闲连接超时时间 (optional, default "90s")
  RetryCount: 3              # 重试次数 (optional, default 0，不重试)
  RetryWaitTime: "100ms"     # 重试等待时间 (optional, default "100ms")
  RetryMaxWaitTime: "2s"     # 最大重试等待时间 (optional, default "2s")
  RetryOnlyIdempotent: true  # 只对幂等方法重试 (optional, default true)
  EnableMetric: true         # 是否启用出站请求 Prometheus 指标采集 (optional, default true)
```

### 3. 使用 demo

* 配置:

```yaml
XHttp:
  Timeout: "10s"
  MaxIdleConns: 200
  MaxIdleConnsPerHost: 20
```

* 获取 resty client 并使用，详细请参考 [go-resty](https://github.com/go-resty/resty):

```go
package main

import (
  "context"
  "fmt"
  "github.com/xiaoshicae/xone/v3/xhttp"
)

func main() {
  ctx := context.Background()

  // 推荐：使用 RWithCtx 保证 traceId 传递到下游
  resp, err := xhttp.RWithCtx(ctx).Get("https://httpbin.org/get")

  // 处理 response
  fmt.Println("Response Info:")
  fmt.Println("  Error      :", err)
  fmt.Println("  Status Code:", resp.StatusCode())
  fmt.Println("  Status     :", resp.Status())
  fmt.Println("  Proto      :", resp.Proto())
  fmt.Println("  Time       :", resp.Time())
  fmt.Println("  Received At:", resp.ReceivedAt())
  fmt.Println("  Body       :\n", resp)

  // 也可以通过 C() 获取 resty client（不推荐，建议使用 RWithCtx）
  client := xhttp.C()
  resp, err = client.R().SetContext(ctx).Get("https://httpbin.org/get")
}
```

* 使用原生 `http.Client`（适用于 SSE 流式请求等场景）:

```go
package main

import (
  "bufio"
  "fmt"
  "github.com/xiaoshicae/xone/v3/xhttp"
)

func main() {
  // 获取原生 http.Client，用于需要直接操作 response body 的场景
  // 注意：必须在 xone 启动后调用，否则会打印警告日志并返回 http.DefaultClient
  rawClient := xhttp.RawClient()

  resp, err := rawClient.Get("https://api.example.com/sse")
  if err != nil {
    panic(err)
  }
  defer resp.Body.Close()

  // 流式读取（如 SSE）
  scanner := bufio.NewScanner(resp.Body)
  for scanner.Scan() {
    fmt.Println(scanner.Text())
  }
}
```

### 4. 出站请求指标

默认启用 Prometheus 出站请求指标采集（需配合 xmetric 模块），自动记录所有 HTTP 客户端请求：

- `http_client_requests_total{method, host, status}` — 请求总数
- `http_client_request_duration_seconds{method, host, status}` — 请求耗时（秒）

关闭方式：

```yaml
XHttp:
  EnableMetric: false
```

### 5. 重试与幂等

`RetryCount > 0` 时默认只重试**幂等方法**（GET / HEAD / OPTIONS / TRACE / PUT / DELETE）。

原因是传输层超时无法区分两种情况：请求根本没到服务端，还是服务端已经处理完了、
只是响应在回程丢了。重发一个 POST，后一种情况就变成了重复下单、重复扣款。

确认接口本身幂等（比如带幂等键）后可以关掉这个限制：

```yaml
XHttp:
  RetryCount: 3
  RetryOnlyIdempotent: false   # 允许所有方法重试
```

重试只在传输层出错时触发；拿到响应（哪怕是 5xx）不会重试。

### 6. 注意事项

- 所有 API 都是线程安全的
- `C()` 与 `RawClient()` 在 xone 未启动或已关闭时返回**带 30s 兜底超时**的 client
  并打印警告。不用 `http.DefaultClient` 是因为它的超时是 0 —— 对端不响应时请求会
  一直挂着，出现在 BeforeStop hook 里就会卡住整个进程的退出流程
- 推荐使用 `RWithCtx(ctx)` 以确保链路追踪信息正确传递
- 时间配置支持 "d"（天）格式，如 `"1d12h"` 表示 1 天 12 小时
- 出站 trace 的 span 名是 `METHOD /path`，用的是**实际路径**而非路由模板。
  访问 `/user/123` 这类含 ID 的接口会产生高基数 span 名，链路后端上会看到大量
  只出现一次的 span。指标侧不受影响（label 只有 method/host/status，不含 path）；
  需要模板化时用 `otelhttp.WithSpanNameFormatter` 自行包装 transport
