# xredis

Redis 客户端：拿到的是原生 `*redis.Client`（go-redis v9），框架按配置建好、退出时关掉。

- 单实例、多实例都行，`xredis.C("name")` 按名字取
- 命令听 ctx 的截止时间（go-redis 默认不听，只认 `ReadTimeout`）
- 每条命令一个 Span，只带命令名、不带参数
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

**`Trace`**：redisotel 默认把整条命令连同参数写进 `db.statement`（实测 `SET` 的值原样出现），这里关掉了
（`WithDBStatement(false)`）。钩子不是零成本：没装链路时实测每条命令约 +3µs、+8 次分配。

**go-redis 的日志**默认用标准库 log 往 stderr 写纯文本（`redis: 2026/09/24 10:00:00 pool.go:762: ...`）。
这里在 xredis 的 `init` 里接到 slog：一条 `xredis go-redis log`，级别 WARN，原文在 `detail`。
用命令 ctx 记的那些带 trace_id；最常见的 `failed to dial` 不带——go-redis 在它自己的协程里用
`context.Background()` 拨号，实测 Redis 拒绝连接时 45 条一条都没有 trace_id。自定义的 logger 在 `main` 里、
或在一个 import 了 xredis 的包里设置；放在没有 import xredis 的包的 `init` 里，可能被 xredis 盖掉。
