# XCache - 本地缓存模块

基于 [ristretto](https://github.com/dgraph-io/ristretto) 的高性能本地缓存模块，支持 TTL、LFU 淘汰策略、并发安全、泛型类型安全访问。

## 配置

```yaml
# 单实例模式
XCache:
  NumCounters: 1000000   # 跟踪频率的键数量，建议为期望条目数的 10 倍（默认 1000000）
  MaxCost: 100000        # 最大缓存条目数，cost=1 时等价于最大条目数（默认 100000）
  BufferItems: 64        # Get 操作的内部缓冲区大小（默认 64）
  DefaultTTL: "5m"       # 默认过期时间（默认 "5m"）

# 多实例模式
XCache:
  - Name: "user-cache"
    MaxCost: 50000
    DefaultTTL: "10m"
  - Name: "product-cache"
    MaxCost: 200000
    DefaultTTL: "1h"
```

> 无需配置也可直接使用，模块会自动懒初始化一个默认缓存实例。

## 使用

### 泛型 API（推荐）

```go
import "github.com/xiaoshicae/xone/v3/xcache"

// 直接使用包级泛型函数，无需手动类型断言
xcache.Set("user:123", user)
user, ok := xcache.Get[*User]("user:123")

// 指定 TTL
xcache.SetWithTTL("session:abc", token, time.Hour)

// 删除
xcache.Del("user:123")
```

### 原始 Cache API

```go
// 获取缓存实例
cache := xcache.C()

// 设置缓存（使用默认 TTL）
cache.Set("key", value)

// 获取缓存（返回 any，需要自行断言）
if val, ok := cache.Get("key"); ok {
    user := val.(*User)
}

// 指定 TTL
cache.SetWithTTL("key", value, time.Hour)

// 指定 cost（用于按大小淘汰）
cache.SetWithCostAndTTL("data:key", data, int64(len(data)), time.Hour)

// 删除 / 清空
cache.Del("key")
cache.Clear()

// 获取底层 ristretto 实例
raw := cache.Raw()
```

### 多实例模式

```go
// 获取指定名称的缓存
userCache := xcache.C("user-cache")
productCache := xcache.C("product-cache")

// 不传名称获取默认缓存（第一个配置的实例）
defaultCache := xcache.C()
```

## 包级函数

操作全局缓存（未配置 `XCache` 时懒初始化一个默认实例）：

| 函数 | 说明 |
|------|------|
| `Get[V](key) (V, bool)` | 取值并转换为目标类型 |
| `Set(key, value) bool` | 设置，使用默认 TTL，cost=1 |
| `SetWithTTL(key, value, ttl) bool` | 指定 TTL |
| `SetWithCost(key, value, cost) bool` | 指定 cost |
| `SetWithCostAndTTL(key, value, cost, ttl) bool` | 同时指定 cost 与 TTL |
| `Del(key)` | 删除 |
| `Clear()` | 清空 |
| `Wait()` | 等待缓冲写入完成（ristretto 是环形缓冲，Set 后不一定立即可读，主要用于测试） |
| `Has(name...) bool` | 指定名称的缓存实例是否已配置，供可选依赖判断，避免 `C()` panic |

需要操作具名实例时用 `C("name")` 取到 `*Cache`，方法名与上表一致。

`C()` 取不到实例就直接 panic，信息里带上你要的名字和实际配了哪些：

```
XOne xcache: no cache found for name=[typo], configured=[hot cold]
```

返回 `nil` 并不会让程序走得更远（`*Cache` 的方法在 `nil` 上都是空指针解引用），
只是把同一个 panic 推迟到第一次使用的时候，那里的栈里看不出根因是配置没配。
可选依赖用 `Has("name")` 先判断。

注意这和上面的包级函数不同：包级 `Get`/`Set` 操作的是全局缓存，未配置 `XCache` 时会懒初始化
一个默认实例，因此不会 panic；`C()` 取的是配置里写明的具名实例，要不到就是配置问题。

### Set 的返回值是「是否被接收」，不是「是否已缓存」

这两件事在 ristretto 里并不等同，返回值只回答前者：

| 返回 | 含义 |
|------|------|
| `true` | 写入请求进了缓冲区。准入策略仍可能判定这个键不值得留下而丢弃它，此时没有任何返回值或日志会提到 |
| `true` 后立即 `Get` | 仍可能 miss：写入走环形缓冲异步生效，要确定性地读到刚写的值（典型是测试里）需先调 `Wait()` |
| `false` | 缓冲区已满、本次写入被直接丢弃，属于瞬时状态，可以重试 |
| `false`（仅包级函数） | 或者缓存不可用——创建失败、模块已关闭，这种情况会打日志，与上一行区分开 |

所以它适合用来观测「缓存是不是在丢写入」，不适合用来判断某个键此刻在不在缓存里——后者只有 `Get` 能回答。缓存语义上本就允许丢失，正常业务路径忽略返回值即可。

## 注意事项

- ristretto 内部使用环形缓冲区，`Set` 后值不一定立即可通过 `Get` 读取。测试场景下调用 `Wait()` 确保写入完成，生产环境通常无需关注。
- `Set` 默认 cost=1，此时 `MaxCost` 等价于最大缓存条目数。需要按实际大小淘汰用 `SetWithCost` / `SetWithCostAndTTL`。
- `NumCounters` 建议设为期望缓存条目数的 10 倍，以获得最佳的频率追踪效果。

**类型不匹配表现为永久 miss。** `Get[V]` 在类型不匹配时返回零值与 `false`，
与 cache miss 的返回值完全相同 —— 存的是 `*User` 却用 `Get[User]` 取，
表现就是「明明 Set 了却永远读不到」。这种情况会额外打一条 warn 日志，排查时先看它。
不会 panic。

**模块关闭后包级函数安全降级。** BeforeStop 之后不再懒初始化新实例，包级的
`Get` / `Set` / `Del` 返回零值。否则新建的缓存再也不会有人来关，等于永久泄漏 ——
用户自己的 BeforeStop hook 在 xcache 之后执行（同为默认 Order，按 LIFO 反序），
里面读一次缓存就会触发。
