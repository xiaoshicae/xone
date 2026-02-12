package xcache

import (
	"context"
	"sync"

	"github.com/xiaoshicae/xone/v2/xlog"
	"github.com/xiaoshicae/xone/v2/xutil"
)

const defaultCacheName = "__default_cache__"

var (
	cacheMap = make(map[string]*Cache)
	cacheMu  sync.RWMutex

	globalCache *Cache
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

// global 获取全局缓存实例，如果没有配置的缓存则懒初始化一个默认缓存
func global() *Cache {
	// 快速路径：读锁检查是否已有配置的缓存
	if cache := get(); cache != nil {
		return cache
	}

	// 慢路径：写锁下懒初始化全局缓存（双重检查避免竞态）
	cacheMu.Lock()
	defer cacheMu.Unlock()

	if cache := cacheMap[defaultCacheName]; cache != nil {
		return cache
	}
	if globalCache != nil {
		return globalCache
	}

	c, err := newCache(configMergeDefault(nil))
	if err != nil {
		xutil.ErrorIfEnableDebug("XOne xcache create default global cache failed, err=[%v]", err)
		return nil
	}
	globalCache = c
	return globalCache
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
