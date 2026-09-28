# 更新日志

这里记录每个版本里**使用者看得见的变化**：新功能、行为变化、不兼容变更、修复、性能。
仓库内部的重构和工具改动不写，除非它改变了使用者要做的事。

格式参考 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)，版本号遵循
[语义化版本](https://semver.org/lang/zh-CN/)。所有模块共用同一个版本号（见 `scripts/release.sh`）。

## [未发布]

### 不兼容变更

- `xlog` 不再跟着 `xone` 根包来，改为跟着 xgin / xecho 来：只用核心（`xone.Func`、`xone.UntilSignal`）或只用 xgorm、xredis、xhttp、xcache 的程序，
  框架不再替你换掉 `slog.Default()`，日志保持标准库默认（stderr 上的 `2026/09/28 14:00:00 INFO msg k=v`），`xlog.AddKV` 的字段、`trace_id`、
  `log_errors_total` 计数也随之没有；这时写了 `XLog` 块启动失败，报错里给出要加的 import。用 xgin / xecho 的程序不受影响。
  迁移：这类程序要继续用 xlog，在 `main` 包加一行 `import _ "github.com/xiaoshicae/xone/xlog"`，行为和原来完全一样。

## [v1.15.1] - 2026-09-28

### 修复

- xgin / xecho 访问日志的 `errors` 字段、Span 的 `gin.errors` / `echo.errors`：没有敏感词的错误文本里，`postgres://app:pw@db`、`app:pw@tcp(db:3306)` 这类 URL / MySQL DSN 的密码现在换成 `***REDACTED***`（用户名和主机留着）；原来原样记录。
- xgin / xecho 的 `panic while handling request` 日志：`error` 字段（panic 的值）按 `errors` 字段同样的规矩脱敏，原来原样记录；`stack` 只有函数和行号，照旧原样记。
- xgin 访问日志的 `errors` 和 Span 的 `gin.errors`：`c.Error` 登记了多条错误时用 `; ` 隔开（`Error #01: a; Error #02: b`），原来换行被直接删掉、几条错误粘成一句。
- `&xgin.XGin{}`、`&xecho.XEcho{}` 这样不经 `New()` 的零值重新和 `New()` 一样：`Start` / `Stop` 的错误记在 `xgin` / `xecho` 名下（`xerror.Is(err, "xgin")` 成立），日志消息带着模块名（`xgin listening`）。
- xgin / xecho 的 `TrustedProxies` 写 IPv4 映射成 IPv6 的地址或网段（`::ffff:10.0.0.1`、`::ffff:10.0.0.0/104`）现在启动失败，报 `TrustedProxies entry ... is an IPv4-mapped IPv6 address; write it as ...`：原来 gin 把单个地址解歪（信的是 `::1` 这类地址），网段在 client_ip 和透传 Header 两处的判断也对不上。迁移：照报错改成 IPv4 写法，`::ffff:10.0.0.1` → `10.0.0.1`，`::ffff:10.0.0.0/104` → `10.0.0.0/8`（前缀长度减 96）。
- xgin / xecho 的 `<模块> listening` 日志改在证书读好、端口绑上之后才打：证书文件缺失或端口被占时只有那条 `listen on ... failed` 的错误，原来会先打一条 listening。
- xecho：带中间件的分组（`e.Group(prefix, mw...)`、`g.Use(...)`）下没匹配上的请求、落到 `e.RouteNotFound` 上的请求，访问日志的 `route`、指标的 `route` 标签、Span 名现在记 `unmatched`，和 xgin 一致；原来记成兜底的模板（`/api/v1/*`）。按 `route="/api/v1/*"` 查 404 的看板和告警改查 `unmatched`。
- xecho：内置中间件改挂在 `e.Pre` 上，在 `e.Pre` 里拒掉、重定向的请求现在也进访问日志、指标、链路并带 `X-Trace-Id`（`route` 记 `unmatched`），`e.Pre` 里的 panic 也被兜住、回 500；原来这些请求什么都不记，panic 时客户端读到 EOF。`WithMiddleware` 仍在 router 之后，`e.Pre(echomw.MethodOverride())` 照常生效；你自己的 `e.Pre` 看到的是请求原来的 ctx（那里打的日志不带 `trace_id`）。
- xecho：handler 返回（不是 panic）一个包着 `http.ErrAbortHandler` 的错误时按客户端实际收到的 500 记录，原来记成 499。
- xgin / xecho：客户端已经断开（或停止时被强制断连）、响应还一个字节都没发的请求，访问日志、指标、链路记 499，Span 不标错；原来 xecho 里 `return ctx.Err()` 记成 500 并标错，xgin 里什么都没写就返回的记成 `bytes_out` 为 0 的 200。按 5xx 或 200 统计这类请求的看板会看到它们挪到了 499。
- xgin / xecho 的 `LogResponseBody`：响应带着 `Content-Encoding`（gzip 等压缩中间件压过的）时 `response_body` 只记 `[gzip-encoded content omitted]`，原来记的是压缩后的二进制乱码。
- xecho 的 `LogResponseBody` 打开时，`c.Response().Writer` 现在满足 `http.Flusher` / `http.Hijacker` 的类型断言，原来只有经 `http.ResponseController` 或 `c.Response().Flush()` 才用得了。
- xgin / xecho：`http.Server` 自己报的错（TLS 握手失败等）现在记成 WARN、消息 `xgin http server error` / `xecho http server error`、原文在 `error` 字段，原来经标准库 log 记成 INFO、消息是那一整行；xecho 的 `e.StdLogger` 也接到这里，原来写 `os.Stdout`。
- xtrace 丢弃不可信对端的透传 Header / baggage 时的告警改成 `only peers in the web server's TrustedProxies (XGin / XEcho) are trusted`，原来只写 `XGin.TrustedProxies`，只用 xecho 的服务照着加一个 XGin 块会启动失败。按原文检索告警的，改按新文本匹配。

## [v1.15.0] - 2026-09-28

### 新增

- 新模块 xecho（`github.com/xiaoshicae/xone/xecho`）：基于 Echo v4 的 Web 服务，配置块 `XEcho`，用法和 xgin 一样（`xecho.New().WithRoutes(...)` 交给 `xone.Run`）；访问日志字段、指标名、Span 名、client_ip 规则、优雅退出与 xgin 相同，handler 返回的错误按客户端实际收到的状态码记录。

### 修复

- xgin / xecho 的访问日志 `errors` 字段和 Span 的 `gin.errors` / `echo.errors`：错误文本里出现敏感词（password、token、secret……，同 body 脱敏的词表）就整段记成 `***REDACTED***`。
  原来原样记录，驱动或下游报的错里夹带的凭证会进日志和链路后端。要看完整原因，在业务代码里自己记一条（并自行脱敏）。

## [v1.14.0] - 2026-09-27

### 不兼容变更

- 核心模块要求 Go 1.23+（原来是 1.22+；集成模块本来就要 1.25+）。迁移：把 Go 升到 1.23 或更高；
  或者保持默认的 `GOTOOLCHAIN=auto`（Go 1.21 起的默认值），go 命令会自己下载够新的工具链，
  `go get` / `go mod tidy` 同时把你 `go.mod` 里的 `go` 行抬到 `1.23.0`。设了 `GOTOOLCHAIN=local` 的环境（常见于 CI）只能升级 Go。

### 修复

- xgorm/clickhouse 往 DSN 里补 `dial_timeout` 时不再改写你写的其余参数：原来整个 query 按参数名重排、`,` `/` 被转义成 `%2C` `%2F`，现在只在末尾接一段。
- xlog 的 `Level`、`Format` 写错时报的是配置错误：错误文本从 `xone xlog new failed, err=[unknown log level ...]` 变成
  `xone xlog config failed, err=[...]`（`Format` 同理），和 `Perm`、`Timezone` 写错时一致。
  迁移：告警或日志检索按 `xlog new failed` 匹配这两种错误的，改成 `xlog config failed`；建日志目录、打开日志文件失败仍是 `xlog new failed`。

## [v1.13.0] - 2026-09-27

### 新增

- xlog 每条日志默认带上 `service`（`XApp.Name`）、`version`（`XApp.Version`）、`hostname`、`pid`，和 Span 上的身份对得上；
  没配 XApp 的那两个就不写。实测每行多约 88 字节，时间和分配量不出差别。
  要去掉哪个，在 `XLog.Fields` 里写成空串。
- `XLog.Fields`：每条日志都带的静态字段，值可以用 `${VAR}` 从环境变量取（K8s 用 Downward API 注入的 Pod IP、节点名等）。

### 修复

- xgin 访问日志里，没匹配上路由（404）和方法不对（405）的请求 `bytes_out` 记成了 0：gin 默认的响应正文在中间件链跑完之后才写。
  现在由 xgin 在链里写同样的正文，响应一个字节不变，字节数记得上；`WithRoutes` 里自己设的 `NoRoute` / `NoMethod` 照样优先。
- 访问日志里被遮掉的查询串和表单字段写成 `***REDACTED***`，原来是转义之后的 `%2A%2A%2AREDACTED%2A%2A%2A`。

## [v1.12.0] - 2026-09-26

### 新增

- `xlog.UseHandler(h)`：日志改由你自己的 `slog.Handler` 写（zap 的 slog 桥、公司的日志 SDK……），在 `xone.Run` 之前调。
  `trace_id`、`AddKV` 的字段、错误日志计数和框架自己的日志都照样写进它；这时再写 `XLog` 块会启动失败。

### 修复

- 只用核心（`xone.Func`、`xone.UntilSignal`，一个集成都没 import）的程序，写了 `XLog` 块会启动失败
  （`config keys [XLog] are not read by anyone`），不写则日志停在标准库的默认输出。现在 `xlog` 跟着框架一起来，
  和 `xapp` 一样不用另外 import；原来为此写的 `_ "github.com/xiaoshicae/xone/xlog"` 可以删掉，留着也无妨。

## [v1.11.0] - 2026-09-26

### 不兼容变更

- xgin 访问日志默认不再记请求头（字段 `request_headers`），和查询串、body、响应头一样由开关控制。
  迁移：要保留原来的输出，配 `XGin.LogRequestHeaders: true`；直接用 `middleware.Log` 的，加 `middleware.WithHeaders(true, false)`。

### 新增

- xgin 访问日志可以记查询串和响应头：`XGin.LogQuery: true` 加字段 `query`（逐字段脱敏），
  `XGin.LogResponseHeaders: true` 加字段 `response_headers`（`Set-Cookie` 等凭证类脱敏）。默认都关。
- xgin 访问日志默认多记五个字段：`host`、`proto`、`user_agent`、`bytes_in`（请求的 `Content-Length`，分块上传是 `-1`）、
  `bytes_out`（响应体字节数）。实测每个请求多约 1µs，不多分配。
  词表里没有的敏感参数名（比如 OAuth 的 `code`）用 `middleware.AddSensitiveFields` 补上。

### 修复

- 日志里的耗时带上单位：xgin 访问日志、xgorm 的 SQL 日志、xflow 的流程 / 步骤日志的 `elapsed` 改名为 `elapsed_ms`，
  值是毫秒（保留到微秒，如 `0.051`）；xgorm 慢查询的 `threshold` 改名为 `threshold_ms`。原来 JSON 里是没有单位的纳秒整数。
  迁移：日志平台里按 `elapsed` / `threshold` 查询、告警、做看板的，换成新字段名，阈值按毫秒写。
- xgin 访问日志里没匹配上路由的请求，`route` 记成 `unmatched`，和指标、Span 一致；原来填的是请求路径，分不出是路由还是 404。

## [v1.10.0] - 2026-09-26

首个版本。所有模块同时发布、共用这个版本号。
版本号从 v1.10.0 起步：这个模块路径上更早的 v0.x / v1.0.0～v1.3.1 是另一份代码，已在 go.mod 里撤回（`retract`）。

- **核心** `github.com/xiaoshicae/xone`：
  - `xone.Run` / `MustRun`：按档位启动、逆序关闭，整个退出流程共用一份停止预算；`xone.Func` / `xone.UntilSignal`。
  - `xconfig`（懒加载、严格解码、`${VAR}` 占位符、`XApp.Profiles` 多环境、`XApp.Import` 拆文件）、`xhook`、`xerror`、
    `xlog`（`AddKV` / `CtxWithKV` 请求级字段）、`xapp`、`xflow`、`xtls`、`xutil`、`xonetest`。
  - `XONE_DEBUG=1`：启动时打出用了哪个配置文件、激活了哪些 profile、按优先级读了哪些文件、合并之后的完整配置
    （凭证遮成 `***`）和启动钩子的顺序。
  - 启动 banner：只在 stderr 是终端时打，带版本号。
- **集成**：`xgin`（默认只信私有网段的代理）、`xginswagger`、`xgorm`（MySQL / PostgreSQL）、`xgorm/clickhouse`、
  `xredis`、`xcache`（`Get[V]` 按类型取值）、`xhttp`、`xtrace`、`xmetric`。
