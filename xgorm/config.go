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
	// optional default "500ms"
	DialTimeout string `mapstructure:"DialTimeout"`

	// ReadTimeout 读超时时间 (仅 mysql 有效)
	// optional default "3s"
	ReadTimeout string `mapstructure:"ReadTimeout"`

	// WriteTimeout 写超时时间 (仅 mysql 有效)
	// optional default "5s"
	WriteTimeout string `mapstructure:"WriteTimeout"`

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
	// optional default true
	IgnoreRecordNotFoundErrorLog bool `mapstructure:"IgnoreRecordNotFoundErrorLog"`

	// EnableLog 是否开启日志(开启后gorm的日志将记录到应用的log文件中)
	// optional default false
	EnableLog bool `mapstructure:"EnableLog"`

	// Name 用于区分多client配置时的唯一身份
	// optional default ""
	Name string `mapstructure:"Name"`
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
	if c.ReadTimeout == "" {
		c.ReadTimeout = "3s"
	}
	if c.WriteTimeout == "" {
		c.WriteTimeout = "5s"
	}
	if c.MaxOpenConns == 0 {
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
