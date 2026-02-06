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
import "github.com/xiaoshicae/xone/xcache"

// 直接使用包级泛型函数，无需手动类型断言
xcache.Set("user:123", user)
user, ok := xcache.Get[*User]("user:123")

// 指定 TTL
xcache.SetWithTTL("session:abc", token, time.Hour)

// 删除
xcache.Del("user:123")
```

### TypedCache 类型安全封装

```go
// 创建类型安全的缓存视图
userCache := xcache.Of[*User]()
userCache.Set("user:123", user)
user, ok := userCache.Get("user:123") // 直接返回 *User，无需断言

// 多实例 + 类型安全
productCache := xcache.Of[*Product]("product-cache")
productCache.Set("product:456", product)
product, ok := productCache.Get("product:456")
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

## 注意事项

- ristretto 内部使用环形缓冲区，`Set` 后值不一定立即可通过 `Get` 读取。在测试场景下可调用 `Wait()` 确保写入完成，生产环境下通常无需关注。
- `Set` 方法默认 cost=1，此时 `MaxCost` 等价于最大缓存条目数。如需按实际大小淘汰，请使用 `SetWithCost` 或 `SetWithCostAndTTL`。
- `NumCounters` 建议设置为期望缓存条目数量的 10 倍，以获得最佳的频率追踪效果。
- 泛型 `Get[V]` 在类型不匹配时返回零值和 `false`，不会 panic。
