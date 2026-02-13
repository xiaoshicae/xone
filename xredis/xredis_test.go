package xredis

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/redis/go-redis/extra/redisotel/v9"
	redis "github.com/redis/go-redis/v9"
	"github.com/xiaoshicae/xone/v2/xconfig"
	"github.com/xiaoshicae/xone/v2/xtrace"
	"github.com/xiaoshicae/xone/v2/xutil"

	"github.com/bytedance/mockey"
	c "github.com/smartystreets/goconvey/convey"
)

func TestConfigMergeDefault(t *testing.T) {
	mockey.PatchConvey("TestConfigMergeDefault-Nil", t, func() {
		config := configMergeDefault(nil)
		c.So(config, c.ShouldResemble, &Config{
			Addr:            "localhost:6379",
			DialTimeout:     "500ms",
			ReadTimeout:     "500ms",
			WriteTimeout:    "500ms",
			MinIdleConns:    5,
			PoolTimeout:     "1s",
			ConnMaxIdleTime: "5m",
			ConnMaxLifetime: "5m",
		})
	})

	mockey.PatchConvey("TestConfigMergeDefault-NotNil", t, func() {
		config := &Config{
			Addr:     "redis.example.com:6379",
			PoolSize: 20,
		}
		config = configMergeDefault(config)
		c.So(config.Addr, c.ShouldEqual, "redis.example.com:6379")
		c.So(config.PoolSize, c.ShouldEqual, 20)
		c.So(config.DialTimeout, c.ShouldEqual, "500ms")
		c.So(config.ReadTimeout, c.ShouldEqual, "500ms")
	})

	mockey.PatchConvey("TestConfigMergeDefault-MaxRetriesPassthrough", t, func() {
		// -1 表示禁用重试，应该透传给 go-redis 处理
		config := &Config{MaxRetries: -1}
		config = configMergeDefault(config)
		c.So(config.MaxRetries, c.ShouldEqual, -1)

		// 0 表示使用 go-redis 默认值（3次），不应被覆盖
		config2 := &Config{MaxRetries: 0}
		config2 = configMergeDefault(config2)
		c.So(config2.MaxRetries, c.ShouldEqual, 0)
	})
}

func TestSanitizeConfigForLog(t *testing.T) {
	mockey.PatchConvey("TestSanitizeConfigForLog-WithPassword", t, func() {
		config := &Config{
			Addr:     "localhost:6379",
			Password: "secret123",
		}
		sanitized := sanitizeConfigForLog(config)
		c.So(sanitized.Password, c.ShouldEqual, "***")
		c.So(sanitized.Addr, c.ShouldEqual, "localhost:6379")
		// 原始对象不受影响
		c.So(config.Password, c.ShouldEqual, "secret123")
	})

	mockey.PatchConvey("TestSanitizeConfigForLog-NoPassword", t, func() {
		config := &Config{
			Addr: "localhost:6379",
		}
		sanitized := sanitizeConfigForLog(config)
		c.So(sanitized.Password, c.ShouldEqual, "")
	})
}

func TestSanitizeConfigsForLog(t *testing.T) {
	mockey.PatchConvey("TestSanitizeConfigsForLog", t, func() {
		configs := []*Config{
			{Addr: "host1:6379", Password: "pass1"},
			{Addr: "host2:6379", Password: "pass2"},
		}
		sanitized := sanitizeConfigsForLog(configs)
		c.So(len(sanitized), c.ShouldEqual, 2)
		c.So(sanitized[0].Password, c.ShouldEqual, "***")
		c.So(sanitized[1].Password, c.ShouldEqual, "***")
	})
}

