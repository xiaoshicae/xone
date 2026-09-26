# xhttp

出站 HTTP 客户端：拿到的是原生 `*resty.Client`（resty v2），任何时候都有一个可用的实例，不配也能用。

- 链路已装好：`traceparent` 自动带给下游，Span 名只用方法
- 出站耗时指标 `http_client_request_duration_seconds`，一次逻辑请求记一次
- 只重试传输层的错，默认只重试幂等方法
- 日志和 Span 里的 URL 去掉查询串；没有 cookie jar
- 要第二套参数（比如调公网、关指标）就用 `xhttp.New` 另建一个

## 快速上手

```yaml
# conf/application.yml（可选，不写就全用默认值）
XHttp:
  Timeout: 1s              # 一次尝试的超时
  RetryCount: 2            # 只重试传输层的错，且只重试幂等方法
```

```go
// Now 是下游返回的 JSON 对应的结构体
func Current(ctx context.Context, city string) (*Now, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second) // 给整次请求封顶：重试和退避都算在里面
	defer cancel()

	var out Now
	resp, err := xhttp.R(ctx).
		SetQueryParam("city", city).
		SetResult(&out). // 2xx 时把 JSON 解进 out
		Get("https://weather.internal/api/v1/now")
	if err != nil {
		return nil, err
	}
	if resp.IsError() { // 4xx / 5xx 不是 err，要自己判断
		return nil, fmt.Errorf("weather api returned %s", resp.Status())
	}
	return &out, nil
}
```

## 配置

不配也能用：`xhttp.R(ctx)` 任何时候都有一个可用的客户端（`Run` 之前和之后是按默认值建的兜底实例）。只有单实例。

```yaml
XHttp:
  Timeout: 60s               # 一次尝试的超时，不是整次逻辑请求；0 = 永不超时
  DialTimeout: 30s
  DialKeepAlive: 30s         # 负数 = 不发 keep-alive 探测
  MaxIdleConns: 100
  MaxIdleConnsPerHost: 10    # 标准库默认只有 2
  MaxConnsPerHost: 0         # 每 host 连接数上限，0 = 不限
  IdleConnTimeout: 90s       # 要小于下游的 keep-alive 超时
  RetryCount: 0              # 默认不重试
  RetryWaitTime: 100ms       # 重试的起始等待
  RetryMaxWaitTime: 2s       # 重试等待的上限
  RetryOnlyIdempotent: true  # 只重试幂等方法
  Trace: true                # 出站 Span；关掉后 traceparent、baggage、透传 Header 照样带给下游
  Metric: true               # 出站耗时指标
  TLS:                       # 规则见 xtls；管这个客户端发出的每一个 https 请求，http:// 不受影响
    Enable: false
    CAFile: ""               # 填了就只认它：公网的 https 下游从此校验不过
    CertFile: ""
    KeyFile: ""
    ServerName: ""           # 填了就拿它比对每一个下游的证书
```

TLS 块适合「只调一类内部下游」的客户端；要同时调公网的，另用 `xhttp.New` 建一个。

## API

| 函数 | 说明 |
|---|---|
| `R(ctx) *resty.Request` | 开一个绑定了 ctx 的请求，链路和超时才传得到下游。常用的就是它 |
| `C() *resty.Client` | 取 resty client。不会 panic：初始化前、关闭后拿到的是带 30s 超时的兜底实例 |
| `RawClient() *http.Client` | 底层的原生 `*http.Client`，给 SSE 这类要自己读响应体的流式请求 |
| `New(cfg) (*resty.Client, io.Closer, error)` | 纯构造器：不碰本包的全局实例；`Metric` 开着时直方图注册到 xmetric 的全局 Registry |

## 注意事项

