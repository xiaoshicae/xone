package xcache

import (
	"github.com/dgraph-io/ristretto"

	"github.com/xiaoshicae/xone/v3/xconfig"
	"github.com/xiaoshicae/xone/v3/xerror"
	"github.com/xiaoshicae/xone/v3/xhook"
	"github.com/xiaoshicae/xone/v3/xutil"
)

func init() {
	xhook.BeforeStart(initXCache)
	xhook.BeforeStop(closeXCache)
}

func initXCache() error {
	cacheMu.Lock()
	closed = false // 重新初始化时解除关闭状态，支持重复初始化
	cacheMu.Unlock()

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

	created := make([]*Cache, 0, len(configs))
	for idx, config := range configs {
		cache, err := newCache(config)
		if err != nil {
			// 回滚：关闭已创建的实例并从 cacheMap 中摘除
			for _, c := range created {
				c.Close()
			}
			removeCaches(configs[:idx])
			return xerror.Newf("xcache", "init", "newCache failed, name=[%v], err=[%v]", config.Name, err)
		}

		created = append(created, cache)
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

	closed = true

	// 用于去重，避免同一个 *Cache 被关闭多次（multi 模式下 default 指向第一个 named cache）
	// 不叫 closed：那样会遮蔽上面的包级标志位，读的人得停下来确认一次
	seen := make(map[*Cache]struct{})

	for _, cache := range cacheMap {
		if _, ok := seen[cache]; ok {
			continue
		}
		seen[cache] = struct{}{}
		cache.Close()
	}
	clear(cacheMap)

	// 关闭懒初始化的全局缓存
	if globalCache != nil {
		if _, ok := seen[globalCache]; !ok {
			globalCache.Close()
		}
		globalCache = nil
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
	seen := make(map[string]struct{}, len(multiConfig))
	for i, c := range multiConfig {
		multiConfig[i] = configMergeDefault(c)
		c = multiConfig[i]
		if c.Name == "" {
			return nil, xerror.Newf("xcache", "getMultiConfig", "multi config XCache.Name can not be empty")
		}
		if c.Name == defaultCacheName {
			return nil, xerror.Newf("xcache", "getMultiConfig", "multi config XCache.Name can not be reserved name [%s]", defaultCacheName)
		}
		if _, ok := seen[c.Name]; ok {
			return nil, xerror.Newf("xcache", "getMultiConfig", "multi config XCache.Name [%s] is duplicated", c.Name)
		}
		seen[c.Name] = struct{}{}
	}
	return multiConfig, nil
}
