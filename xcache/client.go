package xcache

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"

	"github.com/xiaoshicae/xone/v3/xutil"
)

const defaultCacheName = "__default_cache__"

var (
	cacheMap = make(map[string]*Cache)
	cacheMu  sync.RWMutex

	globalCache *Cache

	// closed 模块是否已执行过关闭
	//
	// global() 是懒初始化的：不记住这个状态的话，BeforeStop 之后任何一次
	// Get/Set 都会重建一个 ristretto 实例，而它再也不会被关闭。
	// 用户的 BeforeStop hook 在 xcache 之后执行（同为默认 Order，按 LIFO 反序），
	// 里面读一次缓存就会触发
	closed bool
)

// C 获取具名缓存实例，name 为空则取默认实例
//
// 找不到时 panic。返回 nil 并不会让程序走得更远——*Cache 的任何方法在 nil 上
// 都是空指针解引用，只是把同一个 panic 推迟到调用方第一次用它的时候，
// 而那里的栈里只剩 "invalid memory address"，看不出根因是配置没配。
// 这是启动期的配置问题，不是运行期需要处理的错误。
//
// 注意这和包级的 Get/Set 不同：后者操作的是全局缓存，未配置 XCache 时会懒初始化
// 一个默认实例，因此不会 panic。C 取的是配置里写明的具名实例，要不到就是配置问题。
//
// 可选依赖（配了就用、没配就跳过）用 Has() 先判断。
func C(name ...string) *Cache {
	cache := get(name...)
	if cache == nil {
		panic(noCacheMsg(name...))
	}
	return cache
}

// Has 报告指定缓存实例是否已配置
//
// 供可选依赖使用：配了就用、没配就跳过，不必用 C 去触发 panic。
func Has(name ...string) bool {
	return get(name...) != nil
}

// noCacheMsg 拼装 panic 信息，带上已配置的缓存名称
//
// 只报"没找到"帮助有限：名字写错和整个 XCache 没配是两个不同的问题，
// 列出实际配了哪些，两者一眼可分。
func noCacheMsg(name ...string) string {
	want := "default"
	if len(name) > 0 {
		want = name[0]
	}

	configured := cacheNames()
	if len(configured) == 0 {
		return fmt.Sprintf("XOne xcache: no cache found for name=[%s], no cache configured at all, check the XCache section of your config", want)
	}
	return fmt.Sprintf("XOne xcache: no cache found for name=[%s], configured=[%s]", want, strings.Join(configured, " "))
}

// cacheNames 返回已配置的缓存名称，不含内部默认别名
func cacheNames() []string {
	cacheMu.RLock()
	defer cacheMu.RUnlock()

	names := slices.Sorted(maps.Keys(cacheMap))
	return slices.DeleteFunc(names, func(n string) bool { return n == defaultCacheName })
}

// global 获取全局缓存实例，如果没有配置的缓存则懒初始化一个默认缓存
// 模块关闭后返回 nil，不再重建
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
	if closed {
		// 关闭后不再重建：新建的实例不会有人再来关，等于永久泄漏
		xutil.WarnIfEnableDebug("XOne xcache already closed, skip creating default global cache")
		return nil
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

// removeCaches 把指定配置对应的 cache 从 cacheMap 中摘除，用于初始化失败回滚
func removeCaches(configs []*Config) {
	cacheMu.Lock()
	defer cacheMu.Unlock()
	for _, c := range configs {
		delete(cacheMap, c.Name)
	}
	delete(cacheMap, defaultCacheName)
}
