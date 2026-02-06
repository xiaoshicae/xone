package xcache

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/bytedance/mockey"
	c "github.com/smartystreets/goconvey/convey"

	"github.com/xiaoshicae/xone/xconfig"
	"github.com/xiaoshicae/xone/xutil"
)

func TestXCacheConfig(t *testing.T) {
	mockey.PatchConvey("TestXCacheConfig-configMergeDefault-Nil", t, func() {
		config := configMergeDefault(nil)
		c.So(config, c.ShouldResemble, &Config{
			NumCounters: 1000000,
			MaxCost:     100000,
			BufferItems: 64,
			DefaultTTL:  "5m",
		})
	})

	mockey.PatchConvey("TestXCacheConfig-configMergeDefault-NotNil", t, func() {
		config := configMergeDefault(&Config{
			NumCounters: 500000,
			MaxCost:     50000,
			DefaultTTL:  "10m",
		})
		c.So(config, c.ShouldResemble, &Config{
			NumCounters: 500000,
			MaxCost:     50000,
			BufferItems: 64,
			DefaultTTL:  "10m",
		})
	})

	mockey.PatchConvey("TestXCacheConfig-configMergeDefault-AllCustom", t, func() {
		config := configMergeDefault(&Config{
			NumCounters: 200000,
			MaxCost:     20000,
			BufferItems: 128,
			DefaultTTL:  "1h",
			Name:        "test",
		})
		c.So(config, c.ShouldResemble, &Config{
			NumCounters: 200000,
			MaxCost:     20000,
			BufferItems: 128,
			DefaultTTL:  "1h",
			Name:        "test",
		})
	})
}

func TestCacheOperations(t *testing.T) {
	mockey.PatchConvey("TestCacheOperations-SetAndGet", t, func() {
		cache, err := newCache(configMergeDefault(nil))
		c.So(err, c.ShouldBeNil)
		c.So(cache, c.ShouldNotBeNil)
		defer cache.Close()

		ok := cache.Set("key1", "value1")
		c.So(ok, c.ShouldBeTrue)
		cache.Wait()

		val, found := cache.Get("key1")
		c.So(found, c.ShouldBeTrue)
		c.So(val, c.ShouldEqual, "value1")
	})

	mockey.PatchConvey("TestCacheOperations-SetWithTTL", t, func() {
		cache, err := newCache(configMergeDefault(nil))
		c.So(err, c.ShouldBeNil)
		defer cache.Close()

		ok := cache.SetWithTTL("key2", "value2", 10*time.Minute)
		c.So(ok, c.ShouldBeTrue)
		cache.Wait()

		val, found := cache.Get("key2")
		c.So(found, c.ShouldBeTrue)
		c.So(val, c.ShouldEqual, "value2")
	})

	mockey.PatchConvey("TestCacheOperations-SetWithCost", t, func() {
		cache, err := newCache(configMergeDefault(nil))
		c.So(err, c.ShouldBeNil)
		defer cache.Close()

		ok := cache.SetWithCost("key3", "value3", 5)
		c.So(ok, c.ShouldBeTrue)
		cache.Wait()

		val, found := cache.Get("key3")
		c.So(found, c.ShouldBeTrue)
		c.So(val, c.ShouldEqual, "value3")
	})

	mockey.PatchConvey("TestCacheOperations-SetWithCostAndTTL", t, func() {
		cache, err := newCache(configMergeDefault(nil))
		c.So(err, c.ShouldBeNil)
		defer cache.Close()

		ok := cache.SetWithCostAndTTL("key4", "value4", 5, 10*time.Minute)
		c.So(ok, c.ShouldBeTrue)
		cache.Wait()

		val, found := cache.Get("key4")
		c.So(found, c.ShouldBeTrue)
		c.So(val, c.ShouldEqual, "value4")
	})

	mockey.PatchConvey("TestCacheOperations-Del", t, func() {
		cache, err := newCache(configMergeDefault(nil))
		c.So(err, c.ShouldBeNil)
		defer cache.Close()

		cache.Set("key5", "value5")
		cache.Wait()

		cache.Del("key5")
		cache.Wait()

		_, found := cache.Get("key5")
		c.So(found, c.ShouldBeFalse)
	})

	mockey.PatchConvey("TestCacheOperations-Clear", t, func() {
		cache, err := newCache(configMergeDefault(nil))
		c.So(err, c.ShouldBeNil)
		defer cache.Close()

		cache.Set("k1", "v1")
		cache.Set("k2", "v2")
		cache.Wait()

		cache.Clear()
		cache.Wait()

		_, found := cache.Get("k1")
		c.So(found, c.ShouldBeFalse)
	})

	mockey.PatchConvey("TestCacheOperations-GetNotFound", t, func() {
		cache, err := newCache(configMergeDefault(nil))
		c.So(err, c.ShouldBeNil)
		defer cache.Close()

		_, found := cache.Get("nonexistent")
		c.So(found, c.ShouldBeFalse)
	})

	mockey.PatchConvey("TestCacheOperations-Raw", t, func() {
		cache, err := newCache(configMergeDefault(nil))
		c.So(err, c.ShouldBeNil)
		defer cache.Close()

		raw := cache.Raw()
		c.So(raw, c.ShouldNotBeNil)
	})
}

