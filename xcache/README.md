# XCache - 本地缓存模块

基于 [ristretto](https://github.com/dgraph-io/ristretto) 的高性能本地缓存模块，支持 TTL、LFU 淘汰策略、并发安全。

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

## 使用

### 基本用法

```go
import "github.com/xiaoshicae/xone/xcache"

// 获取缓存实例
cache := xcache.C()

// 设置缓存（使用默认 TTL）
cache.Set("user:123", user)

// 注意：ristretto 使用内部缓冲区，Set 后值可能不会立即可读
// 测试场景下可调用 Wait() 确保写入完成
cache.Wait()

// 获取缓存
if val, ok := cache.Get("user:123"); ok {
    user := val.(*User)
    // ...
}

// 删除缓存
cache.Del("user:123")

// 清空缓存
cache.Clear()
```

### 指定 TTL

```go
cache := xcache.C()

// 设置 1 小时过期
cache.SetWithTTL("session:abc", sessionData, time.Hour)

// TTL 为 0 表示永不过期
cache.SetWithTTL("config:app", appConfig, 0)
```

### 指定 Cost

```go
cache := xcache.C()

// cost 用于控制缓存淘汰，可以按条目大小设置
// 例如按字节数设置 cost
data := []byte("large payload")
cache.SetWithCostAndTTL("data:key", data, int64(len(data)), time.Hour)
```

### 多实例模式

```go
// 获取指定名称的缓存
userCache := xcache.C("user-cache")
userCache.Set("user:123", user)

productCache := xcache.C("product-cache")
productCache.Set("product:456", product)

// 不传名称获取默认缓存（第一个配置的实例）
defaultCache := xcache.C()
```

### 获取底层 ristretto 实例

```go
raw := xcache.C().Raw()
// 使用 ristretto 原生 API
raw.SetWithTTL("key", "value", 1, time.Hour)
```

## 注意事项

- ristretto 内部使用环形缓冲区，`Set` 后值不一定立即可通过 `Get` 读取。在测试场景下可调用 `Wait()` 确保写入完成，生产环境下通常无需关注。
- `Set` 方法默认 cost=1，此时 `MaxCost` 等价于最大缓存条目数。如需按实际大小淘汰，请使用 `SetWithCost` 或 `SetWithCostAndTTL`。
- `NumCounters` 建议设置为期望缓存条目数量的 10 倍，以获得最佳的频率追踪效果。
