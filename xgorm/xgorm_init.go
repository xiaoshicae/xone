package xgorm

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/xiaoshicae/xone/xconfig"
	"github.com/xiaoshicae/xone/xhook"
	"github.com/xiaoshicae/xone/xtrace"
	"github.com/xiaoshicae/xone/xutil"

	stdMysql "github.com/go-sql-driver/mysql"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/plugin/opentelemetry/tracing"
)

const defaultClientName = "__default_client__"

var (
	clientMap = make(map[string]*gorm.DB)
	clientMu  sync.RWMutex
)

func init() {
	xhook.BeforeStart(initXGorm)
	xhook.BeforeStop(closeXGorm)
}

func initXGorm() error {
	if !xconfig.ContainKey(XGormConfigKey) {
		xutil.WarnIfEnableDebug("XOne init %s failed, config key [%s] not exists", XGormConfigKey, XGormConfigKey)
		return nil
	}

	if xutil.IsSlice(xconfig.GetConfig(XGormConfigKey)) {
		return initMulti()
	}

	return initSingle()
}

func initSingle() error {
	config, err := getConfig()
	if err != nil {
		return fmt.Errorf("XOne init %s getConfig failed, err=[%v]", XGormConfigKey, err)
	}
	xutil.InfoIfEnableDebug("XOne init %s got config: %s", XGormConfigKey, xutil.ToJsonString(config))

	client, err := newClient(config)
	if err != nil {
		return fmt.Errorf("XOne init %s newClient failed, err=[%v]", XGormConfigKey, err)
	}

	setDefault(client)
	return nil
}

func initMulti() error {
	configs, err := getMultiConfig()
	if err != nil {
		return fmt.Errorf("XOne init %s getMultiConfig failed, err=[%v]", XGormConfigKey, err)
	}
	xutil.InfoIfEnableDebug("XOne init %s got config: %s", XGormConfigKey, xutil.ToJsonString(configs))

	for idx, config := range configs {
		client, err := newClient(config)
		if err != nil {
			return fmt.Errorf("XOne init %s newClient failed, name: %v, err=[%v]", XGormConfigKey, config.Name, err)
		}

		set(config.Name, client)

		// 第一个client为C()默认获取的client
		if idx == 0 {
			setDefault(client)
		}
	}
	return nil
}

func closeXGorm() error {
	clientMu.Lock()
	defer clientMu.Unlock()

	// 用于去重，避免同一个 *gorm.DB 被关闭多次（multi模式下default指向第一个named client）
	closed := make(map[*gorm.DB]struct{})
	var lastErr error

	for _, client := range clientMap {
		if _, ok := closed[client]; ok {
			continue
		}
		closed[client] = struct{}{}

		db, err := client.DB()
		if err != nil {
			lastErr = err
			continue
		}
		if err := db.Close(); err != nil {
			lastErr = err
		}
	}
	return lastErr
}

func get(name ...string) *gorm.DB {
	n := defaultClientName
	if len(name) > 0 {
		n = name[0]
	}

	clientMu.RLock()
	defer clientMu.RUnlock()
	return clientMap[n]
}

func set(name string, client *gorm.DB) {
	clientMu.Lock()
	defer clientMu.Unlock()
	clientMap[name] = client
}

func setDefault(client *gorm.DB) {
	clientMu.Lock()
	defer clientMu.Unlock()
	clientMap[defaultClientName] = client
}

