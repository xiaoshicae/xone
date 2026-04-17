package xgorm

const XGormConfigKey = "XGorm"

// Driver 数据库驱动类型
type Driver string

const (
	DriverPostgres Driver = "postgres"
	DriverMySQL    Driver = "mysql"
)

type Config struct {
	// Driver 数据库驱动类型
	// optional default "postgres"
	Driver string `mapstructure:"Driver"`

	// DSN 数据库连接的dsn
	// required
	DSN string `mapstructure:"DSN"`

	// DialTimeout 建连超时时间
	// 通用字段：MySQL 注入 DSN timeout 参数；PG 注入 DSN connect_timeout 参数（向上取整为秒）
	// optional default "500ms"
	DialTimeout string `mapstructure:"DialTimeout"`

	// MySQL 仅在 Driver="mysql" 时生效
	// optional
	MySQL MySQLOptions `mapstructure:"MySQL"`

	// Postgres 仅在 Driver="postgres" 时生效
	// optional
	Postgres PostgresOptions `mapstructure:"Postgres"`

	// MaxOpenConns 最大连接数
	// optional default 50
	MaxOpenConns int `mapstructure:"MaxOpenConns"`

	// MaxIdleConns 最大空闲连接数
	// optional default 等于 MaxOpenConns
	MaxIdleConns int `mapstructure:"MaxIdleConns"`

	// MaxLifetime 连接的最长存活时间
	// optional default "5m"
	MaxLifetime string `mapstructure:"MaxLifetime"`

	// MaxIdleTime 空闲连接的最长存活时间
	// optional default 等于 MaxLifetime
	MaxIdleTime string `mapstructure:"MaxIdleTime"`

	// SlowThreshold 慢查询日志阈值(如果开启日志，慢查询会记录到日志)
	// optional default "3s"
	SlowThreshold string `mapstructure:"SlowThreshold"`

	// IgnoreRecordNotFoundErrorLog 是否忽略未查询到结果的错误日志记录
	// optional default false
	IgnoreRecordNotFoundErrorLog bool `mapstructure:"IgnoreRecordNotFoundErrorLog"`

	// EnableLog 是否开启日志(开启后gorm的日志将记录到应用的log文件中)
	// optional default false
	EnableLog bool `mapstructure:"EnableLog"`

	// Name 用于区分多client配置时的唯一身份
	// optional default ""
	Name string `mapstructure:"Name"`
}

// MySQLOptions MySQL 驱动特化配置
type MySQLOptions struct {
	// ReadTimeout 读超时时间（对应 MySQL DSN 的 readTimeout）
	// optional default "3s"
	ReadTimeout string `mapstructure:"ReadTimeout"`

	// WriteTimeout 写超时时间（对应 MySQL DSN 的 writeTimeout）
	// optional default "5s"
	WriteTimeout string `mapstructure:"WriteTimeout"`
}

// PostgresOptions PostgreSQL 驱动特化配置
//
// 下列字段会在建连时注入 DSN，作为 startup message 发送到服务端变成会话级 GUC。
// 若用户已在 DSN 中显式写了同名 key（如 statement_timeout=xxx），不会被覆盖。
type PostgresOptions struct {
	// StatementTimeout 单条 SQL 最长执行时间（PG statement_timeout，毫秒）
	// optional default "" 表示不限制
	StatementTimeout string `mapstructure:"StatementTimeout"`

	// LockTimeout 等待锁的最长时间（PG lock_timeout，毫秒）
	// optional default "" 表示不限制
	LockTimeout string `mapstructure:"LockTimeout"`

	// IdleInTxTimeout 事务中空闲超时，超过即断连（PG idle_in_transaction_session_timeout，毫秒）
	// optional default "" 表示不限制
	IdleInTxTimeout string `mapstructure:"IdleInTxTimeout"`

	// Params 其它任意 PG runtime param（如 application_name、timezone、client_connection_check_interval 等）
	// 会原样拼入 DSN，DSN 已有同名 key 不会被覆盖
	// optional
	Params map[string]string `mapstructure:"Params"`
}

func configMergeDefault(c *Config) *Config {
	if c == nil {
		c = &Config{}
	}
	if c.Driver == "" {
		c.Driver = string(DriverPostgres)
	}
	if c.DialTimeout == "" {
		c.DialTimeout = "500ms"
	}

	// MySQL 读/写超时仅在 MySQL 驱动下补默认值，PG 不使用该字段
	if c.GetDriver() == DriverMySQL {
		if c.MySQL.ReadTimeout == "" {
			c.MySQL.ReadTimeout = "3s"
		}
		if c.MySQL.WriteTimeout == "" {
			c.MySQL.WriteTimeout = "5s"
		}
	}

	if c.MaxOpenConns <= 0 {
		c.MaxOpenConns = 50
	}
	if c.MaxIdleConns == 0 {
		c.MaxIdleConns = c.MaxOpenConns
	}
	if c.MaxLifetime == "" {
		c.MaxLifetime = "5m"
	}
	if c.MaxIdleTime == "" {
		c.MaxIdleTime = c.MaxLifetime
	}
	if c.SlowThreshold == "" {
		c.SlowThreshold = "3s"
	}
	return c
}

// GetDriver 获取驱动类型
func (c *Config) GetDriver() Driver {
	if c.Driver == "" {
		return DriverPostgres
	}
	return Driver(c.Driver)
}
