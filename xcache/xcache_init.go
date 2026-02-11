package xcache

import (
	"sync"

	"github.com/dgraph-io/ristretto"

	"github.com/xiaoshicae/xone/v2/xconfig"
	"github.com/xiaoshicae/xone/v2/xerror"
	"github.com/xiaoshicae/xone/v2/xhook"
	"github.com/xiaoshicae/xone/v2/xutil"
)

func init() {
	xhook.BeforeStart(initXCache)
	xhook.BeforeStop(closeXCache)
}

func initXCache() error {
	if !xconfig.ContainKey(XCacheConfigKey) {
		xutil.WarnIfEnableDebug("XOne init %s failed, config key [%s] not exists", XCacheConfigKey, XCacheConfigKey)
		return nil
	}

	if xutil.IsSlice(xconfig.GetConfig(XCacheConfigKey)) {
		return initMulti()
	}

	return initSingle()
}

func initSingle() error {
	config, err := getConfig()
	if err != nil {
		return xerror.Newf("xcache", "init", "getConfig failed, err=[%v]", err)
	}
	xutil.InfoIfEnableDebug("XOne init %s got config: %s", XCacheConfigKey, xutil.ToJsonString(config))

	cache, err := newCache(config)
	if err != nil {
		return xerror.Newf("xcache", "init", "newCache failed, err=[%v]", err)
	}

	setDefault(cache)
	return nil
}

func initMulti() error {
	configs, err := getMultiConfig()
	if err != nil {
		return xerror.Newf("xcache", "init", "getMultiConfig failed, err=[%v]", err)
	}
	xutil.InfoIfEnableDebug("XOne init %s got config: %s", XCacheConfigKey, xutil.ToJsonString(configs))

	for idx, config := range configs {
		cache, err := newCache(config)
		if err != nil {
			return xerror.Newf("xcache", "init", "newCache failed, name=[%v], err=[%v]", config.Name, err)
		}

		set(config.Name, cache)

		// 第一个 cache 为 C() 默认获取的 cache
		if idx == 0 {
			setDefault(cache)
		}
	}
	return nil
}

func closeXCache() error {
	cacheMu.Lock()
	defer cacheMu.Unlock()

	// 用于去重，避免同一个 *Cache 被关闭多次（multi 模式下 default 指向第一个 named cache）
	closed := make(map[*Cache]struct{})

	for _, cache := range cacheMap {
		if _, ok := closed[cache]; ok {
			continue
		}
		closed[cache] = struct{}{}
		cache.Close()
	}
	clear(cacheMap)

	// 关闭懒初始化的全局缓存
	if globalCache != nil {
		if _, ok := closed[globalCache]; !ok {
			globalCache.Close()
		}
		globalCache = nil
		globalOnce = sync.Once{}
	}

	return nil
}

func newCache(c *Config) (*Cache, error) {
	raw, err := ristretto.NewCache(&ristretto.Config{
		NumCounters: c.NumCounters,
		MaxCost:     c.MaxCost,
		BufferItems: c.BufferItems,
	})
	if err != nil {
		return nil, xerror.Newf("xcache", "newCache", "ristretto.NewCache failed, err=[%v]", err)
	}

	return &Cache{
		raw:        raw,
		defaultTTL: xutil.ToDuration(c.DefaultTTL),
	}, nil
}

func getConfig() (*Config, error) {
	c := &Config{}
	if err := xconfig.UnmarshalConfig(XCacheConfigKey, c); err != nil {
		return nil, err
	}
	c = configMergeDefault(c)
	return c, nil
}

func getMultiConfig() ([]*Config, error) {
	var multiConfig []*Config
	if err := xconfig.UnmarshalConfig(XCacheConfigKey, &multiConfig); err != nil {
		return nil, err
	}
	for _, c := range multiConfig {
		c = configMergeDefault(c)
		if c.Name == "" {
			return nil, xerror.Newf("xcache", "getMultiConfig", "multi config XCache.Name can not be empty")
		}
	}
	return multiConfig, nil
}