func TestC(t *testing.T) {
	mockey.PatchConvey("TestC-NotFound", t, func() {
		clientMu.Lock()
		clear(clientMap)
		clientMu.Unlock()

		c.So(C(), c.ShouldBeNil)
	})

	mockey.PatchConvey("TestC-Found", t, func() {
		rdb := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
		defer rdb.Close()

		setDefault(rdb)
		c.So(C() == rdb, c.ShouldBeTrue)

		// 清理
		clientMu.Lock()
		clear(clientMap)
		clientMu.Unlock()
	})

	mockey.PatchConvey("TestC-NamedClient", t, func() {
		rdb := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
		defer rdb.Close()

		set("cache", rdb)
		c.So(C("cache") == rdb, c.ShouldBeTrue)
		c.So(C("nonexistent"), c.ShouldBeNil)

		// 清理
		clientMu.Lock()
		clear(clientMap)
		clientMu.Unlock()
	})
}

func TestGetConfig(t *testing.T) {
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
		c.So(config.Addr, c.ShouldEqual, "localhost:6379") // 默认值
		c.So(config.PoolSize, c.ShouldEqual, 0)            // 0 由 go-redis 处理为 10*GOMAXPROCS
	})
}

func TestGetMultiConfig(t *testing.T) {
	mockey.PatchConvey("TestGetMultiConfig-UnmarshalFail", t, func() {
		mockey.Mock(xconfig.UnmarshalConfig).Return(errors.New("unmarshal failed")).Build()

		configs, err := getMultiConfig()
		c.So(err, c.ShouldNotBeNil)
		c.So(configs, c.ShouldBeNil)
	})

	mockey.PatchConvey("TestGetMultiConfig-EmptyName", t, func() {
		mockey.Mock(xconfig.UnmarshalConfig).To(func(key string, conf any) error {
			ptr := conf.(*[]*Config)
			*ptr = []*Config{{Addr: "host1:6379"}}
			return nil
		}).Build()

		configs, err := getMultiConfig()
		c.So(err, c.ShouldNotBeNil)
		c.So(err.Error(), c.ShouldContainSubstring, "Name can not be empty")
		c.So(configs, c.ShouldBeNil)
	})

	mockey.PatchConvey("TestGetMultiConfig-ReservedName", t, func() {
		mockey.Mock(xconfig.UnmarshalConfig).To(func(key string, conf any) error {
			ptr := conf.(*[]*Config)
			*ptr = []*Config{{Addr: "host1:6379", Name: defaultClientName}}
			return nil
		}).Build()

		configs, err := getMultiConfig()
		c.So(err, c.ShouldNotBeNil)
		c.So(err.Error(), c.ShouldContainSubstring, "reserved name")
		c.So(configs, c.ShouldBeNil)
	})

	mockey.PatchConvey("TestGetMultiConfig-DuplicateName", t, func() {
		mockey.Mock(xconfig.UnmarshalConfig).To(func(key string, conf any) error {
			ptr := conf.(*[]*Config)
			*ptr = []*Config{
				{Addr: "host1:6379", Name: "cache"},
				{Addr: "host2:6379", Name: "cache"},
			}
			return nil
		}).Build()

		configs, err := getMultiConfig()
		c.So(err, c.ShouldNotBeNil)
		c.So(err.Error(), c.ShouldContainSubstring, "duplicated")
		c.So(configs, c.ShouldBeNil)
	})

	mockey.PatchConvey("TestGetMultiConfig-Success", t, func() {
		mockey.Mock(xconfig.UnmarshalConfig).To(func(key string, conf any) error {
			ptr := conf.(*[]*Config)
			*ptr = []*Config{
				{Addr: "host1:6379", Name: "cache"},
				{Addr: "host2:6379", Name: "session"},
			}
			return nil
		}).Build()

		configs, err := getMultiConfig()
		c.So(err, c.ShouldBeNil)
		c.So(len(configs), c.ShouldEqual, 2)
		c.So(configs[0].Name, c.ShouldEqual, "cache")
		c.So(configs[1].Name, c.ShouldEqual, "session")
	})
}