func TestXCacheClient(t *testing.T) {
	mockey.PatchConvey("TestXCacheClient-GetNotConfigured", t, func() {
		// 清空 cacheMap
		cacheMu.Lock()
		origMap := cacheMap
		cacheMap = make(map[string]*Cache)
		cacheMu.Unlock()
		defer func() {
			cacheMu.Lock()
			cacheMap = origMap
			cacheMu.Unlock()
		}()

		mockey.Mock(xutil.ErrorIfEnableDebug).Return().Build()

		cache := C()
		c.So(cache, c.ShouldBeNil)
	})

	mockey.PatchConvey("TestXCacheClient-GetDefault", t, func() {
		testCache, err := newCache(configMergeDefault(nil))
		c.So(err, c.ShouldBeNil)
		defer testCache.Close()

		cacheMu.Lock()
		origMap := cacheMap
		cacheMap = make(map[string]*Cache)
		cacheMu.Unlock()
		defer func() {
			cacheMu.Lock()
			cacheMap = origMap
			cacheMu.Unlock()
		}()

		setDefault(testCache)

		cache := C()
		c.So(cache, c.ShouldNotBeNil)
		c.So(cache, c.ShouldEqual, testCache)
	})

	mockey.PatchConvey("TestXCacheClient-GetByName", t, func() {
		testCache, err := newCache(configMergeDefault(nil))
		c.So(err, c.ShouldBeNil)
		defer testCache.Close()

		cacheMu.Lock()
		origMap := cacheMap
		cacheMap = make(map[string]*Cache)
		cacheMu.Unlock()
		defer func() {
			cacheMu.Lock()
			cacheMap = origMap
			cacheMu.Unlock()
		}()

		set("myCache", testCache)

		cache := C("myCache")
		c.So(cache, c.ShouldNotBeNil)
		c.So(cache, c.ShouldEqual, testCache)
	})
}

func TestGetConfigXCache(t *testing.T) {
	mockey.PatchConvey("TestGetConfig-UnmarshalFail", t, func() {
		mockey.Mock(xconfig.UnmarshalConfig).Return(errors.New("unmarshal failed")).Build()

		config, err := getConfig()
		c.So(err, c.ShouldNotBeNil)
		c.So(config, c.ShouldBeNil)
	})

	mockey.PatchConvey("TestGetConfig-Success", t, func() {
		mockey.Mock(xconfig.UnmarshalConfig).Return(nil).Build()

		config, err := getConfig()
		c.So(err, c.ShouldBeNil)
		c.So(config, c.ShouldNotBeNil)
		c.So(config.DefaultTTL, c.ShouldEqual, "5m")
		c.So(config.NumCounters, c.ShouldEqual, 1000000)
		c.So(config.MaxCost, c.ShouldEqual, 100000)
	})
}

func TestGetMultiConfigXCache(t *testing.T) {
	mockey.PatchConvey("TestGetMultiConfig-UnmarshalFail", t, func() {
		mockey.Mock(xconfig.UnmarshalConfig).Return(errors.New("unmarshal failed")).Build()

		configs, err := getMultiConfig()
		c.So(err, c.ShouldNotBeNil)
		c.So(configs, c.ShouldBeNil)
	})

	mockey.PatchConvey("TestGetMultiConfig-EmptyName", t, func() {
		mockey.Mock(xconfig.UnmarshalConfig).To(func(key string, out any) error {
			configs := out.(*[]*Config)
			*configs = []*Config{{Name: ""}}
			return nil
		}).Build()

		configs, err := getMultiConfig()
		c.So(err, c.ShouldNotBeNil)
		c.So(err.Error(), c.ShouldContainSubstring, "Name can not be empty")
		c.So(configs, c.ShouldBeNil)
	})
}