- **`Timeout` 管一次尝试，不是整次请求**：`Timeout: 300ms` 配 `RetryCount: 3` 实测跑满 1.24s。要封顶就像上面那样用 ctx。
  见[「行为与实测」](#行为与实测)。
- **只重试传输层的错**（建连失败、超时、连接被重置），拿到了响应就不重试，5xx 也不重试；`RetryOnlyIdempotent` 默认开着，POST 不重发。
- **设了 `HTTP_PROXY` / `HTTPS_PROXY` 就全部出站都走代理**；跨 host 重定向时自定义的凭证头（如 `X-Api-Key`）照样带给新 host。
  见[「行为与实测」](#行为与实测)的表。
- **TLS 块管这个客户端的每一个 https 请求**：填了 `CAFile` 公网的 https 下游就校验不过了。
- **指标的 `host` 标签是 URL 的 `host[:port]` 原样**：目标来自用户输入或直连一批 IP 时基数会失控，那种调用另建一个
  `Metric: false` 的客户端。见[「指标」](#指标)。

## 可观测

### 指标

| 指标 | 类型 | 标签 | 来源 |
|---|---|---|---|
| `http_client_request_duration_seconds` | histogram | `method`、`host`、`status` | xhttp，`XHttp.Metric`；一次逻辑请求记一次（含重试和退避），没拿到响应时 `status` 是 `0` |

- **`host`** 是出站请求 URL 的 `host[:port]`，原样照抄，基数等于你调过的目标数。每个新值乘上 `method` × `status`，
  每个组合 15 条时间序列（默认 12 个桶加 `+Inf`、`_sum`、`_count`）。目标来自用户输入或直连一批 IP 的调用另建一个
  `Metric: false` 的客户端。

指标名的前缀、常量标签和几条通用规则见 [`docs/observability.md`「指标」](../docs/observability.md#指标)。

### 链路

| 来源 | Span 名 | 关键属性 |
|---|---|---|
| xhttp（出站） | 只用方法，如 `GET` | otelhttp 的标准属性；`url.full` **去掉了查询串和片段** |

链路的全貌、传播与信任边界见 [`docs/observability.md`「链路」](../docs/observability.md#链路)。

## 行为与实测

实测环境和跨模块的总表见 [`docs/behavior.md`](../docs/behavior.md)。

resty v2.17.2、otelhttp v0.71.0、Go 1.25。

**`Timeout` 管一次尝试，不是一次逻辑请求。** 开了 `RetryCount` 之后最坏是 `(RetryCount+1) × Timeout` 加上几次退避：
`Timeout: 300ms` 配 `RetryCount: 3`，实测跑了 1.24s。要给整次逻辑请求封顶，用调用方的 ctx（`xhttp.R(ctx)`），
每次尝试和中间的退避都听它的。

**重试什么**：和 resty 自己的默认一致，只重试传输层的错（建连失败、超时、连接被重置、响应体没收全），
拿到了响应就不重试——5xx 也不重试。挂上重试条件会让 resty 自己的判断整个作废，所以这条是 xhttp 自己守着的：
不守的话实测 200 + 坏 JSON、`RetryCount: 3` 时同一个 GET 发了 4 次。`RetryOnlyIdempotent` 默认开着：
传输层超时分不出「请求没到服务端」和「处理完了但响应丢了」，重发一个 POST 就可能重复下单。

**连接池没配的那些是标准库的默认**（从 `http.DefaultTransport` 克隆）：

| 项 | 默认 | 实测的行为 |
|---|---|---|
| 代理 | `ProxyFromEnvironment` | 设了 `HTTP_PROXY` / `HTTPS_PROXY` 就全部出站都走代理，`http://` 的请求连同查询串原样交给代理；回环地址不走，`NO_PROXY` 可以排除。环境变量第一次用到时读一次就缓存，之后再改不生效 |
| TLS 握手超时 | 10s | 对端收下 TCP 连接不回握手，10.0s 报 `TLS handshake timeout` |
| 等响应头 | 不限 | 由 `Timeout` 管住整次尝试；`Timeout` 也配 0 的话会永远挂着 |
| `MaxConnsPerHost` | 0，不限 | 对一个 300ms 才回的下游并发 200 个请求，它收到 200 条新连接；紧接着再来 200 个，只有 `MaxIdleConnsPerHost` 那 10 条复用得上 |
| 重定向 | 标准库：最多跟 10 次 | 第 11 次报 `stopped after 10 redirects`。跨 host 跳转时只去掉 `Authorization`、`Cookie` 这几个，**自定义的凭证头（如 `X-Api-Key`）照样带给新 host**；302 把 POST 变成不带 body 的 GET，307 保留方法和 body |
| HTTP/2 | `ForceAttemptHTTP2: true` | 换了 `TLSClientConfig`（TLS 块）之后照旧协商出 `HTTP/2.0` |

不想跟随重定向：`xhttp.C().SetRedirectPolicy(resty.NoRedirectPolicy())`。

**`IdleConnTimeout` 要小于下游的 keep-alive 超时**：对端先关掉空闲连接时，恰好在那一刻复用它的请求会失败。
实测服务端空闲超时 200ms、请求间隔在 200ms 上下：300 个 POST 失败 27 个（`connection reset by peer`），
GET 由标准库自动在新连接上重发，200 个一个没失败。

**跟 resty / otelhttp 默认不一样的地方：**

| | 库自己的默认 | 这里 |
|---|---|---|
| resty 的日志 | 写 `os.Stderr`；开了重试后每次失败打一行 `WARN RESTY Get "http://…?token=…": …, Attempt 1`，用完再打一行 ERROR | 接到 slog（`xhttp resty log`，内容在 `detail`），级别照搬，URL 去掉查询串 |
| cookie jar | `resty.New()` 自带一个，同一 client 的所有请求共享会话 cookie | 没有（用 `NewWithClient`）。初始化前 / 关闭后的兜底实例也没有 |
| 出站 Span 名 | 常见写法是 `GET /users/42`，基数随 id 增长 | 只用方法 `GET`（OTel 语义约定在没有路由模板时的写法） |
| `url.full` | otelhttp 只去掉 `user:password`，查询串原样写进去 | 查询串和片段一并去掉 |
| `CloseIdleConnections` | otelhttp 的 Transport 没实现，`http.Client.CloseIdleConnections()` 断在它那一层，整条调用变成空操作 | 关闭时 xhttp 直接关它自己持有的连接池 |