func TestInitXRedis(t *testing.T) {
	mockey.PatchConvey("TestInitXRedis-ConfigKeyNotFound", t, func() {
		mockey.Mock(xconfig.ContainKey).Return(false).Build()

		err := initXRedis()
		c.So(err, c.ShouldBeNil)
	})

	mockey.PatchConvey("TestInitXRedis-SingleMode-GetConfigFail", t, func() {
		mockey.Mock(xconfig.ContainKey).Return(true).Build()
		mockey.Mock(xutil.IsSlice).Return(false).Build()
		mockey.Mock(getConfig).Return(nil, errors.New("config error")).Build()

		err := initXRedis()
		c.So(err, c.ShouldNotBeNil)
		c.So(err.Error(), c.ShouldContainSubstring, "getConfig failed")
	})

	mockey.PatchConvey("TestInitXRedis-SingleMode-NewClientFail", t, func() {
		mockey.Mock(xconfig.ContainKey).Return(true).Build()
		mockey.Mock(xutil.IsSlice).Return(false).Build()
		mockey.Mock(getConfig).Return(&Config{Addr: "localhost:6379"}, nil).Build()
		mockey.Mock(newClient).Return(nil, errors.New("connect failed")).Build()

		err := initXRedis()
		c.So(err, c.ShouldNotBeNil)
		c.So(err.Error(), c.ShouldContainSubstring, "newClient failed")
	})

	mockey.PatchConvey("TestInitXRedis-SingleMode-Success", t, func() {
		mockey.Mock(xconfig.ContainKey).Return(true).Build()
		mockey.Mock(xutil.IsSlice).Return(false).Build()
		mockey.Mock(getConfig).Return(&Config{Addr: "localhost:6379"}, nil).Build()

		rdb := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
		defer rdb.Close()
		mockey.Mock(newClient).Return(rdb, nil).Build()

		err := initXRedis()
		c.So(err, c.ShouldBeNil)

		// 清理
		clientMu.Lock()
		clear(clientMap)
		clientMu.Unlock()
	})

	mockey.PatchConvey("TestInitXRedis-MultiMode-GetMultiConfigFail", t, func() {
		mockey.Mock(xconfig.ContainKey).Return(true).Build()
		mockey.Mock(xutil.IsSlice).Return(true).Build()
		mockey.Mock(getMultiConfig).Return(nil, errors.New("multi config error")).Build()

		err := initXRedis()
		c.So(err, c.ShouldNotBeNil)
		c.So(err.Error(), c.ShouldContainSubstring, "getMultiConfig failed")
	})

	mockey.PatchConvey("TestInitXRedis-MultiMode-Success", t, func() {
		mockey.Mock(xconfig.ContainKey).Return(true).Build()
		mockey.Mock(xutil.IsSlice).Return(true).Build()

		rdb1 := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
		rdb2 := redis.NewClient(&redis.Options{Addr: "localhost:6380"})
		defer rdb1.Close()
		defer rdb2.Close()

		callCount := 0
		mockey.Mock(newClient).To(func(c *Config) (*redis.Client, error) {
			callCount++
			if callCount == 1 {
				return rdb1, nil
			}
			return rdb2, nil
		}).Build()

		mockey.Mock(getMultiConfig).Return([]*Config{
			{Addr: "host1:6379", Name: "cache"},
			{Addr: "host2:6379", Name: "session"},
		}, nil).Build()

		err := initXRedis()
		c.So(err, c.ShouldBeNil)

		// 清理
		clientMu.Lock()
		clear(clientMap)
		clientMu.Unlock()
	})

	mockey.PatchConvey("TestInitXRedis-MultiMode-PartialFail-Rollback", t, func() {
		mockey.Mock(xconfig.ContainKey).Return(true).Build()
		mockey.Mock(xutil.IsSlice).Return(true).Build()

		rdb1 := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
		defer rdb1.Close()

		callCount := 0
		mockey.Mock(newClient).To(func(c *Config) (*redis.Client, error) {
			callCount++
			if callCount == 1 {
				return rdb1, nil
			}
			return nil, errors.New("connect failed")
		}).Build()

		mockey.Mock(getMultiConfig).Return([]*Config{
			{Addr: "host1:6379", Name: "cache"},
			{Addr: "host2:6379", Name: "session"},
		}, nil).Build()

		err := initXRedis()
		c.So(err, c.ShouldNotBeNil)
		c.So(err.Error(), c.ShouldContainSubstring, "newClient failed")
	})
}