func newClient(c *Config) (*gorm.DB, error) {
	dialector, err := resolveDialector(c)
	if err != nil {
		return nil, fmt.Errorf("newClient invoke resolveDialector failed, err=[%v]", err)
	}

	gormConfig := &gorm.Config{}
	if c.EnableLog {
		gormConfig.Logger = newGormLogger(c)
	}
	client, err := gorm.Open(dialector, gormConfig)
	if err != nil {
		return nil, fmt.Errorf("newClient invoke gorm.Open failed, err=[%v]", err)
	}

	db, err := client.DB()
	if err != nil {
		return nil, fmt.Errorf("newClient invoke client.DB failed, err=[%v]", err)
	}

	// 连接池参数配置
	db.SetMaxOpenConns(c.MaxOpenConns)
	db.SetMaxIdleConns(c.MaxIdleConns)
	db.SetConnMaxLifetime(xutil.ToDuration(c.MaxLifetime))
	db.SetConnMaxIdleTime(xutil.ToDuration(c.MaxIdleTime))

	err = xutil.Retry(func() error { return db.PingContext(context.Background()) }, 3, time.Second)
	if err != nil {
		return nil, fmt.Errorf("newClient invoke db.PingContext failed, err=[%v]", err)
	}

	if xtrace.EnableTrace() {
		if err := client.Use(tracing.NewPlugin(tracing.WithoutMetrics())); err != nil {
			return nil, fmt.Errorf("newClient use tracing.NewPlugin failed, err=[%v]", err)
		}
	}

	return client, nil
}

// resolveDialector 根据 driver 类型返回对应的 gorm dialector
func resolveDialector(c *Config) (gorm.Dialector, error) {
	if c == nil {
		return nil, fmt.Errorf("config can't be empty")
	}

	if c.DSN == "" {
		return nil, fmt.Errorf("dsn can't be empty")
	}

	switch c.GetDriver() {
	case DriverMySQL:
		resolvedDSN, err := resolveMySQLDSN(c)
		if err != nil {
			return nil, fmt.Errorf("resolve mysql dsn failed, err=[%v]", err)
		}
		xutil.InfoIfEnableDebug("XOne initXGorm newClient resolve MySQL DSN: %s", resolvedDSN)
		return mysql.Open(resolvedDSN), nil

	case DriverPostgres:
		xutil.InfoIfEnableDebug("XOne initXGorm newClient use Postgres DSN: %s", c.DSN)
		return postgres.Open(c.DSN), nil

	default:
		return nil, fmt.Errorf("unsupported driver: %s, supported: mysql, postgres", c.GetDriver())
	}
}

// resolveMySQLDSN 根据config构建MySQL DSN
// DSN协议: [username[:password]@][protocol[(address)]]/dbname[?param1=value1&param2=value2&...]
func resolveMySQLDSN(c *Config) (string, error) {
	mysqlConfig, err := stdMysql.ParseDSN(c.DSN)
	if err != nil {
		return "", err
	}

	if mysqlConfig.ReadTimeout == 0 && c.ReadTimeout != "" {
		mysqlConfig.ReadTimeout = xutil.ToDuration(c.ReadTimeout)
	}

	if mysqlConfig.WriteTimeout == 0 && c.WriteTimeout != "" {
		mysqlConfig.WriteTimeout = xutil.ToDuration(c.WriteTimeout)
	}

	if mysqlConfig.Timeout == 0 && c.DialTimeout != "" {
		mysqlConfig.Timeout = xutil.ToDuration(c.DialTimeout)
	}

	return mysqlConfig.FormatDSN(), nil
}

func getConfig() (*Config, error) {
	c := &Config{}
	if err := xconfig.UnmarshalConfig(XGormConfigKey, c); err != nil {
		return nil, err
	}
	c = configMergeDefault(c)
	if c.DSN == "" {
		return nil, fmt.Errorf("config XGorm.DSN can not be empty")
	}
	return c, nil
}

func getMultiConfig() ([]*Config, error) {
	var multiConfig []*Config
	if err := xconfig.UnmarshalConfig(XGormConfigKey, &multiConfig); err != nil {
		return nil, err
	}
	for _, c := range multiConfig {
		c = configMergeDefault(c)
		if c.DSN == "" {
			return nil, fmt.Errorf("multi config XGorm.DSN can not be empty")
		}
		if c.Name == "" {
			return nil, fmt.Errorf("multi config XGorm.Name can not be empty")
		}
	}
	return multiConfig, nil
}
