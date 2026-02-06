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
func (c *Cache) Set(key string, value any) bool {
	return c.raw.SetWithTTL(key, value, 1, c.defaultTTL)
}

// SetWithTTL 设置缓存值，指定 TTL，cost=1
func (c *Cache) SetWithTTL(key string, value any, ttl time.Duration) bool {
	return c.raw.SetWithTTL(key, value, 1, ttl)
}

// SetWithCost 设置缓存值，指定 cost，使用默认 TTL
func (c *Cache) SetWithCost(key string, value any, cost int64) bool {
	return c.raw.SetWithTTL(key, value, cost, c.defaultTTL)
}

// SetWithCostAndTTL 设置缓存值，指定 cost 和 TTL
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
// ristretto 内部使用环形缓冲区，Set 后值不一定立即可读，调用 Wait 可确保写入完成
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
