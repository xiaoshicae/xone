package xcache

import (
	"time"

	"github.com/dgraph-io/ristretto"
)

// Cache 本地缓存封装，基于 ristretto
type Cache struct {
	raw        *ristretto.Cache
	defaultTTL time.Duration
}

// Get 获取缓存值
func (c *Cache) Get(key string) (any, bool) {
	return c.raw.Get(key)
}

// Set 设置缓存值，使用默认 TTL，cost=1
//
// 返回值是「是否被接收」，不是「是否已缓存」，两件事在 ristretto 里并不等同：
//
//   - 返回 true 只代表写入请求进了缓冲区。ristretto 的准入策略仍可能判定这个
//     键不值得留下而丢弃它，此时没有任何返回值或日志会提到。
//   - 返回 true 之后立即 Get 也可能 miss。写入走环形缓冲异步生效，
//     要确定性地读到刚写的值（典型是测试里）需要先调 Wait。
//   - 返回 false 代表缓冲区已满、这次写入被直接丢弃，属于瞬时状态，可以重试。
//
// 所以它适合用来观测「缓存是不是在丢写入」，不适合用来判断某个键此刻在不在缓存里
// ——后者只有 Get 能回答。缓存语义上本就允许丢失，正常业务路径忽略返回值即可。
func (c *Cache) Set(key string, value any) bool {
	return c.raw.SetWithTTL(key, value, 1, c.defaultTTL)
}

// SetWithTTL 设置缓存值，指定 TTL，cost=1
//
// 返回值含义同 Set。ttl 为负数时 ristretto 直接丢弃该值。
func (c *Cache) SetWithTTL(key string, value any, ttl time.Duration) bool {
	return c.raw.SetWithTTL(key, value, 1, ttl)
}

// SetWithCost 设置缓存值，指定 cost，使用默认 TTL
//
// 返回值含义同 Set。
func (c *Cache) SetWithCost(key string, value any, cost int64) bool {
	return c.raw.SetWithTTL(key, value, cost, c.defaultTTL)
}

// SetWithCostAndTTL 设置缓存值，指定 cost 和 TTL
//
// 返回值含义同 Set。
func (c *Cache) SetWithCostAndTTL(key string, value any, cost int64, ttl time.Duration) bool {
	return c.raw.SetWithTTL(key, value, cost, ttl)
}

// Del 删除缓存值
func (c *Cache) Del(key string) {
	c.raw.Del(key)
}

// Clear 清空缓存
func (c *Cache) Clear() {
	c.raw.Clear()
}

// Wait 等待所有缓冲写入完成，主要用于测试场景
//
// ristretto 内部使用环形缓冲区，Set 返回后值不一定立即可读，调用 Wait 可确保写入完成。
// 会阻塞到缓冲区排空，不要放在请求路径上。
func (c *Cache) Wait() {
	c.raw.Wait()
}

// Close 关闭缓存，释放资源
func (c *Cache) Close() {
	c.raw.Close()
}

// Raw 获取底层 ristretto.Cache 实例，用于高级操作
func (c *Cache) Raw() *ristretto.Cache {
	return c.raw
}
