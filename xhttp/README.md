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
  "github.com/xiaoshicae/xone/v2/xhttp"
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
  "github.com/xiaoshicae/xone/v2/xhttp"
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

### 4. 注意事项

- 所有 API 都是线程安全的
- `RawClient()` 在 xone 未启动时会返回 `http.DefaultClient` 并打印警告日志
- 推荐使用 `RWithCtx(ctx)` 以确保链路追踪信息正确传递
- 时间配置支持 "d"（天）格式，如 `"1d12h"` 表示 1 天 12 小时
