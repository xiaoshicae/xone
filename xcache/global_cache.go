package xcache

import "time"

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
