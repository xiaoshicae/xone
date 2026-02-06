package xcache

import (
	"context"
	"sync"

	"github.com/xiaoshicae/xone/xlog"
)

const defaultCacheName = "__default_cache__"

var (
	cacheMap = make(map[string]*Cache)
	cacheMu  sync.RWMutex
)

// C 获取缓存实例，支持指定名称获取，name 为空则默认获取第一个缓存实例
func C(name ...string) *Cache {
	cache := get(name...)
	if cache != nil {
		return cache
	}

	n := ""
	if len(name) > 0 {
		n = name[0]
	}
	xlog.Error(context.Background(), "no cache found for name: %s, maybe config not assigned", n)
	return nil
}

func get(name ...string) *Cache {
	n := defaultCacheName
	if len(name) > 0 {
		n = name[0]
	}

	cacheMu.RLock()
	defer cacheMu.RUnlock()
	return cacheMap[n]
}

func set(name string, cache *Cache) {
	cacheMu.Lock()
	defer cacheMu.Unlock()
	cacheMap[name] = cache
}

func setDefault(cache *Cache) {
	cacheMu.Lock()
	defer cacheMu.Unlock()
	cacheMap[defaultCacheName] = cache
}