func TestInitXCache(t *testing.T) {
	mockey.PatchConvey("TestInitXCache-NoConfig", t, func() {
		mockey.Mock(xconfig.ContainKey).Return(false).Build()

		err := initXCache()
		c.So(err, c.ShouldBeNil)
	})

	mockey.PatchConvey("TestInitXCache-SingleMode-GetConfigFail", t, func() {
		mockey.Mock(xconfig.ContainKey).Return(true).Build()
		mockey.Mock(xutil.IsSlice).Return(false).Build()
		mockey.Mock(getConfig).Return(nil, errors.New("config failed")).Build()

		err := initXCache()
		c.So(err, c.ShouldNotBeNil)
		c.So(err.Error(), c.ShouldContainSubstring, "getConfig failed")
	})

	mockey.PatchConvey("TestInitXCache-SingleMode-Success", t, func() {
		mockey.Mock(xconfig.ContainKey).Return(true).Build()
		mockey.Mock(xutil.IsSlice).Return(false).Build()
		mockey.Mock(getConfig).Return(configMergeDefault(nil), nil).Build()

		cacheMu.Lock()
		origMap := cacheMap
		cacheMap = make(map[string]*Cache)
		cacheMu.Unlock()
		defer func() {
			cacheMu.Lock()
			for _, v := range cacheMap {
				v.Close()
			}
			cacheMap = origMap
			cacheMu.Unlock()
		}()

		err := initXCache()
		c.So(err, c.ShouldBeNil)

		cache := C()
		c.So(cache, c.ShouldNotBeNil)
	})

	mockey.PatchConvey("TestInitXCache-MultiMode-GetConfigFail", t, func() {
		mockey.Mock(xconfig.ContainKey).Return(true).Build()
		mockey.Mock(xutil.IsSlice).Return(true).Build()
		mockey.Mock(getMultiConfig).Return(nil, errors.New("multi config failed")).Build()

		err := initXCache()
		c.So(err, c.ShouldNotBeNil)
		c.So(err.Error(), c.ShouldContainSubstring, "getMultiConfig failed")
	})

	mockey.PatchConvey("TestInitXCache-MultiMode-Success", t, func() {
		mockey.Mock(xconfig.ContainKey).Return(true).Build()
		mockey.Mock(xutil.IsSlice).Return(true).Build()
		mockey.Mock(getMultiConfig).Return([]*Config{
			configMergeDefault(&Config{Name: "cache1"}),
			configMergeDefault(&Config{Name: "cache2"}),
		}, nil).Build()

		cacheMu.Lock()
		origMap := cacheMap
		cacheMap = make(map[string]*Cache)
		cacheMu.Unlock()
		defer func() {
			cacheMu.Lock()
			for _, v := range cacheMap {
				v.Close()
			}
			cacheMap = origMap
			cacheMu.Unlock()
		}()

		err := initXCache()
		c.So(err, c.ShouldBeNil)

		c1 := C("cache1")
		c.So(c1, c.ShouldNotBeNil)

		c2 := C("cache2")
		c.So(c2, c.ShouldNotBeNil)

		// 默认 cache 应该是第一个
		defaultCache := C()
		c.So(defaultCache, c.ShouldNotBeNil)
		c.So(defaultCache, c.ShouldEqual, c1)
	})
}

func TestCloseXCache(t *testing.T) {
	mockey.PatchConvey("TestCloseXCache-Success", t, func() {
		cache1, _ := newCache(configMergeDefault(nil))
		cache2, _ := newCache(configMergeDefault(nil))

		cacheMu.Lock()
		origMap := cacheMap
		cacheMap = map[string]*Cache{
			defaultCacheName: cache1,
			"named":          cache2,
		}
		cacheMu.Unlock()
		defer func() {
			cacheMu.Lock()
			cacheMap = origMap
			cacheMu.Unlock()
		}()

		err := closeXCache()
		c.So(err, c.ShouldBeNil)
	})

	mockey.PatchConvey("TestCloseXCache-Dedup", t, func() {
		cache1, _ := newCache(configMergeDefault(nil))

		cacheMu.Lock()
		origMap := cacheMap
		// default 和 named 指向同一个 cache 实例
		cacheMap = map[string]*Cache{
			defaultCacheName: cache1,
			"named":          cache1,
		}
		cacheMu.Unlock()
		defer func() {
			cacheMu.Lock()
			cacheMap = origMap
			cacheMu.Unlock()
		}()

		err := closeXCache()
		c.So(err, c.ShouldBeNil)
	})
}

