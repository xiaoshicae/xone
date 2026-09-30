# xredis

Redis 客户端：拿到的是原生 `*redis.Client`（go-redis v9），框架按配置建好、退出时关掉。

- 单实例、多实例都行，`xredis.C("name")` 按名字取
- 命令听 ctx 的截止时间（go-redis 默认不听，只认 `ReadTimeout`）
- 每条命令一个 Span，只带命令名、不带参数
- 可选的命令日志（`Log: true`）：命令名和第一个 key，不记值
- 连接池指标 `redis_pool_*`，按实例开关
- 启动时探一次，连不上直接启动失败

## 快速上手

```yaml
# conf/application.yml
XRedis:
  Addr: "redis:6379"
  Password: "${REDIS_PASSWORD}"
```

```go
import (
	"errors"

	"github.com/redis/go-redis/v9"

	"github.com/xiaoshicae/xone/xredis"
)

err := xredis.C().Set(ctx, "k", "v", time.Minute).Err()

v, err := xredis.C().Get(ctx, "k").Result()
if errors.Is(err, redis.Nil) {
	// key 不存在
}
```

go-redis 的方法本来就收 ctx，所以没有 `CWithCtx`。

## 配置

```yaml
XRedis:
  Addr: "127.0.0.1:6379"
  Username: ""                  # Redis 6+ ACL 用户名
  Password: "${REDIS_PASSWORD}"
  DB: 0
  DialTimeout: 500ms            # 拨号 + TLS 握手
  ReadTimeout: 500ms            # 调用方没给截止时间时，一次读最多等这么久
  WriteTimeout: 500ms
  PoolSize: 0                   # 0 = go-redis 默认的 10 × GOMAXPROCS
  MinIdleConns: 5               # 常驻的空闲连接；多实例时每个实例各这么多
  MaxIdleConns: 0               # 0 = 不限
  MaxActiveConns: 0             # 0 = 不限
  PoolTimeout: 1s               # 池子满了等一条连接最多多久
  ConnMaxIdleTime: 5m
  ConnMaxLifetime: 5m
  MaxRetries: 0                 # 0 = go-redis 默认的 3 次；-1 关掉重试
  MinRetryBackoff: 0s           # 0 = go-redis 默认的 10ms；-1ns 关掉
  MaxRetryBackoff: 0s           # 0 = go-redis 默认的 1s；-1ns 关掉
  Trace: true                   # 每条命令一个 Span
  Metric: true                  # 连接池指标
  Log: false                    # 每条命令一行日志（pipeline 整个一行）：命令名 + 第一个 key，不记值
  SlowThreshold: 100ms          # 超过它记 WARN；需 Log 开启，0 = 不记
  TLS:                          # 规则见 xtls；ServerName 空着 = Addr 的主机部分
    Enable: false
    CAFile: ""
    CertFile: ""
    KeyFile: ""
    ServerName: ""
```

**多实例**写在 `Clients` 下，每个实例的字段同上，没写的用上面的默认值：

```yaml
XRedis:
  Clients:
    default: {Addr: "redis:6379"}
    session: {Addr: "redis-session:6379", DB: 1}
```

`xredis.C()` 取 `default`，`xredis.C("session")` 取另一个。两种写法不能混用。

## API

| 函数 | 说明 |
|---|---|
| `C(name ...string) *redis.Client` | 取实例，不带参数取 `default`。取不到直接 panic，消息里说清是调早了、没配还是名字写错 |
| `Has(name ...string) bool` | 实例配了没有，可选依赖先判断 |
| `Names() []string` | 配了哪些实例 |
| `New(ctx, cfg) (*redis.Client, io.Closer, error)` | 纯构造器：不碰全局、不读配置文件，离开框架也能用 |

## 注意事项

