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
//
// 返回值含义见 Cache.Set：true 只代表被接收，既不保证最终留下，也不保证
// 下一次 Get 能读到。全局缓存额外多一种 false：缓存不可用（创建失败，
// 或模块已关闭），这种情况 global 会打日志，与「缓冲区满」这种瞬时丢弃区分开。
func Set(key string, value any) bool {
	cache := global()
	if cache == nil {
		return false
	}
	return cache.Set(key, value)
}

// SetWithTTL 向全局缓存设置值，指定 TTL
//
// 返回值含义同 Set。
func SetWithTTL(key string, value any, ttl time.Duration) bool {
	cache := global()
	if cache == nil {
		return false
	}
	return cache.SetWithTTL(key, value, ttl)
}

// SetWithCost 向全局缓存设置值，指定 cost，使用默认 TTL
//
// 返回值含义同 Set。
func SetWithCost(key string, value any, cost int64) bool {
	cache := global()
	if cache == nil {
		return false
	}
	return cache.SetWithCost(key, value, cost)
}

// SetWithCostAndTTL 向全局缓存设置值，指定 cost 和 TTL
//
// 返回值含义同 Set。
func SetWithCostAndTTL(key string, value any, cost int64, ttl time.Duration) bool {
	cache := global()
	if cache == nil {
		return false
	}
	return cache.SetWithCostAndTTL(key, value, cost, ttl)
}

// Del 从全局缓存删除值
func Del(key string) {
	cache := global()
	if cache == nil {
		return
	}
	cache.Del(key)
}

// Clear 清空全局缓存
func Clear() {
	cache := global()
	if cache == nil {
		return
	}
	cache.Clear()
}

// Wait 等待全局缓存的缓冲写入完成，主要用于测试场景
//
// ristretto 内部使用环形缓冲区，Set 返回后值不一定立即可读。
// 会阻塞到缓冲区排空，不要放在请求路径上。
func Wait() {
	cache := global()
	if cache == nil {
		return
	}
	cache.Wait()
}