func TestNewCache(t *testing.T) {
	mockey.PatchConvey("TestNewCache-Success", t, func() {
		cache, err := newCache(configMergeDefault(nil))
		c.So(err, c.ShouldBeNil)
		c.So(cache, c.ShouldNotBeNil)
		c.So(cache.defaultTTL, c.ShouldEqual, 5*time.Minute)
		cache.Close()
	})

	mockey.PatchConvey("TestNewCache-CustomTTL", t, func() {
		config := configMergeDefault(&Config{DefaultTTL: "1h"})
		cache, err := newCache(config)
		c.So(err, c.ShouldBeNil)
		c.So(cache, c.ShouldNotBeNil)
		c.So(cache.defaultTTL, c.ShouldEqual, time.Hour)
		cache.Close()
	})
}

// --- 全局缓存 & 泛型 API 测试 ---

type testUser struct {
	Name string
	Age  int
}

func withCleanGlobal(fn func()) {
	cacheMu.Lock()
	origMap := cacheMap
	cacheMap = make(map[string]*Cache)
	cacheMu.Unlock()

	origOnce := globalOnce
	origGlobal := globalCache
	globalOnce = sync.Once{}
	globalCache = nil

	defer func() {
		if globalCache != nil {
			globalCache.Close()
		}
		cacheMu.Lock()
		for _, v := range cacheMap {
			v.Close()
		}
		cacheMap = origMap
		cacheMu.Unlock()
		globalOnce = origOnce
		globalCache = origGlobal
	}()

	fn()
}

func TestGlobalLazyInit(t *testing.T) {
	mockey.PatchConvey("TestGlobal-LazyInit", t, func() {
		withCleanGlobal(func() {
			// 无配置时，global() 应懒初始化一个默认缓存
			g := global()
			c.So(g, c.ShouldNotBeNil)
			c.So(globalCache, c.ShouldNotBeNil)
			c.So(g, c.ShouldEqual, globalCache)
		})
	})

	mockey.PatchConvey("TestGlobal-UsesConfiguredCache", t, func() {
		withCleanGlobal(func() {
			// 有配置的缓存时，global() 应返回配置的缓存而不是创建新的
			configuredCache, _ := newCache(configMergeDefault(nil))
			setDefault(configuredCache)

			g := global()
			c.So(g, c.ShouldEqual, configuredCache)
			c.So(globalCache, c.ShouldBeNil) // 不应创建全局缓存
		})
	})
}

func TestPackageLevelGenericFunctions(t *testing.T) {
	mockey.PatchConvey("TestGeneric-SetAndGet", t, func() {
		withCleanGlobal(func() {
			user := &testUser{Name: "alice", Age: 30}

			ok := Set("user:1", user)
			c.So(ok, c.ShouldBeTrue)
			global().Wait()

			got, found := Get[*testUser]("user:1")
			c.So(found, c.ShouldBeTrue)
			c.So(got.Name, c.ShouldEqual, "alice")
			c.So(got.Age, c.ShouldEqual, 30)
		})
	})

	mockey.PatchConvey("TestGeneric-SetWithTTL", t, func() {
		withCleanGlobal(func() {
			ok := SetWithTTL("key", "hello", time.Hour)
			c.So(ok, c.ShouldBeTrue)
			global().Wait()

			got, found := Get[string]("key")
			c.So(found, c.ShouldBeTrue)
			c.So(got, c.ShouldEqual, "hello")
		})
	})

	mockey.PatchConvey("TestGeneric-Del", t, func() {
		withCleanGlobal(func() {
			Set("to-del", 42)
			global().Wait()

			Del("to-del")
			global().Wait()

			_, found := Get[int]("to-del")
			c.So(found, c.ShouldBeFalse)
		})
	})

	mockey.PatchConvey("TestGeneric-GetNotFound", t, func() {
		withCleanGlobal(func() {
			val, found := Get[string]("nonexistent")
			c.So(found, c.ShouldBeFalse)
			c.So(val, c.ShouldEqual, "")
		})
	})

	mockey.PatchConvey("TestGeneric-GetTypeMismatch", t, func() {
		withCleanGlobal(func() {
			Set("str-key", "hello")
			global().Wait()

			// 类型不匹配时返回 false
			val, found := Get[int]("str-key")
			c.So(found, c.ShouldBeFalse)
			c.So(val, c.ShouldEqual, 0)
		})
	})
}

