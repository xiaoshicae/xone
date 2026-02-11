package xcache

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/dgraph-io/ristretto"

	"github.com/xiaoshicae/xone/v2/xconfig"
	"github.com/xiaoshicae/xone/v2/xutil"

	. "github.com/bytedance/mockey"
	c "github.com/smartystreets/goconvey/convey"
)

// withCleanCacheMap 保存并清空 cacheMap，测试结束后恢复
func withCleanCacheMap(fn func()) {
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
	fn()
}

// withCleanGlobal 保存并清空 cacheMap + globalCache/globalOnce，测试结束后恢复
func withCleanGlobal(fn func()) {
	cacheMu.Lock()
	origMap := cacheMap
	cacheMap = make(map[string]*Cache)
	cacheMu.Unlock()

	origGlobal := globalCache
	globalOnce = sync.Once{} // nolint: govet // 测试场景下重置 Once 是安全的
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
		globalOnce = sync.Once{} // nolint: govet // 测试场景下重置 Once 是安全的
		globalCache = origGlobal
	}()

	fn()
}

// ==================== config.go ====================

func TestConfigMergeDefault(t *testing.T) {
	PatchConvey("TestConfigMergeDefault", t, func() {
		PatchConvey("Nil", func() {
			config := configMergeDefault(nil)
			c.So(config, c.ShouldResemble, &Config{
				NumCounters: defaultNumCounters,
				MaxCost:     defaultMaxCost,
				BufferItems: defaultBufferItems,
				DefaultTTL:  defaultTTL,
			})
		})

		PatchConvey("PartialCustom", func() {
			config := configMergeDefault(&Config{
				NumCounters: 500000,
				MaxCost:     50000,
				DefaultTTL:  "10m",
			})
			c.So(config, c.ShouldResemble, &Config{
				NumCounters: 500000,
				MaxCost:     50000,
				BufferItems: defaultBufferItems,
				DefaultTTL:  "10m",
			})
		})

		PatchConvey("AllCustom", func() {
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
	})
}

// ==================== cache.go ====================

func TestCacheOperations(t *testing.T) {
	PatchConvey("TestCacheOperations", t, func() {
		cache, err := newCache(configMergeDefault(nil))
		c.So(err, c.ShouldBeNil)
		c.So(cache, c.ShouldNotBeNil)
		defer cache.Close()

		PatchConvey("SetAndGet", func() {
			ok := cache.Set("key1", "value1")
			c.So(ok, c.ShouldBeTrue)
			cache.Wait()

			val, found := cache.Get("key1")
			c.So(found, c.ShouldBeTrue)
			c.So(val, c.ShouldEqual, "value1")
		})

		PatchConvey("SetWithTTL", func() {
			ok := cache.SetWithTTL("key2", "value2", 10*time.Minute)
			c.So(ok, c.ShouldBeTrue)
			cache.Wait()

			val, found := cache.Get("key2")
			c.So(found, c.ShouldBeTrue)
			c.So(val, c.ShouldEqual, "value2")
		})

		PatchConvey("SetWithCost", func() {
			ok := cache.SetWithCost("key3", "value3", 5)
			c.So(ok, c.ShouldBeTrue)
			cache.Wait()

			val, found := cache.Get("key3")
			c.So(found, c.ShouldBeTrue)
			c.So(val, c.ShouldEqual, "value3")
		})

		PatchConvey("SetWithCostAndTTL", func() {
			ok := cache.SetWithCostAndTTL("key4", "value4", 5, 10*time.Minute)
			c.So(ok, c.ShouldBeTrue)
			cache.Wait()

			val, found := cache.Get("key4")
			c.So(found, c.ShouldBeTrue)
			c.So(val, c.ShouldEqual, "value4")
		})

		PatchConvey("Del", func() {
			cache.Set("key5", "value5")
			cache.Wait()

			cache.Del("key5")
			cache.Wait()

			_, found := cache.Get("key5")
			c.So(found, c.ShouldBeFalse)
		})

		PatchConvey("Clear", func() {
			cache.Set("k1", "v1")
			cache.Set("k2", "v2")
			cache.Wait()

			cache.Clear()
			cache.Wait()

			_, found := cache.Get("k1")
			c.So(found, c.ShouldBeFalse)
		})

		PatchConvey("GetNotFound", func() {
			_, found := cache.Get("nonexistent")
			c.So(found, c.ShouldBeFalse)
		})

		PatchConvey("Raw", func() {
			raw := cache.Raw()
			c.So(raw, c.ShouldNotBeNil)
		})
	})
}

// ==================== client.go ====================

func TestC(t *testing.T) {
	PatchConvey("TestC", t, func() {
		PatchConvey("NotConfigured", func() {
			withCleanCacheMap(func() {
				cache := C()
				c.So(cache, c.ShouldBeNil)
			})
		})

		PatchConvey("NotFoundByName", func() {
			withCleanCacheMap(func() {
				cache := C("nonexistent")
				c.So(cache, c.ShouldBeNil)
			})
		})

		PatchConvey("GetDefault", func() {
			withCleanCacheMap(func() {
				testCache, err := newCache(configMergeDefault(nil))
				c.So(err, c.ShouldBeNil)
				defer testCache.Close()

				setDefault(testCache)

				cache := C()
				c.So(cache, c.ShouldNotBeNil)
				c.So(cache, c.ShouldEqual, testCache)
			})
		})

		PatchConvey("GetByName", func() {
			withCleanCacheMap(func() {
				testCache, err := newCache(configMergeDefault(nil))
				c.So(err, c.ShouldBeNil)
				defer testCache.Close()

				set("myCache", testCache)

				cache := C("myCache")
				c.So(cache, c.ShouldNotBeNil)
				c.So(cache, c.ShouldEqual, testCache)
			})
		})

		PatchConvey("MultiArgs", func() {
			withCleanCacheMap(func() {
				testCache, err := newCache(configMergeDefault(nil))
				c.So(err, c.ShouldBeNil)
				defer testCache.Close()

				set("first", testCache)

				// 多参数取第一个
				cache := C("first", "second")
				c.So(cache, c.ShouldNotBeNil)
				c.So(cache, c.ShouldEqual, testCache)
			})
		})
	})
}

// ==================== xcache_init.go ====================

func TestGetConfig(t *testing.T) {
	PatchConvey("TestGetConfig", t, func() {
		PatchConvey("UnmarshalErr", func() {
			Mock(xconfig.UnmarshalConfig).Return(errors.New("unmarshal failed")).Build()
			config, err := getConfig()
			c.So(err, c.ShouldNotBeNil)
			c.So(config, c.ShouldBeNil)
		})

		PatchConvey("Success", func() {
			Mock(xconfig.UnmarshalConfig).Return(nil).Build()
			config, err := getConfig()
			c.So(err, c.ShouldBeNil)
			c.So(config, c.ShouldNotBeNil)
			c.So(config.DefaultTTL, c.ShouldEqual, defaultTTL)
			c.So(config.NumCounters, c.ShouldEqual, int64(defaultNumCounters))
			c.So(config.MaxCost, c.ShouldEqual, int64(defaultMaxCost))
		})
	})
}

func TestGetMultiConfig(t *testing.T) {
	PatchConvey("TestGetMultiConfig", t, func() {
		PatchConvey("UnmarshalErr", func() {
			Mock(xconfig.UnmarshalConfig).Return(errors.New("unmarshal failed")).Build()
			configs, err := getMultiConfig()
			c.So(err, c.ShouldNotBeNil)
			c.So(configs, c.ShouldBeNil)
		})

		PatchConvey("EmptyName", func() {
			Mock(xconfig.UnmarshalConfig).To(func(key string, out any) error {
				configs := out.(*[]*Config)
				*configs = []*Config{{Name: ""}}
				return nil
			}).Build()

			configs, err := getMultiConfig()
			c.So(err, c.ShouldNotBeNil)
			c.So(err.Error(), c.ShouldContainSubstring, "Name can not be empty")
			c.So(configs, c.ShouldBeNil)
		})

		PatchConvey("Success", func() {
			Mock(xconfig.UnmarshalConfig).To(func(key string, out any) error {
				configs := out.(*[]*Config)
				*configs = []*Config{{Name: "n1"}}
				return nil
			}).Build()

			configs, err := getMultiConfig()
			c.So(err, c.ShouldBeNil)
			c.So(configs, c.ShouldHaveLength, 1)
		})
	})
}

func TestInitXCache(t *testing.T) {
	PatchConvey("TestInitXCache", t, func() {
		PatchConvey("NoConfig", func() {
			Mock(xconfig.ContainKey).Return(false).Build()
			Mock(xutil.WarnIfEnableDebug).Return().Build()
			err := initXCache()
			c.So(err, c.ShouldBeNil)
		})

		PatchConvey("SingleMode", func() {
			Mock(xconfig.ContainKey).Return(true).Build()
			Mock(xutil.IsSlice).Return(false).Build()

			PatchConvey("GetConfigErr", func() {
				Mock(getConfig).Return(nil, errors.New("config failed")).Build()
				err := initXCache()
				c.So(err, c.ShouldNotBeNil)
				c.So(err.Error(), c.ShouldContainSubstring, "getConfig failed")
			})

			PatchConvey("NewCacheErr", func() {
				Mock(getConfig).Return(configMergeDefault(nil), nil).Build()
				Mock(xutil.InfoIfEnableDebug).Return().Build()
				Mock(newCache).Return(nil, errors.New("new cache err")).Build()

				err := initXCache()
				c.So(err, c.ShouldNotBeNil)
				c.So(err.Error(), c.ShouldContainSubstring, "newCache failed")
			})

			PatchConvey("Success", func() {
				Mock(getConfig).Return(configMergeDefault(nil), nil).Build()
				Mock(xutil.InfoIfEnableDebug).Return().Build()

				withCleanCacheMap(func() {
					err := initXCache()
					c.So(err, c.ShouldBeNil)

					cache := C()
					c.So(cache, c.ShouldNotBeNil)
				})
			})
		})

		PatchConvey("MultiMode", func() {
			Mock(xconfig.ContainKey).Return(true).Build()
			Mock(xutil.IsSlice).Return(true).Build()

			PatchConvey("GetConfigErr", func() {
				Mock(getMultiConfig).Return(nil, errors.New("multi config failed")).Build()
				err := initXCache()
				c.So(err, c.ShouldNotBeNil)
				c.So(err.Error(), c.ShouldContainSubstring, "getMultiConfig failed")
			})

			PatchConvey("NewCacheErr", func() {
				Mock(getMultiConfig).Return([]*Config{
					configMergeDefault(&Config{Name: "cache1"}),
				}, nil).Build()
				Mock(xutil.InfoIfEnableDebug).Return().Build()
				Mock(newCache).Return(nil, errors.New("new cache err")).Build()

				err := initXCache()
				c.So(err, c.ShouldNotBeNil)
				c.So(err.Error(), c.ShouldContainSubstring, "newCache failed")
			})

			PatchConvey("Success", func() {
				Mock(getMultiConfig).Return([]*Config{
					configMergeDefault(&Config{Name: "cache1"}),
					configMergeDefault(&Config{Name: "cache2"}),
				}, nil).Build()
				Mock(xutil.InfoIfEnableDebug).Return().Build()

				withCleanCacheMap(func() {
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
			})
		})
	})
}

func TestCloseXCache(t *testing.T) {
	PatchConvey("TestCloseXCache", t, func() {
		PatchConvey("EmptyMap", func() {
			withCleanCacheMap(func() {
				err := closeXCache()
				c.So(err, c.ShouldBeNil)
				c.So(cacheMap, c.ShouldBeEmpty)
			})
		})

		PatchConvey("Success", func() {
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
			c.So(cacheMap, c.ShouldBeEmpty)
		})

		PatchConvey("Dedup", func() {
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
			c.So(cacheMap, c.ShouldBeEmpty)
		})

		PatchConvey("WithGlobalCache", func() {
			withCleanGlobal(func() {
				// 触发懒初始化全局缓存
				g := global()
				c.So(g, c.ShouldNotBeNil)
				c.So(globalCache, c.ShouldNotBeNil)

				err := closeXCache()
				c.So(err, c.ShouldBeNil)
				c.So(cacheMap, c.ShouldBeEmpty)
				c.So(globalCache, c.ShouldBeNil)
			})
		})
	})
}

func TestNewCache(t *testing.T) {
	PatchConvey("TestNewCache", t, func() {
		PatchConvey("RistrettoErr", func() {
			Mock(ristretto.NewCache).Return(nil, errors.New("ristretto err")).Build()
			cache, err := newCache(configMergeDefault(nil))
			c.So(err, c.ShouldNotBeNil)
			c.So(err.Error(), c.ShouldContainSubstring, "ristretto.NewCache failed")
			c.So(cache, c.ShouldBeNil)
		})

		PatchConvey("Success", func() {
			cache, err := newCache(configMergeDefault(nil))
			c.So(err, c.ShouldBeNil)
			c.So(cache, c.ShouldNotBeNil)
			c.So(cache.defaultTTL, c.ShouldEqual, 5*time.Minute)
			cache.Close()
		})

		PatchConvey("CustomTTL", func() {
			config := configMergeDefault(&Config{DefaultTTL: "1h"})
			cache, err := newCache(config)
			c.So(err, c.ShouldBeNil)
			c.So(cache, c.ShouldNotBeNil)
			c.So(cache.defaultTTL, c.ShouldEqual, time.Hour)
			cache.Close()
		})
	})
}

// ==================== global_cache.go ====================

type testUser struct {
	Name string
	Age  int
}

func TestGlobal(t *testing.T) {
	PatchConvey("TestGlobal", t, func() {
		PatchConvey("LazyInit", func() {
			withCleanGlobal(func() {
				// 无配置时，global() 应懒初始化一个默认缓存
				g := global()
				c.So(g, c.ShouldNotBeNil)
				c.So(globalCache, c.ShouldNotBeNil)
				c.So(g, c.ShouldEqual, globalCache)
			})
		})

		PatchConvey("LazyInitError", func() {
			withCleanGlobal(func() {
				Mock(newCache).Return(nil, errors.New("new cache err")).Build()
				Mock(xutil.ErrorIfEnableDebug).Return().Build()

				g := global()
				c.So(g, c.ShouldBeNil)
				c.So(globalCache, c.ShouldBeNil)
			})
		})

		PatchConvey("UsesConfiguredCache", func() {
			withCleanGlobal(func() {
				// 有配置的缓存时，global() 应返回配置的缓存而不是创建新的
				configuredCache, _ := newCache(configMergeDefault(nil))
				setDefault(configuredCache)

				g := global()
				c.So(g, c.ShouldEqual, configuredCache)
				c.So(globalCache, c.ShouldBeNil) // 不应创建全局缓存
			})
		})
	})
}

func TestGenericFunctions(t *testing.T) {
	PatchConvey("TestGenericFunctions", t, func() {
		PatchConvey("SetAndGet", func() {
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

		PatchConvey("SetWithTTL", func() {
			withCleanGlobal(func() {
				ok := SetWithTTL("key", "hello", time.Hour)
				c.So(ok, c.ShouldBeTrue)
				global().Wait()

				got, found := Get[string]("key")
				c.So(found, c.ShouldBeTrue)
				c.So(got, c.ShouldEqual, "hello")
			})
		})

		PatchConvey("Del", func() {
			withCleanGlobal(func() {
				Set("to-del", 42)
				global().Wait()

				Del("to-del")
				global().Wait()

				_, found := Get[int]("to-del")
				c.So(found, c.ShouldBeFalse)
			})
		})

		PatchConvey("GetNotFound", func() {
			withCleanGlobal(func() {
				val, found := Get[string]("nonexistent")
				c.So(found, c.ShouldBeFalse)
				c.So(val, c.ShouldEqual, "")
			})
		})

		PatchConvey("GetTypeMismatch", func() {
			withCleanGlobal(func() {
				Set("str-key", "hello")
				global().Wait()

				// 类型不匹配时返回 false
				val, found := Get[int]("str-key")
				c.So(found, c.ShouldBeFalse)
				c.So(val, c.ShouldEqual, 0)
			})
		})

		PatchConvey("NilGlobal", func() {
			Mock(global).Return(nil).Build()

			val, found := Get[string]("key")
			c.So(found, c.ShouldBeFalse)
			c.So(val, c.ShouldEqual, "")

			c.So(Set("key", "val"), c.ShouldBeFalse)
			c.So(SetWithTTL("key", "val", time.Second), c.ShouldBeFalse)
			Del("key") // 不 panic 即通过
		})
	})
}
