# 更新日志

这里记录每个版本里**使用者看得见的变化**：新功能、行为变化、不兼容变更、修复、性能。
仓库内部的重构和工具改动不写，除非它改变了使用者要做的事。

格式参考 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)，版本号遵循
[语义化版本](https://semver.org/lang/zh-CN/)。所有模块共用同一个版本号（见 `scripts/release.sh`）。

## [未发布]

### 新增

- 新模块 xkafka（franz-go v1.21.7）：`xkafka.C()` 是生产用的原生 `*kgo.Client`，链路上下文自动写进消息头，退出时先 `Flush` 再关；`xkafka.Consume(topic, group, fn)` 登记消费者，每个分区一个协程、分区内按顺序，处理完才提交（至少一次），失败按退避重试、用完写进 `<topic>.dlq`（`xutil.Permanent` 不重试），panic 被接住，再均衡和退出时等在途的消息处理完、提交之后才交出分区；新消费组默认从 latest 开始（franz-go 默认 earliest）。

## [v1.23.0] - 2026-10-01

### 新增

- 新模块 xcron：进程内的定时任务，`xcron.Add("*/5 * * * *", fn)` 登记，Web 服务和只跑定时任务的进程（`xone.MustRun(xone.UntilSignal())`）都能用；每次执行带根 Span `cron <name>` 和 `job` 日志字段，上一次没跑完时跳过，panic 被接住，退出时取消并等在途的执行返回之后才关客户端；`RunOnStartAndWait` 让第一次跑不成功时启动失败。默认按 UTC，多副本时每个副本都会跑。

## [v1.22.1] - 2026-10-01

### 修复

- 安全：xredis 链路的 Span 名不再带命令参数：redisotel 拿第 1 个参数当 Span 名，`Do(ctx, "SET k1 <值>")` 的 Span 名原来是 `set k1 <值>`、pipeline 里是 `redis.pipeline set k2 <值>`；现在不像命令名的记成 `<invalid>`（规则同命令日志的 `cmd` 字段），合规的命令名和 `redis.dial` 不变。
- xgorm/clickhouse：`DialTimeout: 0` 且 DSN 里没写 `dial_timeout` 时，启动建连探测的单次预算按驱动实际用的 30s 算（60s），不再只有 1s 兜底，慢一点但合法的建连不再在启动时失败。
- `XONE_DEBUG` 打出的最终配置：`${VAR:默认值}` 的默认值里带凭证查询参数时，遮挡不再把收尾的 `}` 和后面的内容一起吞掉。

## [v1.22.0] - 2026-10-01

### 不兼容变更

- 配置文件里用 `---` 隔开的多份 YAML 文档现在启动失败，报 `multiple YAML documents in one file are not supported ... put per-environment settings in application-{profile}.yml instead`；原来只读第一份，后面的整段静默丢掉（也不算「没人读的配置块」）。开头一个 `---`、结尾多写的空 `---` 照常加载。迁移：把 `---` 之后的内容并进第一份；按环境区分的那几段挪进 `application-{profile}.yml`，用 `XApp.Profiles` / `--profile` 激活。
- xgin 关掉了 gin 的 `RedirectTrailingSlash`：只注册了 `/users` 时，`/users/` 不再被 301 / 307 重定向到 `/users`，而是 404，并且和别的 404 一样进访问日志、指标和链路（`route` 是 `unmatched`）；原先重定向的请求不经过任何中间件，哪里都看不到。依赖这个重定向的：两条路由都注册（`e.GET("/users", h)` 和 `e.GET("/users/", h)`），或者在 `WithRoutes` 里设回 `e.RedirectTrailingSlash = true`（重定向的请求照旧不进访问日志、指标和链路）。

### 修复

- 安全：xgorm 的 MySQL 开着 `TLS:` 块时，DSN 里的 `allowFallbackToPlaintext=true` 会让驱动在服务端（或中间人）不报 TLS 能力时改走明文、把登录包和之后的 SQL 明文发出去；现在 DSN 里写了这个参数直接启动失败（`the DSN sets allowFallbackToPlaintext while the TLS block is enabled`），交给驱动的连接配置里也钉死不退回明文。迁移：把这个参数从 DSN 里删掉。
- xhttp：body 是 `io.Reader` 的请求（`SetBody(strings.NewReader(…))` 这类）不再重试——第一次尝试就把它读完了，重试发出去的是空 body，服务端回 200 时调用方看到的是成功；现在调用方拿到第一次的错误。要重试就传 `[]byte` / `string`，或 `SetContentLength(true)`。
- xhttp：`RetryOnlyIdempotent` 开着时，自己 `AddRetryCondition` 挂的条件不再能让没拿到响应的 POST / PATCH 重试；按状态码（比如 5xx）重试的条件照旧生效，那种条件里的方法要自己判断。
- xredis：链路里出错的 Span 不再带服务端的错误原文（`ERR unknown command 'foo', with args beginning with: '<参数>'`、`EVAL` 里 `error_reply(ARGV[1])` 的值），状态描述和命令日志的 `error` 字段一样只写错误码，`exception` 事件只记网络错误、超时这类。
- xgorm：`SQL failed` 日志和 Span 里不再带客户端一侧错误里的参数值（pgx 编码不了的参数会被整个写进错误，`database/sql` 扫描失败会引出读回来的值）；网络错误、ctx 取消 / 超时、`record not found` 这类照原文记，其余写 `client error (message omitted, it may contain parameter values)`。返回给调用方的错误不变。
- xgorm 的 MySQL：`MySQL.ReadTimeout: 0`（或 `DialTimeout: 0`）时启动建连探测的单次预算不再只剩另一个超时，配成 0 的那一段按另一段算（`ReadTimeout: 0`、`DialTimeout: 500ms` 是 1s）。
- xredis：时长配 0 时（go-redis 换成它的默认值 `DialTimeout` / `ReadTimeout` 5s）启动建连探测的预算按换算之后的值算，不再只有 1s 兜底，慢一点但合法的建连不再在启动时失败；配置文档写明了每个时长配 0 的含义。
- xgorm/clickhouse：库名写成 `?database=` 时，建连日志和 Span 里的 `db` 是真正连上的那个库，不再是空的或路径里被盖掉的那个。
- `xone.Run`：收到退出信号后，服务的 `Stop` 恰好在截止时间返回、`Start` 紧跟着带错误返回时，`Start` 的错误约有一半的概率丢掉，还多打一条 `server did not exit within its share of the stop budget` 告警；现在错误照常返回，服务已经退出时不再告警。
- profile 文件和 import 进来的文件里用合并键 `<<: *x` 写的值，现在照常压过低优先级文件里写明的同名 key；原来输给了它们（YAML「写明的压过并进来的」被套到了跨文件合并上）。同一个文件里的 `<<` 规矩不变。
- `XONE_DEBUG` 打出的最终配置：凭证 key 下的列表和 map 整个遮成 `***`（原来只遮标量）；密码里带 `@`、`/` 的 URL / MySQL DSN 遮到最后一个 `@`（原来后半截连同主机原样打出）；查询串里的 `token=`、`api_key=`、`access_token=` 这类也遮（原来只遮 `password=` / `passwd=` / `pwd=`）；来自 `${VAR}` 的值显示原文 `${VAR}`，不再显示展开出来的值。
- 没设置的环境变量报错（`environment variables not set: ...`）按名字排序、每个只列一次；原来同一个变量用了几处就列几遍。
- `--config`、`--profile` 写在命令行最后却没带值时启动失败（`--config needs a value`）；原来被静默忽略，接着按 `XONE_CONFIG`、约定路径找文件。
- `xconfig.Unmarshal`、`xconfig.DecodeStrict` 传了非指针或 nil 时返回 `xconfig` 的 `config` 错误（`decode target must be a non-nil pointer`），不再 panic；`Unmarshal` 在这一块没配时也照样报。
- `xlog.Location()`：同一个进程里后一次 `Run` 没配 `XLog.Timezone`（或用了 `xlog.UseHandler`）时回到 `time.Local`；原来一直留着上一次配的时区。
- xgorm、xredis、xcache 多实例建到一半失败时，回头关已建好的实例失败的错误并进启动错误里返回；原来被丢掉。
- xgin / xecho 的 `Stop`：劫持了连接的 handler（WebSocket、自己 `Hijack` 的）在 `Shutdown` / 强制断连之后看得到请求 ctx 取消，原先等满整份停止预算再报 `1 handler(s) still running`；handler 的读循环要看 `c.Request.Context()`（xecho 是 `c.Request().Context()`）才退得出来。
- xgin / xecho 的 `Start`：监听失败（端口被占、证书读不出来）之后可以再调 `Start`，不再一直报 `server is already running`。
- xgin 里把 `*gin.Context` 当 `context.Context` 传（`xlog.AddKV(c, ...)`、`db.WithContext(c)`、`otel.Tracer(...).Start(c, ...)`）不再丢掉日志作用域、父 Span 和取消：engine 开了 `ContextWithFallback`。
- 访问日志的 `response_headers` / `request_headers`：`Location`、`Content-Location`、`Refresh` 和 `Referer` 那几个一样去掉查询串和片段（OAuth 回调的 `?code=`、`#access_token=`），这几个 URL 头里的 userinfo 也去掉；别的头的值里夹着的 DSN 密码（`postgres://app:pw@db`、`app:pw@tcp(db:3306)`）遮掉，其余照原样（`Vary: Cookie` 不受影响）。
- 访问日志的 `request_body` / `response_body`：Content-Type 是 `text/plain` 或者没带、内容却是 JSON 的 body 按 JSON 字段脱敏（gin 的 `ShouldBindJSON` 不看 Content-Type），原先用 Unicode 转义写的键名认不出来、原样进日志；JSON 的字符串值里夹着的 DSN 密码也遮掉了。
- xgin / xecho 抓 `MetricPath` 的请求不再开 Span（抓取系统按秒轮询，原先每次一个 Span）。
- xgin 在 `WithRoutes` 里设了 gin 自己的 `e.UseH2C = true` 时不再绕过优雅退出：服务交给 net/http 的是 engine 本身，h2c 只看 `XGin.UseH2C`。
- xgin / xecho 的 Recover：客户端断开导致的 `connection broken` 从 ERROR 改记 WARN。
- xgin / xecho 没配证书时不再校验 `TLS.MinVersion`（用不上的值不再让服务起不来）；配了证书时 `MinVersion` 留空按 `"1.2"`，原先是启动失败。

## [v1.21.0] - 2026-09-30

### 不兼容变更

- xflow 默认监控把成功步骤的 `xflow step process done` / `xflow step rollback done` 从 DEBUG 改记 INFO：开着 `XFlow.Monitor`（默认开）时，默认日志级别下就能看到每一步，一个 N 步的流程每次执行多写 N 行。不想要逐步日志的：日志级别设成 warn（失败的步骤照样记 WARN），或者 `XFlow.Monitor: false`，或者用 `xflow.SetMonitor` 换成自己的实现。

### 修复

- xhttp 的 `Log: true`：重定向策略拒绝时（`NoRedirectPolicy`、`FlexibleRedirectPolicy` 用完、`Location` 转义不合法）`error` 里带着 `Location` 的查询串，查询串里有没转义的空格时空格之后的部分也留在日志里；现在这两种都去掉了，resty 自己的日志（`xhttp resty log`）同样。
- xhttp 的 `Log: true` 不再替换每个请求的 resty logger：自己 `client.SetLogger(…)` / `R().SetLogger(…)` 的，resty 的提醒（比如明文 HTTP 上用 Basic Auth）重新交给你的 logger。
- xhttp 的请求日志：一次都没发出去就失败的请求（比如 `SetSRV` 查不到）`elapsed_ms` 记 `0`，不再是 `9223372036854.775`；`path` 改记转义过的形式（`/a%2Fb` 不再记成 `/a/b`）。
- xredis 的 `Log: true`：`cmd` 字段不像命令名时记成 `<invalid>`（`Do(ctx, "SET k1 <值>")` 原先把整条命令连同值记进 `cmd`），pipeline 的 `cmds` 同样。
- xredis 的 `Log: true`：WATCH 冲突（`redis.TxFailedErr`）不再记成 `redis pipeline failed` 的 WARN，改为 INFO 的 `redis pipeline`、带 `tx_failed: true`。
- xredis 的 `Log: true`：超过 256 字节、又不是 UTF-8 的 key 不再截成空串。
- 文档更正：xredis 的命令日志只对服务端报的错省略原文，超时、`context canceled`、`redis: client is closed` 这类客户端一侧的错误照原文记；key 是整条记的，开着 `Log` 时别把令牌、手机号、邮箱拼进 key。

## [v1.20.0] - 2026-09-30

### 不兼容变更

- 删除 `xutil.GetOrDefault` 和 `xutil.ToPtr`。迁移：`xutil.GetOrDefault(v, d)` 换成标准库的 `cmp.Or(v, d)`（Go 1.22 起，行为完全相同，还能一次传多个候选值）；`xutil.ToPtr(x)` 在 `go.mod` 声明 `go 1.26` 及以上时换成 `new(x)`，更低的版本写成 `v := x` 再取 `&v`。

## [v1.19.0] - 2026-09-30

### 新增

- xredis 新增配置项 `Log`（默认 false）和 `SlowThreshold`（默认 100ms，需 `Log` 开启，0 不记，负数启动失败）：每条命令一行 `redis command`（pipeline 整个一行 `redis pipeline`），带实例名、命令名、第一个 key 和耗时，不记值和其余参数；key 不存在（`redis.Nil`）记 INFO 并带 `nil: true`，失败和慢命令记 WARN，失败时只记服务端的错误码。
- xhttp 新增配置项 `Log`（默认 false）和 `SlowThreshold`（默认 1s，需 `Log` 开启，0 不记，负数启动失败）：每个逻辑请求在重试结束后记一行 `http request`，带方法、host、路径、状态码、耗时和尝试次数，不记查询串、Header、body；传输层错误和 5xx 记 WARN（`http request failed`），开着时 resty 在重试路径上的 `xhttp resty log` 不再重复打。

## [v1.18.0] - 2026-09-30

### 不兼容变更

- XGin / XEcho 的 `CertFile`、`KeyFile`、`ClientCAFile`、`MinVersion` 挪进 `TLS:` 块，和 XGorm / XRedis / XHttp 客户端那一侧的 `TLS:` 同一个样子（服务端没有 `Enable`，照旧是证书配了就开），这几项的校验报错也跟着带上 `TLS.` 前缀（如 `TLS.CertFile and TLS.KeyFile must both be set or both be empty`）。
  旧的平铺写法启动失败，报 `field CertFile not found in type xgin.Config (did you mean TLS.CertFile? move it under TLS:)`。迁移：在 `XGin:` / `XEcho:` 下加一行 `TLS:`，把这四行缩进到它下面；Go 代码里 `c.CertFile` 改成 `c.TLS.CertFile`（其余三项同理）。

### 新增

- 配置里的 key 写高了一层（它其实是某个子块的字段）时，报错直接指出该挪到哪，如 XRedis 顶层写了 `CAFile` 会提示 `did you mean TLS.CAFile? move it under TLS:`；几个子块都有这个字段时不猜。

## [v1.17.0] - 2026-09-29

### 新增

- xgorm 新增配置项 `DisableForeignKeyConstraintWhenMigrating`（默认 false）：设为 true 时 `AutoMigrate` 建表不建外键约束，查询和 `Preload` 不受影响；已经建好的外键不会被删。
- xgorm 新增配置项 `SkipDefaultTransaction`、`PrepareStmt`（默认都是 false）和 `CreateBatchSize`（默认 0，负数启动失败），原样交给 GORM，默认值与 GORM 相同，不配的行为不变。

## [v1.16.0] - 2026-09-28

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