func TestCloseXRedis(t *testing.T) {
	mockey.PatchConvey("TestCloseXRedis-Empty", t, func() {
		clientMu.Lock()
		clear(clientMap)
		clientMu.Unlock()

		err := closeXRedis()
		c.So(err, c.ShouldBeNil)
	})

	mockey.PatchConvey("TestCloseXRedis-WithClients", t, func() {
		rdb := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
		setDefault(rdb)
		set("cache", rdb) // 同一个 client 指向两个 key，测试去重

		err := closeXRedis()
		c.So(err, c.ShouldBeNil)
		c.So(len(clientMap), c.ShouldEqual, 0)
	})

	mockey.PatchConvey("TestCloseXRedis-CloseError", t, func() {
		// 先关闭 client，再放入 map，二次关闭会返回 "redis: client is closed"
		rdb := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
		_ = rdb.Close()
		setDefault(rdb)

		err := closeXRedis()
		c.So(err, c.ShouldNotBeNil)
		c.So(err.Error(), c.ShouldContainSubstring, "close redis client failed")
		c.So(len(clientMap), c.ShouldEqual, 0)
	})
}

func TestNewClient(t *testing.T) {
	mockey.PatchConvey("TestNewClient-PingFail", t, func() {
		mockey.Mock(xutil.Retry).Return(errors.New("ping timeout")).Build()

		client, err := newClient(&Config{Addr: "localhost:6379", DialTimeout: "1s"})
		c.So(err, c.ShouldNotBeNil)
		c.So(err.Error(), c.ShouldContainSubstring, "ping failed")
		c.So(client, c.ShouldBeNil)
	})

	mockey.PatchConvey("TestNewClient-PingSuccess-NoTrace", t, func() {
		mockey.Mock(xutil.Retry).Return(nil).Build()
		mockey.Mock(xtrace.EnableTrace).Return(false).Build()

		client, err := newClient(&Config{Addr: "localhost:6379", DialTimeout: "1s"})
		c.So(err, c.ShouldBeNil)
		c.So(client, c.ShouldNotBeNil)
		_ = client.Close()
	})

	mockey.PatchConvey("TestNewClient-PingSuccess-WithTrace", t, func() {
		mockey.Mock(xutil.Retry).Return(nil).Build()
		mockey.Mock(xtrace.EnableTrace).Return(true).Build()

		client, err := newClient(&Config{Addr: "localhost:6379", DialTimeout: "1s"})
		c.So(err, c.ShouldBeNil)
		c.So(client, c.ShouldNotBeNil)
		_ = client.Close()
	})

	mockey.PatchConvey("TestNewClient-InstrumentTracingFail", t, func() {
		mockey.Mock(xutil.Retry).Return(nil).Build()
		mockey.Mock(xtrace.EnableTrace).Return(true).Build()
		mockey.Mock(redisotel.InstrumentTracing).Return(errors.New("tracing error")).Build()

		client, err := newClient(&Config{Addr: "localhost:6379", DialTimeout: "1s"})
		c.So(err, c.ShouldNotBeNil)
		c.So(err.Error(), c.ShouldContainSubstring, "instrument tracing failed")
		c.So(client, c.ShouldBeNil)
	})

	mockey.PatchConvey("TestNewClient-PingLambdaExecuted", t, func() {
		// 让 Retry 实际调用 fn，覆盖 Ping lambda 内部路径
		mockey.Mock(xutil.Retry).To(func(fn func() error, attempts int, sleep time.Duration) error {
			return fn()
		}).Build()
		mockey.Mock((*redis.Client).Ping).To(func(client *redis.Client, ctx context.Context) *redis.StatusCmd {
			cmd := redis.NewStatusCmd(ctx, "PONG")
			return cmd
		}).Build()
		mockey.Mock(xtrace.EnableTrace).Return(false).Build()

		client, err := newClient(&Config{Addr: "localhost:6379", DialTimeout: "100ms"})
		c.So(err, c.ShouldBeNil)
		c.So(client, c.ShouldNotBeNil)
		_ = client.Close()
	})
}
