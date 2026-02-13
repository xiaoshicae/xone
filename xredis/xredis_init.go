package xredis

import (
	"context"
	"errors"
	"time"

	"github.com/redis/go-redis/extra/redisotel/v9"
	redis "github.com/redis/go-redis/v9"
	"github.com/xiaoshicae/xone/v2/xconfig"
	"github.com/xiaoshicae/xone/v2/xerror"
	"github.com/xiaoshicae/xone/v2/xhook"
	"github.com/xiaoshicae/xone/v2/xtrace"
	"github.com/xiaoshicae/xone/v2/xutil"
)

const defaultClientName = "__default_client__"

func init() {
	xhook.BeforeStart(initXRedis)
	xhook.BeforeStop(closeXRedis)
}

func initXRedis() error {
	if !xconfig.ContainKey(XRedisConfigKey) {
		xutil.WarnIfEnableDebug("XOne init %s failed, config key [%s] not exists", XRedisConfigKey, XRedisConfigKey)
		return nil
	}

	if xutil.IsSlice(xconfig.GetConfig(XRedisConfigKey)) {
		return initMulti()
	}

	return initSingle()
}

func initSingle() error {
	config, err := getConfig()
	if err != nil {
		return xerror.Newf("xredis", "init", "getConfig failed, err=[%v]", err)
	}
	xutil.InfoIfEnableDebug("XOne init %s got config: %s", XRedisConfigKey, xutil.ToJsonString(sanitizeConfigForLog(config)))

	client, err := newClient(config)
	if err != nil {
		return xerror.Newf("xredis", "init", "newClient failed, err=[%v]", err)
	}

	setDefault(client)
	return nil
}

func initMulti() error {
	configs, err := getMultiConfig()
	if err != nil {
		return xerror.Newf("xredis", "init", "getMultiConfig failed, err=[%v]", err)
	}
	xutil.InfoIfEnableDebug("XOne init %s got config: %s", XRedisConfigKey, xutil.ToJsonString(sanitizeConfigsForLog(configs)))

	// 先创建所有 client，部分失败时回滚已创建的连接
	created := make([]*redis.Client, 0, len(configs))
	for idx, config := range configs {
		client, err := newClient(config)
		if err != nil {
			// 回滚已创建的连接
			for _, c := range created {
				_ = c.Close()
			}
			return xerror.Newf("xredis", "init", "newClient failed, name=[%v], err=[%v]", config.Name, err)
		}

		created = append(created, client)
		set(config.Name, client)

		// 第一个 client 为 C() 默认获取的 client
		if idx == 0 {
			setDefault(client)
		}
	}
	return nil
}

func closeXRedis() error {
	clientMu.Lock()
	defer clientMu.Unlock()

	// 用于去重，避免同一个 client 被关闭多次（multi 模式下 default 指向第一个 named client）
	closed := make(map[*redis.Client]struct{})
	var errs []error

	for _, client := range clientMap {
		if _, ok := closed[client]; ok {
			continue
		}
		closed[client] = struct{}{}

		if err := client.Close(); err != nil {
			errs = append(errs, xerror.Newf("xredis", "close", "close redis client failed, err=[%v]", err))
		}
	}
	clear(clientMap)
	return errors.Join(errs...)
}

func newClient(c *Config) (*redis.Client, error) {
	opts := &redis.Options{
		Addr:            c.Addr,
		Username:        c.Username,
		Password:        c.Password,
		DB:              c.DB,
		DialTimeout:     xutil.ToDuration(c.DialTimeout),
		ReadTimeout:     xutil.ToDuration(c.ReadTimeout),
		WriteTimeout:    xutil.ToDuration(c.WriteTimeout),
		PoolSize:        c.PoolSize,
		MinIdleConns:    c.MinIdleConns,
		MaxIdleConns:    c.MaxIdleConns,
		MaxActiveConns:  c.MaxActiveConns,
		PoolTimeout:     xutil.ToDuration(c.PoolTimeout),
		ConnMaxIdleTime: xutil.ToDuration(c.ConnMaxIdleTime),
		ConnMaxLifetime: xutil.ToDuration(c.ConnMaxLifetime),
		MaxRetries:      c.MaxRetries,
		MinRetryBackoff: xutil.ToDuration(c.MinRetryBackoff),
		MaxRetryBackoff: xutil.ToDuration(c.MaxRetryBackoff),
	}

	client := redis.NewClient(opts)

	// Ping 连接验证（带重试）
	pingTimeout := xutil.ToDuration(c.DialTimeout)
	err := xutil.Retry(func() error {
		ctx, cancel := context.WithTimeout(context.Background(), pingTimeout)
		defer cancel()
		return client.Ping(ctx).Err()
	}, 3, time.Second)
	if err != nil {
		_ = client.Close()
		return nil, xerror.Newf("xredis", "newClient", "ping failed, addr=[%s], err=[%v]", c.Addr, err)
	}

	// OpenTelemetry 链路追踪集成
	if xtrace.EnableTrace() {
		if err := redisotel.InstrumentTracing(client); err != nil {
			_ = client.Close()
			return nil, xerror.Newf("xredis", "newClient", "instrument tracing failed, err=[%v]", err)
		}
	}

	return client, nil
}

func getConfig() (*Config, error) {
	c := &Config{}
	if err := xconfig.UnmarshalConfig(XRedisConfigKey, c); err != nil {
		return nil, err
	}
	c = configMergeDefault(c)
	return c, nil
}

func getMultiConfig() ([]*Config, error) {
	var multiConfig []*Config
	if err := xconfig.UnmarshalConfig(XRedisConfigKey, &multiConfig); err != nil {
		return nil, err
	}
	seen := make(map[string]struct{}, len(multiConfig))
	for i, c := range multiConfig {
		multiConfig[i] = configMergeDefault(c)
		c = multiConfig[i]
		if c.Name == "" {
			return nil, xerror.Newf("xredis", "getMultiConfig", "multi config XRedis.Name can not be empty")
		}
		if c.Name == defaultClientName {
			return nil, xerror.Newf("xredis", "getMultiConfig", "multi config XRedis.Name can not be reserved name [%s]", defaultClientName)
		}
		if _, ok := seen[c.Name]; ok {
			return nil, xerror.Newf("xredis", "getMultiConfig", "multi config XRedis.Name [%s] is duplicated", c.Name)
		}
		seen[c.Name] = struct{}{}
	}
	return multiConfig, nil
}
