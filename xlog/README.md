# xlog

日志：装好之后 `slog.Default()` 就是按 `XLog` 配好的原生 `*slog.Logger`，业务代码直接用标准库 `log/slog`。

- 核心模块，零依赖，跟着框架一起来：import 了 `xone` 就装好了，不用另外 import；不写配置就是 info 级别的 JSON 打到标准输出
- 有链路时每条日志自动带 `trace_id` / `span_id`
- `xlog.AddKV` 给整个请求加字段（访问日志也带上），`xlog.CtxWithKV` 只给一段调用加
- 文件输出按时间轮转、按保留时长清理，只删自己命名的文件
- 时间戳可按指定时区渲染

## 快速上手

```yaml
# conf/application.yml（可选）
XLog:
  Level: info
  File: {Enable: true, Path: /var/log/app}
```

```go
import (
	"context"
	"log/slog"

	"github.com/xiaoshicae/xone/xlog"
)

func Create(ctx context.Context, userID string, orders []Order) {
	xlog.AddKV(ctx, "user_id", userID) // 同一请求之后的每条日志（包括访问日志）都带着它

	for _, o := range orders {
		ctx := xlog.CtxWithKV(ctx, map[string]any{"order_id": o.ID}) // 只有这一轮的日志带着它
		slog.InfoContext(ctx, "order created", "amount", o.Amount)
	}
}
```

一律用带 ctx 的 `slog.InfoContext`，才带得上 `trace_id` 和上面这些字段。
日志的 message 和字段名建议用英文：它们会变成日志平台里的检索词和 JSON key。

## 配置

```yaml
XLog:
  Level: info              # debug / info / warn / error，默认 info
  Format: json             # json / text，默认 json
  AddSource: false         # 记代码位置，有开销，默认关
  Timezone: ""             # 时间戳按哪个 IANA 时区渲染，如 Europe/Berlin；空 = 进程本地时区
  Console: true            # 打到标准输出，默认开
  File:
    Enable: false          # 默认关
    Path: /var/log/app     # 目录
    Name: app.log          # 默认 app.log；实际文件带时间后缀，另有同名符号链接指向当前文件
    RotateTime: 24h        # 轮转周期，按本地时区对齐，默认一天，至少 1m
    MaxAge: 168h           # 历史保留时长，默认 7 天；0 = 不清理，负数启动失败
    Perm: "0644"           # 按八进制解析的字符串，默认 "0644"，644 / 0o644 也认
```

- 文件名后缀随 `RotateTime` 的粒度：≥ 24h 是 `app.log.20260918`，≥ 1h 是 `app.log.2026091815`，更短是 `app.log.202609181504`。
- 清理启动时一次、之后每次轮转一次，只删 `app.log.<时间后缀>`；`app.log.bak`、`app.log.1.gz` 不碰。

## API

| 函数 | 说明 |
|---|---|
| `AddKV(ctx, k, v)` / `AddKVs(ctx, map[string]any)` | 往当前作用域原地写字段，同一请求之后的每条日志都带上 |
| `CtxWithKV(ctx, map[string]any) context.Context` | 派生一个带这些字段的新 ctx，只有用它写的日志带着；不影响父 ctx 和兄弟 |
| `CtxWithScope(ctx) context.Context` | 开一个字段作用域，非 Web 入口（消费一条消息、跑一次任务）用；已有作用域时原样返回 |
| `DroppedKVCount() int64` | 因为没有作用域被丢掉的 `AddKV` 字段数，不为零多半是漏了 `CtxWithScope` |
| `Location() *time.Location` | 生效中的时区：`t.In(xlog.Location()).Format(time.RFC3339)`；没配时是 `time.Local` |
| `TraceIDs(ctx) (traceID, spanID string)` | 当前 ctx 的链路标识，就是日志里写的那两个值；没有时是两个空串 |
| `New(cfg) (*slog.Logger, io.Closer, error)` | 纯构造器：不碰全局、不读配置文件，离开框架也能用 |

`SetTraceExtractor` / `AddObserver` 是给 xtrace、xmetric 这类集成注入能力用的，业务代码用不到。

## 注意事项

- **用 `slog.InfoContext(ctx, …)`**：不带 ctx 的 `slog.Info` 拿不到 `trace_id`，也带不上 `AddKV` / `CtxWithKV` 的字段。
- **`AddKV` 要有作用域**：xgin 在每个请求开头开好；自己的非 Web 入口用 `xlog.CtxWithScope(ctx)` 开，否则字段被丢掉（计进 `DroppedKVCount`）。
  见 [observability.md「日志」](../docs/observability.md#日志)。
- **`AddKV` 和 `CtxWithKV` 的分工在影响范围**：`AddKV` 原地写，整个请求都带上；`CtxWithKV` 只影响它返回的 ctx，
  批量处理的每一条、起的每个 goroutine 各派生一个，互相不串，也不回流到访问日志。派生之后父 ctx 再 `AddKV` 的字段它看不到。
- **`Timezone` 配了却加载不到直接启动失败**；scratch / distroless 镜像要 `import _ "time/tzdata"`。见[「行为与实测」](#行为与实测)。
- **`Name` 的位置上已经有一个普通文件**（不是符号链接）时启动失败，不会把旧日志吞掉。

## 行为与实测

实测环境和跨模块的总表见 [`docs/behavior.md`](../docs/behavior.md)。

**`Perm` 为什么是字符串**：实测 yaml.v3 把 `0644` 解析成 420（对的），漏掉前导 0 写成 `644` 却是十进制 644 = 0o1204，
不报错。所以按八进制解析字符串，`0644`、`644`、`0o644` 都认。

**`RotateTime` 的文件名后缀**按周期取粒度：一天及以上是 `app.log.20260918`，一小时及以上是 `app.log.2026091815`，
更短是 `app.log.202609181504`。最细到分钟，所以短于 1m 直接启动失败：0s 实际每分钟一个文件，
30s 两个周期落在同一个文件名上。轮转按本地时区对齐。

**`Timezone` 配了却加载不到直接启动失败**，不会悄悄退回本地时区。scratch / distroless 镜像里没有
`/usr/share/zoneinfo`，要在自己的 `main` 包加一行 `import _ "time/tzdata"`（约 400KB）；框架不替你编，
没用到这项的人不该背这 400KB。
