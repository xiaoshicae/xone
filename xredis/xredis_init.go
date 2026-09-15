package xredis

import (
	"context"
	"errors"
	"time"

	"github.com/redis/go-redis/extra/redisotel/v9"
	redis "github.com/redis/go-redis/v9"
	"github.com/xiaoshicae/xone/v3/xconfig"
	"github.com/xiaoshicae/xone/v3/xerror"
	"github.com/xiaoshicae/xone/v3/xhook"
	"github.com/xiaoshicae/xone/v3/xtrace"
	"github.com/xiaoshicae/xone/v3/xutil"
)

const (
	defaultClientName = "__default_client__"

	// pingAttempts 建连验证的尝试次数
	pingAttempts = 3
	// pingRetryInterval 建连验证的重试间隔
	pingRetryInterval = time.Second
	// defaultPingTimeout 无法从配置推算时单次 Ping 的兜底超时
	defaultPingTimeout = time.Second
)

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

	if config.metricEnabled() {
		registerPoolMetrics()
	}
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
			// 回滚：关闭已创建的连接，并从 clientMap 中摘除
			// 只关不摘的话，回滚到 BeforeStop 执行之间 C() 会返回已关闭的 client
			for _, c := range created {
				_ = c.Close()
			}
			removeClients(configs[:idx])
			return xerror.Newf("xredis", "init", "newClient failed, name=[%v], err=[%v]", config.Name, err)
		}

		created = append(created, client)
		set(config.Name, client)

		// 第一个 client 为 C() 默认获取的 client
		if idx == 0 {
			setDefault(client)
		}
	}

	// 指标是进程级的单个 collector，只要有任一 client 开启就注册
	for _, config := range configs {
		if config.metricEnabled() {
			registerPoolMetrics()
			break
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
	if err := pingWithRetry(client, c); err != nil {
		_ = client.Close()
		return nil, xerror.Newf("xredis", "newClient", "ping failed, addr=[%s], err=[%v]", c.Addr, err)
	}

	// OpenTelemetry 链路追踪集成
	if xtrace.TraceEnabled() {
		if err := redisotel.InstrumentTracing(client); err != nil {
			_ = client.Close()
			return nil, xerror.Newf("xredis", "newClient", "instrument tracing failed, err=[%v]", err)
		}
	}

	return client, nil
}

// pingWithRetry 建连验证，失败按固定间隔重试
//
// 用带 context 的重试：初始化跑在 BeforeStart 里，不可中断的重试会让
// 启动阶段收到的退出信号必须等满 attempts×interval 才生效
func pingWithRetry(client *redis.Client, c *Config) error {
	ctx, cancel := context.WithTimeout(context.Background(), pingTotalBudget(c))
	defer cancel()

	timeout := pingTimeout(c)
	return xutil.RetryWithContext(ctx, func(ctx context.Context) error {
		pingCtx, pingCancel := context.WithTimeout(ctx, timeout)
		defer pingCancel()
		return client.Ping(pingCtx).Err()
	}, pingAttempts, pingRetryInterval)
}

// pingTimeout 单次 Ping 的超时
//
// 不能只用 DialTimeout：Ping 的耗时是建连加一个往返，
// 拿建连预算当整体预算，会让连接刚建成就判超时
func pingTimeout(c *Config) time.Duration {
	d := xutil.ToDuration(c.DialTimeout) + xutil.ToDuration(c.ReadTimeout)
	if d <= 0 {
		return defaultPingTimeout
	}
	return d
}

// pingTotalBudget 建连验证的总预算，兜住整轮重试的最坏耗时
func pingTotalBudget(c *Config) time.Duration {
	return pingTimeout(c)*pingAttempts + pingRetryInterval*(pingAttempts-1)
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
