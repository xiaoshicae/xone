package xcache

import (
	"time"

	"github.com/xiaoshicae/xone/v2/xutil"
)

// --- 包级泛型函数，操作全局缓存 ---

// Get 从全局缓存获取值，自动转换为目标类型
//
// 类型不匹配时返回零值与 false，与 cache miss 的返回值相同，
// 因此额外打一条日志：否则「明明 Set 了却永远 miss」没有任何线索，
// 典型场景是存的是 *T 而取的是 T
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
		xutil.WarnIfEnableDebug("XOne xcache type mismatch, key=[%s], stored=[%T], want=[%T], treated as miss", key, val, zero)
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
