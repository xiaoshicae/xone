package xcache

import "time"

// TypedCache 类型安全的缓存封装
type TypedCache[V any] struct {
	cache *Cache
}

// Of 获取类型安全的缓存视图，无参数时使用全局缓存，有参数时按名称获取
func Of[V any](name ...string) *TypedCache[V] {
	var cache *Cache
	if len(name) > 0 {
		cache = C(name[0])
	} else {
		cache = global()
	}
	return &TypedCache[V]{cache: cache}
}

// Get 获取缓存值，自动转换为目标类型
func (c *TypedCache[V]) Get(key string) (V, bool) {
	var zero V
	if c.cache == nil {
		return zero, false
	}
	val, ok := c.cache.Get(key)
	if !ok {
		return zero, false
	}
	typed, ok := val.(V)
	if !ok {
		return zero, false
	}
	return typed, ok
}

// Set 设置缓存值，使用默认 TTL
func (c *TypedCache[V]) Set(key string, value V) bool {
	if c.cache == nil {
		return false
	}
	return c.cache.Set(key, value)
}

// SetWithTTL 设置缓存值，指定 TTL
func (c *TypedCache[V]) SetWithTTL(key string, value V, ttl time.Duration) bool {
	if c.cache == nil {
		return false
	}
	return c.cache.SetWithTTL(key, value, ttl)
}

// Del 删除缓存值
func (c *TypedCache[V]) Del(key string) {
	if c.cache == nil {
		return
	}
	c.cache.Del(key)
}

// Wait 等待所有缓冲写入完成
func (c *TypedCache[V]) Wait() {
	if c.cache == nil {
		return
	}
	c.cache.Wait()
}

// --- 包级泛型函数，操作全局缓存 ---

// Get 从全局缓存获取值，自动转换为目标类型
func Get[V any](key string) (V, bool) {
	var zero V
	cache := global()
	if cache == nil {
		return zero, false
	}
	val, ok := cache.Get(key)
	if !ok {
		return zero, false
	}
	typed, ok := val.(V)
	if !ok {
		return zero, false
	}
	return typed, ok
}

// Set 向全局缓存设置值，使用默认 TTL
func Set(key string, value any) bool {
	cache := global()
	if cache == nil {
		return false
	}
	return cache.Set(key, value)
}

// SetWithTTL 向全局缓存设置值，指定 TTL
func SetWithTTL(key string, value any, ttl time.Duration) bool {
	cache := global()
	if cache == nil {
		return false
	}
	return cache.SetWithTTL(key, value, ttl)
}

// Del 从全局缓存删除值
func Del(key string) {
	cache := global()
	if cache == nil {
		return
	}
	cache.Del(key)
}