- **要停得下来，就给 ctx 带截止时间。** 命令听截止时间、不听取消：ctx 被取消叫不醒一条已经阻塞在读上的命令，
  它照样等到 `ReadTimeout`。见[「行为与实测」](#行为与实测)。
- **一条命令最多要多久**（调用方没给截止时间时）：`(MaxRetries+1) × (DialTimeout+ReadTimeout) + MaxRetries × MaxRetryBackoff`，默认 7s。
- **负数一律不收**，只有 `MaxRetries: -1`、`MinRetryBackoff: -1ns`、`MaxRetryBackoff: -1ns` 表示「关掉」。
- **内存**：每条连接约 66KiB 堆，池子涨满时是 `PoolSize × 66KiB`。
- **启动探测**：每个实例探一次，最多试 3 次；`WRONGPASS` / `NOAUTH` 不重试。见
  [behavior.md「启动期建连探测」](../docs/behavior.md#启动期建连探测)。

## 可观测

### 日志

| 消息 | 级别 | 字段 |
|---|---|---|
| `xredis connected` | INFO | `name`、`addr`、`db`、`tls`、`min_idle_conns` |
| `xredis ready` | INFO | `instances` |
| `redis command` / `slow redis command` / `redis command failed` | INFO / WARN / WARN | `name`（实例名）、`cmd`（命令名，小写；不像命令名的记 `<invalid>`）、`key`（第一个 key，认不准的命令不带）、`elapsed_ms`（含等连接池、新连接的拨号和 `HELLO`）；key 超过 256 字节时截断并带 `key_truncated`；key 不存在时 `nil: true`（仍是 INFO）；慢命令带 `threshold_ms`；失败时 `error`、`error_code`（需 `XRedis.Log: true`） |
| `redis pipeline` / `slow redis pipeline` / `redis pipeline failed` | INFO / WARN / WARN | `name`、`count`（命令数，`TxPipelined` 的含 `MULTI` 和 `EXEC`）、`cmds`（前 10 个命令名）、`elapsed_ms`；WATCH 冲突时 `tx_failed: true`（仍是 INFO）；失败时是第一个不是 nil 的错误的 `error`、`error_code`（需 `XRedis.Log: true`） |

命令日志**只记命令名和第一个 key**：值、其余参数一律不记；`AUTH`、`HELLO`、`MIGRATE`、`CONFIG` 这类参数里可能有凭证的命令只记命令名。
**key 是整条记的**（超过 256 字节才截断）：开着 `Log` 时别把令牌、手机号、邮箱拼进 key，它们会跟着 key 进日志。
服务端报的错，`error` 只写 `redis server error <错误码> (message omitted, …)`，错误码另见 `error_code`，原文不记（原文会把参数带出来，见[「行为与实测」](#行为与实测)）；
客户端一侧的错误——超时、连不上、连接断开、`context canceled`、`redis: client is closed`、连接池超时——照原文记。
失败优先：又慢又失败的记 `… failed`，不另记慢。返回给调用方的错误不变。

日志的全局约定（`trace_id` 注入、`xlog.AddKV`、框架的启停日志）见 [`docs/observability.md`](../docs/observability.md#日志)。

### 指标

| 指标 | 类型 | 标签 | 来源 |
|---|---|---|---|
| `redis_pool_connections` / `redis_pool_connections_idle` | gauge | `name` | xredis，`XRedis.Metric`，按实例 |
| `redis_pool_connections_stale_total` / `redis_pool_hits_total` / `redis_pool_misses_total` / `redis_pool_timeouts_total` | counter | `name` | xredis |

指标名的前缀、常量标签和几条通用规则见 [`docs/observability.md`「指标」](../docs/observability.md#指标)。

### 链路

| 来源 | Span 名 | 关键属性 |
|---|---|---|
| xredis | redisotel 按命令起名 | 只有命令名，没有 `db.statement` 里的参数 |

链路的全貌、传播与信任边界见 [`docs/observability.md`「链路」](../docs/observability.md#链路)。

## 行为与实测

实测环境和跨模块的总表见 [`docs/behavior.md`](../docs/behavior.md)。

go-redis v9.22.0、redisotel、Redis 7.0.15。

**命令听 ctx 的截止时间，不听取消。** go-redis 默认连截止时间都不听，只认 `ReadTimeout`：实测 200ms 预算的请求
在慢 Redis 上等满 5s。这里打开 `ContextTimeoutEnabled`，截止时间设成 socket 的 deadline。但 ctx 被**取消**叫不醒一个
已经阻塞在读上的命令（`ReadTimeout: 1s`、100ms 时取消，命令 1.0s 返回），xredis 在里面补不上：结果写在调用方拿着的
`*Cmd` 上，提前返回就是和还在读的协程抢着写。落到停止流程上：XGin 到点断连时取消了请求的 ctx，卡在 Redis 读上的
handler 要等 `ReadTimeout`，或者等 xredis 的停止钩子关掉连接池（实测 `ReadTimeout: 10s`、`WithStopTimeout(3s)`：
1.6s 断连，handler 到 2.0s 连接池关掉时才返回，`Stop` 报 `1 handler(s) still running`）。要停得下来就给 ctx 带截止时间。

**一条命令最多要多久**（调用方没给截止时间时）：

```
(MaxRetries+1) × (DialTimeout+ReadTimeout) + MaxRetries × MaxRetryBackoff
```

默认 4 × 1s + 3 × 1s = 7s，`MaxRetries: -1` 时是 1s（`MaxRetries` 默认 3、`MinRetryBackoff` 10ms、`MaxRetryBackoff` 1s）。
这条式子成立是因为一次建连只拨一次号：go-redis 默认在每次建连里还藏着一层重试（`DialerRetries` 5 次、间隔 100ms），
一次「建连」就是 5 × 500ms + 4 × 100ms = 2.9s，实测主机宕机（SYN 没有回音）时一条命令用了 11.7s。
xredis 把 `DialerRetries` 固定成 1，同样的场景默认配置 2.1s、`MaxRetries: -1` 时 0.5s。
连接池累计 `PoolSize` 次建连失败之后 go-redis 不再拨号、直接报上一次的错。

**每条新连接上发什么**（挂着 redisotel 数 Span）：

| | go-redis 默认 | 这里 |
|---|---|---|
| `HELLO` | 每条新连接一次，协商 RESP3；服务端不认就退回 RESP2。配成 2 也照样发 `HELLO 2` | 不改 |
| `CLIENT SETINFO` | 每条新连接多一个 pipeline，只为让 `CLIENT LIST` 显示库名。Redis 7.2 之前没有这个子命令：7.0.15 上**每条连接**回 `unknown subcommand 'setinfo'`，错误被吞掉，但每条连接留下一个报错的 `redis.pipeline` Span——`ConnMaxLifetime` 每 5 分钟换一轮连接，就每 5 分钟一批 | 关掉（`DisableIdentity: true`） |
| `CLIENT MAINT_NOTIFICATIONS` | auto：每条新连接先试一次，服务端认就开启「维护通知」，维护期间把读写超时临时放宽（源码默认 10s，手头没有认这条命令的服务端，未实测）。7.0.15 回 `unknown subcommand`，并发建起来的头几条连接各留一个报错的 Span | 关掉：放宽到 10s 和 `ReadTimeout` 的承诺对不上 |

**每条连接的内存**：32KiB 读缓冲 + 32KiB 写缓冲，实测（200 条空闲连接）每条约 66KiB 堆。连接池涨满时是
`PoolSize × 66KiB`：默认 `PoolSize` 是 10 × GOMAXPROCS，4 核 40 条约 2.6MB，64 核 640 条约 41MB；
平时只有 `MinIdleConns`（默认 5）条。缓冲区大小不开放配置。`MaxConcurrentDials` 默认等于 `PoolSize`，不另设。

**TLS 握手受 `DialTimeout` 管**（go-redis 用 `tls.DialWithDialer`，拨号和握手共用一个超时）。

**命令日志（`Log: true`）记什么**。实测 go-redis v9.22.0、Redis 7.0.15，调用方的 ctx 里没有 Span（一条命令一个根 Span）：

| 调用 | 日志 |
|---|---|
| `Set(ctx, "user:42", <值>, time.Minute)` | `redis command`，`cmd: set`、`key: user:42` |
| `Get(ctx, "user:42")` 命中 | `redis command`，`cmd: get`、`key: user:42` |
| `Get(ctx, "user:missing")` 不存在 | `redis command`（INFO），`cmd: get`、`key: user:missing`、`nil: true` |
| `HSet(ctx, "h:1", <字段>, <值>)` | `cmd: hset`、`key: h:1` |
| `MGet(ctx, "a:1", "a:2")` | `cmd: mget`、`key: a:1`（只记第一个） |
| `Eval(ctx, script, []string{"lua:key"}, <ARGV>)` | `cmd: eval`、`key: lua:key`；`keys` 为空时不带 key（第 3 个参数已经是 ARGV） |
| `XRead(... Streams: {"stream:1", "0"})` | `cmd: xread`、`key: stream:1` |
| `Do(ctx, "auth", <密码>)` | `cmd: auth`，除命令名什么都不记 |
| `Incr` 一个不是数字的值 | `redis command failed`（WARN），`error_code: ERR` |
| pipeline：`Set`、`Get`（不存在）、`Incr` | 一行 `redis pipeline`，`count: 3`、`cmds: [set get incr]`；`TxPipelined` 的 `count` 和 `cmds` 里都算上 `multi` / `exec`（一条 `Set` 是 `count: 3`、`cmds: [multi set exec]`） |
| `Do(ctx, "SET k1 <值>")`（整条命令塞进一个参数） | `redis command failed`，`cmd: <invalid>`：go-redis 的命令名就是第 1 个参数原样转小写，不校验不截断，不修正的话记出来是 `cmd: "set k1 <值>"`。只认 `^[a-z][a-z0-9._\|-]{0,63}$`，模块命令（`json.set`）照记；pipeline 的 `cmds` 同样 |
| `Watch` 里别人先改了 key，`TxPipelined` 返回 `redis.TxFailedErr` | 一行 INFO 的 `redis pipeline`，`cmds: [multi set exec]`、`tx_failed: true`，不带 `error`（返回给调用方的照旧是 `TxFailedErr`） |

**key 在哪由这里认，不用 go-redis 的**。go-redis 自己知道（`Cmder` 未导出的 `firstKeyPos`），兜底却是「不在免 key 名单里就当
第 1 个参数」：`MIGRATE` 的第 1 个参数是 host，`INFO`、`KEYS` 也会被当成有 key。这里反过来，只认一张「第 1 个参数一定是 key」的
命令名单（string、hash、list、set、zset、stream、geo、bitmap 的常用命令），外加 `EVAL` / `EVALSHA` / `FCALL`（`numkeys` 大于 0 时
取第 3 个参数；go-redis 的 `Eval` 传的 `numkeys` 是 int，`Do` 拼的是字符串，两种都认）、`XREAD` / `XREADGROUP`（`STREAMS` 后面那个）；
名单外的一律不记 key——猜错的代价是把一个值写进日志，漏记一个 key 只是少一个字段。key 只取字符串或 `[]byte` 类型的参数，别的类型不猜。

**服务端的错误原文会带出参数**，所以只记错误码（Redis 7.0.15 实测）：

| 情形 | 原文 | 日志里的 `error_code` |
|---|---|---|
| 未知命令 | `ERR unknown command 'foo', with args beginning with: 'secretarg1' 'secretarg2'` | `ERR` |
| `EVAL` 里 `redis.error_reply(ARGV[1])` | `ERR mysecretvalue`（7.0 自动补了 `ERR`） | `ERR` |
| `EVAL` 里 `return {err=ARGV[1]}` | `plainsecret`：错误码的位置上就是值 | 不带 |
| `WRONGTYPE`、`ERR value is not an integer or out of range` | 不含参数 | `WRONGTYPE` / `ERR` |

最后一种说明「取第一个词当错误码」也不安全：错误码只认 Redis 自己用的那组（`ERR`、`WRONGTYPE`、`NOSCRIPT`、`BUSY`、`NOAUTH`、
`WRONGPASS`、`NOPERM`、`OOM`、`READONLY`、`EXECABORT`、`LOADING`、`MOVED`、`ASK`、`CROSSSLOT`、`CLUSTERDOWN` 等），其余的只说是服务端的错。
客户端一侧的错误只有超时、连不上、连接断开、ctx 取消、连接池超时 / 已关闭这几类照原文记；别的也不记原文——go-redis 解析回复失败时
把回复内容写进错误（`reader.go` 的 `can't parse %q`）。

**`elapsed_ms` 从钩子进门算起**，不只是网络往返：实测 `PoolSize: 1`、唯一的连接被一条 1s 的 `BLPOP` 占着时，
紧跟着的 `GET` 记 `elapsed_ms: 1028`（等连接池）；要新建连接的命令含拨号和 `HELLO`（假服务端让 `HELLO` 慢 80ms，那条 `GET` 记 81.7）。

**key 超过 256 字节时截断，不切开 UTF-8 字符**：往回找字符起点最多退 3 个字节。key 是任意字节串，一串 `0x80` 这样的没有字符起点，
就照 256 字节截（JSON 里是一串 `\ufffd`），不会截成空串。

**`redis.Nil` 不是失败**：key 不存在是读穿缓存天天走的分支，记 INFO、带 `nil: true`。
**WATCH 冲突也不是失败**：`redis.TxFailedErr` 是 go-redis 在客户端造的 `proto.RedisError`，和服务端的错误同一个类型，
不单独认的话会记成 `redis pipeline failed`、`error: redis server error (message omitted, …)`。它是乐观锁的正常结果，记 INFO、带 `tx_failed: true`。pipeline 的 `Exec` 在有一条拿到 nil 时
整个返回 `redis.Nil`，这里跳过它找第一个真错误：`GET`（不存在）后面跟一条报错的 `INCR`，记的是 `redis pipeline failed`、`error_code: ERR`。

**日志和链路**：日志钩子挂在 redisotel 的钩子之后，go-redis 先挂的在外层，所以它在 redis Span 里面——日志的 `span_id` 是
这条命令的 Span，`trace_id` 与调用方的相同（实测：调用方 Span `48ee662c…`，redis Span `c97ed554…` 的父是它，日志带的是
`c97ed554…`）。调用方没有 Span 时一条命令一个根 Span，日志带的是它的 `trace_id`；`Trace: false` 时日志的 ctx 就是调用方的。

**每条新连接一行 `hello`**：go-redis 在新连接上发的 `HELLO`（和 `DB` 不为 0 时的 `SELECT`）也走钩子，实测每条新连接一行
`redis command`、`cmd: hello`（`DB: 1` 时再一行 `redis pipeline`、`cmds: [select]`）；配了密码时 `HELLO 3 AUTH <用户> <密码>` 也只记
`cmd: hello`，实测输出里搜不到密码。连接池按 `ConnMaxLifetime`（默认 5m）换连接，就每 5 分钟一批。Redis 6 之前没有 `HELLO`，
每条新连接会是一行 `redis command failed`（go-redis 吞掉这个错、退回 RESP2）。启动时的建连探测不记：钩子在探测成功之后才挂。

**`Trace`**：redisotel 默认把整条命令连同参数写进 `db.statement`（实测 `SET` 的值原样出现），这里关掉了
（`WithDBStatement(false)`）。钩子不是零成本：没装链路时实测每条命令约 +3µs、+8 次分配。

**go-redis 的日志**默认用标准库 log 往 stderr 写纯文本（`redis: 2026/09/24 10:00:00 pool.go:762: ...`）。
这里在 xredis 的 `init` 里接到 slog：一条 `xredis go-redis log`，级别 WARN，原文在 `detail`。
用命令 ctx 记的那些带 trace_id；最常见的 `failed to dial` 不带——go-redis 在它自己的协程里用
`context.Background()` 拨号，实测 Redis 拒绝连接时 45 条一条都没有 trace_id。自定义的 logger 在 `main` 里、
或在一个 import 了 xredis 的包里设置；放在没有 import xredis 的包的 `init` 里，可能被 xredis 盖掉。
