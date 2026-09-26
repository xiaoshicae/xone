# 与底层库不同的默认值与实测

框架接的每个库都有自己的默认行为，其中不少和它的 README、和「大家以为的」不一样。
这份文档记两件事：**框架在哪里改了库的默认值、为什么**，以及**没改的那些实际是什么行为**。
每一条都是用代码量出来的，写着库的默认、这里的默认、实测的数字和当时的依赖版本——
升级依赖之后数字对不上，就说明行为变了，要重新量。

这里只留总表和跨模块的那一项（启动期建连探测），每个模块的实测在它 README 的「行为与实测」一节，
配置项在同一个 README 的「配置」一节（索引见 [`config.md`](config.md)），日志 / 指标 / 链路的字段见
[`observability.md`](observability.md) 和各模块 README 的「可观测」一节。

**怎么读**：先看[总表](#总表)——每一行是一处「库的默认 ≠ 这里的默认」，链接跳到那个模块的实测细节。

实测环境：没有特别说明的都是本机回环、Go 1.25；带 e2e 的是 `e2e/` 下真起进程、连真服务
（PG 16、MySQL 8.0.46、Redis 7.0.15、ClickHouse 24.8.14）量出来的。

- [总表](#总表)
- [启动期建连探测](#启动期建连探测)
- [各模块的实测](#各模块的实测)

## 总表

| 配置 | 库自己的默认 | 这里的默认 | 为什么 |
|---|---|---|---|
| [`XGin.TrustedProxies`](../xgin/README.md#行为与实测) | 全都信（`0.0.0.0/0`） | 只信私有网段（`private`） | 否则谁发 `X-Forwarded-For` 谁就是访问日志里的 `client_ip`，限流和审计跟着失效；私有网段是负载均衡、Ingress、Pod 所在的那一跳 |
| [`XGin.MaxMultipartMemory`](../xgin/README.md#行为与实测) | 32MB | 8MB | 它是落盘阈值不是请求体上限，堆开销约为它的三倍 |
| [`XGin.ReadHeaderTimeout: 0`](../xgin/README.md#行为与实测) | 退到 `ReadTimeout`（默认 0），即不限时 | 启动失败 | 发半个请求头就能一直占着连接 |
| [`XGin.UseH2C`](../xgin/README.md#行为与实测) | x/net 的 `h2c.NewHandler` | 标准库的 `Protocols.SetUnencryptedHTTP2` | 前者劫持连接，`Shutdown` 管不到在途请求 |
| [`XGorm.Log: false`](../xgorm/README.md#行为与实测) | 换成 GORM 自己的 stdout logger | 真的不打 | 那个默认实现带 ANSI 颜色直写 `os.Stdout` |
| [`XGorm.Log: true`](../xgorm/README.md#行为与实测) | 参数值代进 SQL 再记 | 只记带占位符的 SQL | 否则 `WHERE password = ?` 记下来的是真实的密码 |
| [`XGorm` 建连](../xgorm/README.md#行为与实测) | `gorm.Open` 自己 ping 一次 | 关掉，走框架的 ctx-aware 探测 | 它用自己的 context，退出信号和重试都管不到 |
| [`XGorm` MySQL / ClickHouse 查版本](../xgorm/README.md#行为与实测) | `Initialize` 里用 `context.Background()` 查 | 挪进建连探测 | 那是第一次建连，失败不重试、不听退出信号 |
| [`XGorm` MySQL `parseTime`](../xgorm/README.md#行为与实测) | `false` | DSN 没写时补 `true` | 否则 `DATETIME` 扫不进 `time.Time` |
| [`XGorm` 服务端错误原文](../xgorm/README.md#行为与实测) | 原样进日志和 Span | 只记错误码 | 原文带着参数值 |
| [go-sql-driver 的日志](../xgorm/README.md#行为与实测) | 标准库 log 写 `os.Stderr` | 接到 slog，记成 WARN | 绕开 slog 的纯文本进不了日志平台 |
| [`XRedis` 命令超时](../xredis/README.md#行为与实测) | 只认 `ReadTimeout` | 听调用方 ctx 的截止时间 | 不然 200ms 预算的请求会等满 `ReadTimeout` |
| [`XRedis` 建连](../xredis/README.md#行为与实测) | 每次建连内部重拨 5 次 | 只拨一次 | 否则主机宕机时一条命令 11.7s |
| [`XRedis` 新连接上的握手](../xredis/README.md#行为与实测) | `CLIENT SETINFO`、维护通知都开 | 都关 | 7.2 之前的服务端每条连接留一个报错的 Span |
| [`XRedis.Trace`](../xredis/README.md#行为与实测) | 整条命令连同参数写进 `db.statement` | 只有命令名 | 否则 `SET` 的值原样进链路后端 |
| [go-redis 的日志](../xredis/README.md#行为与实测) | 标准库 log 写 `os.Stderr` | 接到 slog，记成 WARN | 同 go-sql-driver |
| [`XCache.MaxCost`](../xcache/README.md#行为与实测) | 每条另计 56 字节内部开销 | 只算你给的 cost | 否则 `MaxCost: 2000` 实际只存得下 35 条 |
| [`XCache` 停止](../xcache/README.md#行为与实测) | `Close` | `Clear` | `Close` 与并发读写一起跑会 panic |
| [`XHttp` 的 resty 日志](../xhttp/README.md#行为与实测) | 写 `os.Stderr`，URL 带查询串 | 接到 slog，去掉查询串 | 令牌不落盘 |
| [`XHttp` 的 cookie](../xhttp/README.md#行为与实测) | `resty.New()` 自带 cookie jar | 没有 | 不相干的调用会共享别人种下的会话 cookie |
| [`XHttp` 出站 Span](../xhttp/README.md#行为与实测) | `url.full` 带查询串 | 去掉查询串，Span 名只用方法 | 令牌不进链路后端，Span 名基数有界 |
| [`XHttp` 重试条件](../xhttp/README.md#行为与实测) | 挂上条件后 resty 自己的判断作废 | 只重试传输层错误 | 否则 200 + 坏 JSON 也会重试 |
| [`XTrace` 透传与 baggage](../xtrace/README.md#行为与实测) | 入站的值照单全收 | 只收直连对端在 `XGin.TrustedProxies` 里的 | 否则公网客户端能伪造 `X-Tenant-Id` 带进内网 |
| [`XTrace` 采样](../xtrace/README.md#行为与实测) | `AlwaysSample` 无视上游 | `ParentBased`，有上游时听上游 | 否则上游 `sampled=00` 被改成 `-01` 往下传 |
| [`XMetric.Namespace`](../xmetric/README.md#行为与实测) | 不合规的名字导出时转义 | 读配置时失败 | 否则看板按你写的名字查不到 |

每一项的实测在对应模块的 README 里，见[各模块的实测](#各模块的实测)。

## 启动期建连探测

XGorm、XRedis 启动时各探一次，共用同一份实现（`internal/xclient.Probe`，基于 `xutil.Retry`）：

- **最多试 3 次**（第一次加两次重试），两次之间的等待逐次翻倍并带抖动：退避从 1s 起，
  实际等待在 `[0, 当前退避]` 之间取值，两次退避的上界是 1s、2s。
  所以一个实例最多等 `3 × 单次探测预算 + 3s`：XGorm 连 PostgreSQL 默认 `3 × 1.5s + 3s = 7.5s`，
  XRedis 默认 `3 × 1s + 3s = 6s`。
- **认证失败不重试**：PostgreSQL 的 SQLSTATE 第 28 类（密码错、用户不存在都是 28P01）；
  MySQL 的 1045（密码错、用户不存在）和 1044（没有这个库的权限——没有全局权限的账号连一个不存在的库
  拿到的也是 1044；实测 MySQL 8.0.46）；Redis 的 `WRONGPASS` / `NOAUTH`；ClickHouse 的 516 和 192 / 193 / 194。
  错误报 `authentication to <地址> failed`，不是 `cannot reach`。
- **证书被拒不重试**：`x509` 校验失败，或者对端发来 `bad_certificate` / `unknown_ca` /
  `certificate_required` 告警。再试还是同一张证书、同一个结论。
- 启动期间收到退出信号时当场放弃，不等卡着的那次探测撞上超时。

## 各模块的实测

| 模块 | 实测 |
|---|---|
| [xtls](../xtls/README.md#行为与实测) | 客户端 TLS：各驱动的报错、耗时，不开 TLS 块时驱动的默认 |
| [xgorm](../xgorm/README.md#行为与实测) | GORM 通用、MySQL、PostgreSQL |
| [xgorm/clickhouse](../xgorm/clickhouse/README.md#行为与实测) | ClickHouse：超时与取消、重发、认证、TLS |
| [xredis](../xredis/README.md#行为与实测) | go-redis：超时、建连重试、新连接上的握手、内存 |
| [xcache](../xcache/README.md#行为与实测) | ristretto：内部开销、停止、指标开销、TTL |
| [xhttp](../xhttp/README.md#行为与实测) | resty / otelhttp / 标准库 Transport 的默认 |
| [xgin](../xgin/README.md#行为与实测) | gin / net/http：代理、上传、超时、h2c、TLS、优雅退出 |
| [xmetric](../xmetric/README.md#行为与实测) | client_golang：桶、名字、常量标签 |
| [xtrace](../xtrace/README.md#行为与实测) | OTel SDK：采样、service.name、透传 |
| [xlog](../xlog/README.md#行为与实测) | `Perm`、轮转文件名、时区 |
