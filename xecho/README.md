# xecho

Web 服务：拿到的是原生 `*echo.Echo`（[Echo](https://echo.labstack.com/) v4），`xecho.New()` 就是交给 `xone.Run` 的 Runnable，
监听、优雅退出都是框架的事。和 [xgin](../xgin/README.md) 是同一套东西换了个框架：访问日志的字段、指标名、Span 名、
client_ip 信谁、退出时等不等 handler，两边一字不差（写在两者共用的 `internal/web` 里）。

- 访问日志（凭证逐字段脱敏）、链路、指标（自动挂 `/metrics`）、panic 恢复已经装好
- handler 返回的错误在中间件里就渲染成响应：404、405、500 在访问日志、指标、链路里记的是客户端真正收到的状态码和字节数
- 默认只信私有网段的代理（echo 自己默认全信）：负载均衡、Ingress、Pod 转发来的 `X-Forwarded-For` 照收，公网直连的伪造不了
- 慢连接有防线：`ReadHeaderTimeout` 10s、`IdleTimeout` 60s，写 0 启动失败（echo 自己的 `e.Server` 全是 0）
- HTTPS、双向认证、h2c 都在配置里开
- 停止时等在途请求做完，到点断连并报出还没返回的 handler 数
- echo 自己的日志（gommon，默认写 stdout）接到 slog

## 快速上手

```yaml
# conf/application.yml
XApp:
  Name: demo.user.api          # 链路里的 service.name
XEcho:
  Port: 8080
  LogSkipPaths: [/healthz]     # 健康检查不记访问日志
```

```go
import (
	"log/slog"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/xiaoshicae/xone"
	"github.com/xiaoshicae/xone/xecho"
)

func main() {
	xone.MustRun(xecho.New().WithRoutes(func(e *echo.Echo) {
		e.GET("/healthz", func(c echo.Context) error { return c.NoContent(http.StatusNoContent) })

		v1 := e.Group("/api/v1")
		v1.GET("/users/:id", func(c echo.Context) error {
			ctx := c.Request().Context() // 客户端断开、服务停止时跟着取消，往下游传它
			slog.InfoContext(ctx, "get user", "user_id", c.Param("id")) // 自动带上这次请求的 trace_id
			return c.JSON(http.StatusOK, map[string]string{"id": c.Param("id")})
		})
	}))
}
```

`curl localhost:8080/api/v1/users/1` 的响应头带 `X-Trace-Id`；`curl localhost:8080/metrics` 能看到
`http_requests_total`，`route` 标签是 `/api/v1/users/:id`。

## 配置

```yaml
XEcho:
  Host: "0.0.0.0"
  Port: 8080
  UseH2C: false            # 非 TLS 下启用 HTTP/2（只认先验知识，不支持 Upgrade: h2c）
  CertFile: ""             # 与 KeyFile 同时配或同时留空；配了就是 https
  KeyFile: ""
  ClientCAFile: ""         # 校验客户端证书的 CA；配了就是双向认证，需同时配证书
  MinVersion: "1.2"        # "1.2" / "1.3"，只在配了证书时生效
  ReadHeaderTimeout: 10s   # 慢连接攻击的主要防线，必须 > 0
  ReadTimeout: 0s          # 默认不限：限制它会打断大文件上传
  WriteTimeout: 0s         # 默认不限：限制它会打断 SSE、长轮询、大文件下载
  IdleTimeout: 60s         # 必须 > 0
  TrustedProxies: [private]  # 信任哪些直连对端的 X-Forwarded-For / X-Real-IP 和透传 Header；private = 回环 + 私有网段，[] = 谁都不信
  Log: true                # 访问日志
  LogSkipPaths: []         # 不记访问日志的路径：以 / 结尾的按前缀，其余精确匹配
  LogQuery: false          # 查询串进访问日志（逐字段脱敏），默认关
  LogRequestHeaders: false  # 请求头进访问日志（凭证类脱敏），默认关
  LogRequestBody: false    # 请求体进访问日志（逐字段脱敏），默认关
  LogResponseHeaders: false  # 响应头进访问日志（凭证类脱敏），默认关
  LogResponseBody: false   # 响应体进访问日志，默认关
  Trace: true              # 每个请求一个服务端 Span、回带 X-Trace-Id
  Metric: true             # 请求指标，并自动挂上 MetricPath
  MetricPath: /metrics     # 必须以 / 开头；Metric 开着时自动加进 LogSkipPaths
```

和 XGin 块相比少了 `Mode`、`MaxMultipartMemory`、`ZHTranslations` 三项：echo 没有进程级的运行模式，
multipart 的落盘阈值是写死的（见[「行为与实测」](#行为与实测)），validator 也不是它内置的。写了这三项启动失败。

**同一进程里的第二个服务**（比如内部管理端口）不读配置文件，用 `WithConfig` 给一份完整配置，从 `CurrentConfig()` 改起：

```go
c := xecho.CurrentConfig()
c.Host, c.Port = "127.0.0.1", 9090
admin := xecho.New().WithConfig(c).WithRoutes(adminRoutes)
```

- `private` 展开成 `127.0.0.0/8`、`10.0.0.0/8`、`172.16.0.0/12`、`192.168.0.0/16`、`100.64.0.0/10`、`::1/128`、`fc00::/7`。
  列表整体替换默认值：要再加网段就连 `private` 一起写（`[private, 203.0.113.0/24]`），写错的网段启动失败；
  内网里也有不可信的客户端（办公网、VPN 用户能直连服务）时别用 `private`，写确切的那几段，或者 `[]`。
- `ClientCAFile` 管整个端口：`/metrics` 同样要客户端证书。handler 里用 `c.Request().TLS.PeerCertificates` 看是谁。
- xgin 和 xecho 可以在一个进程里同时用：两块配置各是各的（`XGin` / `XEcho`），端口别撞上。

## API

| 函数 | 说明 |
|---|---|
| `New() *XEcho` | 创建一个服务（Runnable），什么都不读，配置在装配那一刻才取 |
| `(*XEcho).WithRoutes(f ...func(*echo.Echo))` | 注册路由，回调拿到原生 echo；可以调多次，按顺序生效 |
| `(*XEcho).WithMiddleware(m ...echo.MiddlewareFunc)` | 追加自定义中间件，排在所有内置中间件之后，对每个请求都生效（含 `/metrics` 和 404） |
| `(*XEcho).WithRecoverFunc(f func(echo.Context, any) error)` | 自定义 panic 之后的响应；返回的错误交给 `e.HTTPErrorHandler`，默认回 500 |
| `(*XEcho).WithConfig(c Config)` | 用这份完整配置起服务，不读配置文件里的 XEcho 块 |
| `(*XEcho).Engine() *echo.Echo` | 触发装配并返回原生 echo；之后再 `With...` 不生效 |
| `(*XEcho).Start(ctx)` / `(*XEcho).Stop(ctx)` | 监听 / 优雅停止，通常交给 `xone.Run`，不用自己调 |
| `CurrentConfig() Config` | 配置文件里 XEcho 那一块的拷贝，`Start` 之前任何时候调都行 |
| `DefaultConfig() Config` | 全部默认值；`WithConfig` 从头写时从它开始 |
| `middleware.AddSensitiveFields(...string)` | 追加访问日志的脱敏敏感词（body 和请求头一起生效；和 xgin 共用一张词表） |
| `middleware.AddSensitiveHeaders(...string)` | 追加精确匹配的敏感请求头名 |

## 注意事项

- **不要调 `e.Start` / `e.StartTLS`**：服务由 `Start` 起（`xone.Run` 替你调），超时、TLS、h2c、优雅退出都在那一边。
  echo 自己的 `e.Server` 四个超时全是 0。
- **handler 返回错误就行**：`return echo.NewHTTPError(http.StatusNotFound, "no such user")` 或者普通的 `error`，
  响应由 `e.HTTPErrorHandler` 写（默认是 echo 的 JSON `{"message":...}`，普通 error 一律 500、不带原文），
  要换格式就在 `WithRoutes` 里设 `e.HTTPErrorHandler`。错误原文进访问日志的 `errors` 和 Span 的 `echo.errors`。
  自定义的错误处理只会被调一次；响应已经写出去了（`c.Response().Committed`）再返回错误时，echo 默认的错误处理什么都不做。
- **请求体没有上限**：框架不替业务定，要限就用 echo 自带的中间件，比如 `e.Use(echomw.BodyLimit("10M"))`
  （`echomw` 即 `github.com/labstack/echo/v4/middleware`）。multipart 的落盘阈值写死 32MB，见[「行为与实测」](#行为与实测)。
- **handler 里的慢操作传 `c.Request().Context()`**：`Stop` 没有单独的超时，等在途请求最多到停止预算的 2/3
  （`xone.WithStopTimeout` 的 2/3，默认 10s），到点断开连接、取消请求的 ctx；不看 ctx 的 handler 停不下来，
  `Stop` 会报 `N handler(s) still running`。
- **`WithRoutes` 回调里的设置盖过配置**（如 `e.IPExtractor`、`e.HTTPErrorHandler`），但透传 Header 的可信判断只看配置里的
  `TrustedProxies`，两边要一起改就改配置。XEcho 块在装配（`Engine()` 或 `Start`）那一刻才读。
- **路由比 gin 严**：末尾斜杠严格匹配（`/a/` 是 404，gin 默认 301 到 `/a`）；HEAD 不自动走 GET（405）；
  路径参数在路由末尾时吃得下后面的 `/`（`/users/:id` 匹配 `/users/1/z`，`id` 是 `1/z`）。要放宽末尾斜杠用 `e.Pre(echomw.RemoveTrailingSlash())`。
- **中间件用 `e.Use`，别用 `e.Pre` 挂观测类的**：`Pre` 在 router 之前跑，那时还没有路由模板，错误也还没渲染。
- `e.Debug = true` 时 echo 的错误响应会带上错误原文（`{"message":...,"error":"..."}`），线上别开。

## 在负载均衡 / Cloudflare 后面

和 xgin 完全一样，见 [xgin「在负载均衡 / Cloudflare 后面」](../xgin/README.md#在负载均衡--cloudflare-后面)。
client_ip 的算法也一样：直连对端可信时先看 `X-Forwarded-For` 再看 `X-Real-IP`，从右往左找第一个不在 `TrustedProxies` 里的地址。
echo 自带的 `ExtractIPFromXFFHeader` 默认信任回环、链路本地和私有网段，规则与这里不同，别在回调里换成它，除非你就是要它的规则。

## 可观测

### 访问日志

每个请求结束时一条，消息 `request completed`，级别 INFO（`XEcho.Log` 开着时；`LogSkipPaths` 里的路径和指标端点不记）。
字段的名字、顺序、取值和脱敏规则与 xgin 相同（同一份实现），完整说明见 [xgin「访问日志」](../xgin/README.md#访问日志)：

| 字段 | 内容 |
|---|---|
| `method` | 请求方法，原样 |
| `route` | 路由模板，如 `/users/:id`；没匹配上路由（404）、方法不对（405）、echo 自动应答的 OPTIONS 都是 `unmatched`，真实路径看 `path` |
| `path` | 请求路径，**不带查询串** |
| `status` | 客户端收到的状态码（handler 返回的错误渲染之后的那个）；中止的请求记 `499` |
| `elapsed_ms` | 耗时，毫秒，保留到微秒 |
| `client_ip` | `c.RealIP()`：直连对端，或 `XEcho.TrustedProxies` 里的代理转发来的地址 |
| `host` / `proto` / `user_agent` | 请求的 `Host`、`HTTP/1.1` / `HTTP/2.0`、`User-Agent` |
| `bytes_in` | 请求头里的 `Content-Length`；分块上传记 `-1` |
| `bytes_out` | 写出的响应体字节数（`c.Response().Size`），不含响应头；错误响应也算 |
| `query` / `request_headers` / `request_body` / `response_headers` / `response_body` | 各自的开关打开时，脱敏规则同 xgin |
| `errors` | handler 返回的错误（`err.Error()`，如 `code=404, message=Not Found`）、中止或断连的原因；没有就不写 |
| `trace_id` / `span_id` | 有链路时 |

`middleware.AddSensitiveFields(...)` / `AddSensitiveHeaders(...)` 和 xgin 的同名函数写的是同一张表，用哪个都行。

panic 由 Recover 中间件记一条 `panic while handling request`（ERROR，带 `error`、`stack`、`path`、`method`）并回 500；
客户端提前断开导致的写失败记 `connection broken`，不打栈。

### 日志

| 消息 | 级别 | 字段 |
|---|---|---|
| `xecho listening` | INFO | `addr`、`tls`、`mtls`、`h2c` |
| `echo internal log` | echo 的级别（默认只有 ERROR） | `message`：echo 自己写的那一行，如错误响应写失败时的 `write: broken pipe` |

日志的全局约定见 [`docs/observability.md`](../docs/observability.md#日志)。

### 指标

和 xgin 同名、同标签、同桶，看板和告警两边通用：

| 指标 | 类型 | 标签 | 来源 |
|---|---|---|---|
| `http_requests_total` | counter | `method`、`route`、`status` | xecho，`XEcho.Metric` |
| `http_request_duration_seconds` | histogram | `method`、`route`、`status` | xecho，桶是 `XMetric.HTTPDurationBuckets` |

- **`route`** 是路由模板；没匹配上的是 `unmatched`，规则同访问日志。`method` 收敛到标准方法，别的记 `OTHER`。
- 同一个进程里 xgin 和 xecho 都用时，两边的请求记在同一组指标上，分不出是哪个服务的；要分开就给它们不同的路由前缀。

### 链路

| 来源 | Span 名 | 关键属性 |
|---|---|---|
| xecho（入站） | `GET /users/:id`：方法 + 路由模板；没匹配上是 `GET unmatched` | `http.request.method`（收敛过的）、`http.request.method_original`（原始值和收敛值不同时）、`http.route`、`url.path`、`http.response.status_code`、`echo.errors`（handler 返回的错误） |

5xx 和中止标成错误，4xx 不算。`XEcho.Trace` 开着时每个响应（包括 echo 渲染的错误响应）带 `X-Trace-Id`。
链路的全貌、传播与信任边界见 [`docs/observability.md`「链路」](../docs/observability.md#链路)。

### 499：中止的请求

handler 以 `http.ErrAbortHandler` 中止的请求在访问日志、指标、链路里记 499，规则同
[xgin「499：中止的请求」](../xgin/README.md#499中止的请求)。

## 行为与实测

实测环境和跨模块的总表见 [`docs/behavior.md`](../docs/behavior.md)。echo v4.16.0、gommon v0.5.0、Go 1.25 net/http。

**client_ip（`e.IPExtractor`）**：echo 的默认是 `nil`，`c.RealIP()` 退回老的写法——谁发来的 `X-Forwarded-For`、`X-Real-IP` 都信。
实测公网对端 `203.0.113.9` 发 `X-Forwarded-For: 1.2.3.4`，`c.RealIP()` 是 `1.2.3.4`；只发 `X-Real-IP: 5.6.7.8` 就是 `5.6.7.8`。
这里按 `TrustedProxies` 装上 `IPExtractor`（默认只信私有网段），算法照抄 gin v1.12.0 的 `ClientIP`，xgin 的测试逐条比对两边的结果。

**handler 返回的错误**在整条中间件链返回之后才由 `e.HTTPErrorHandler` 写成响应（`Echo.ServeHTTP` 的最后一步）。
包在外面的中间件拿到错误的那一刻，404、405、500 实测都是 `Status 200`、`Committed false`、`Size 0`——照读的话每个出错的请求
在访问日志、指标、链路里都是一次 `bytes_out` 为 0 的 200。所以内置的三个中间件在自己这一层就调 `c.Error(err)` 渲染、
再往外返回 `nil`：404 是 `{"message":"Not Found"}` 24 字节，405 是 `{"message":"Method Not Allowed"}` 33 字节、带
`Allow: OPTIONS, GET`，普通 error 是 500 `{"message":"Internal Server Error"}` 36 字节（错误原文不进响应），
`echo.NewHTTPError(418)` 是 `{"message":"I'm a teapot"}` 27 字节，都是 `application/json`，访问日志记的就是这些数。
`HTTPErrorHandler` 只跑这一次；照样返回错误的话 echo 会再调一遍，不查 `Committed` 的自定义错误处理会把错误响应写两遍。
handler 先写了 200 再返回错误时，echo 默认的错误处理见 `Committed` 就什么都不做，客户端收到的是那个 200，日志也记 200。

**路由模板**：没匹配上任何路由时 `c.Path()` 是空串；路径对、方法不对时 `c.Path()` 是那条路由的模板、handler 是
`MethodNotAllowedHandler`（405），方法是 OPTIONS 时 echo 自己回 204 和 `Allow`。后两种和 404 一样记 `unmatched`，
和 xgin（gin 在这两种情况下都没有模板）对得上。

**panic** echo 默认不兜：实测客户端读到 `EOF`（连接被 net/http 断掉），栈由 net/http 写进 stderr
（`http: panic serving 127.0.0.1:…: boom`），不经过 slog。这里的 Recover 记一条 ERROR 日志并回 500。

**echo 自己的日志**（gommon）默认写 `os.Stdout`，是它自己的 JSON：错误响应写失败（客户端已经断开）时实测留下
`{"time":…,"level":"ERROR","prefix":"echo","file":"echo.go","line":"492","message":"write: broken pipe"}`。
级别默认 ERROR，`Warn("response already committed")` 这类写不出来，`Print` 系列不看级别、总会写。
这里把它接到 slog（消息 `echo internal log`），级别沿用 echo 的。

**服务器**：`e.Server` 的 `ReadHeaderTimeout`、`ReadTimeout`、`WriteTimeout`、`IdleTimeout` 全是 0，banner 只在 `e.Start` 时打。
这里不用 `e.Start`，服务由和 xgin 共用的 `web.Server` 按配置起，超时、TLS、h2c、优雅退出的实测见
[xgin「行为与实测」](../xgin/README.md#行为与实测)。

**`MetricPath`** echo 不拒绝不以 `/` 开头的写法：`metrics` 注册成 `/metrics`，访问日志却跳不过它；留空注册在根路径 `/` 上，
业务再注册首页时后注册的那个悄悄盖掉前一个（不报错，也不 panic）。所以读配置时就失败。

**请求体**：没有上限，实测 64MB 的 body 照收。`c.FormValue`、`c.FormFile`、`c.Bind` 解析 multipart 时用写死的 32MB 落盘阈值
（`ParseMultipartForm(32 << 20)`，没有配置项）：一次 60MB 的上传，解析这一步实测分配 128MB（`TotalAlloc`）；
在中间件里先 `c.Request().ParseMultipartForm(8 << 20)` 的话是 32MB——请求只会被解析一次，先调的那个说了算。
框架不替你做这一步：提前解析会打断用 `c.Request().MultipartReader()` 流式读上传的 handler。

**响应字节数** `c.Response().Size` 数的是经 `c.Response()` 写出去的正文：`c.String(200, "hello")` 是 5；`c.JSON` 带一个换行
（`{"a":"b"}` 记 10）；分三次写、每次 `Flush` 的 `chunk` 是 15；`c.NoContent(204)` 是 0，`Status` 是 204。
绕过 `c.Response()`、直接写 `c.Response().Writer` 的不算在内。`?pretty` 查询参数会让 `c.JSON` 缩进输出（`{"a":"b"}` 变成 15 字节）。

**路由**：末尾斜杠严格匹配，`/a/` 是 404（gin 默认 301 到 `/a`）；只注册了 GET 时 HEAD 是 405，不像 net/http 的 `ServeMux` 那样自动走 GET；
路径参数在末尾时吃得下后面的 `/`，`/users/:id` 匹配 `/users/1/z`，`id` 是 `1/z`。框架都没改。

## 排错

| 错误原文 | 原因 | 怎么改 |
|---|---|---|
| `CertFile and KeyFile must both be set or both be empty`（XEcho） | 服务端证书只配了一半 | 两个都填，或者都留空 |
| `ClientCAFile requires CertFile and KeyFile, mutual TLS runs on top of TLS`（XEcho） | 配了双向认证却没配服务端证书 | 补上 `CertFile` / `KeyFile` |
| `field Mode not found in type xecho.Config`（`MaxMultipartMemory`、`ZHTranslations` 同理） | 照抄了 XGin 块 | 删掉这几项，理由见[「配置」](#配置) |

停止时的 `N handler(s) still running when the shutdown deadline passed` 见 [`docs/troubleshooting.md`「Runnable 与退出」](../docs/troubleshooting.md#runnable-与退出)。
