package xredis

const XRedisConfigKey = "XRedis"

type Config struct {
	// Addr Redis 服务器地址
	// required default "localhost:6379"
	Addr string `mapstructure:"Addr"`

	// Password Redis 认证密码
	// optional default ""
	Password string `mapstructure:"Password"`

	// DB Redis 数据库编号
	// optional default 0
	DB int `mapstructure:"DB"`

	// Username Redis 6.0+ ACL 用户名
	// optional default ""
	Username string `mapstructure:"Username"`

	// DialTimeout 建立连接超时时间
	// optional default "500ms"
	DialTimeout string `mapstructure:"DialTimeout"`

	// ReadTimeout 读超时时间
	// optional default "500ms"
	ReadTimeout string `mapstructure:"ReadTimeout"`

	// WriteTimeout 写超时时间
	// optional default "500ms"
	WriteTimeout string `mapstructure:"WriteTimeout"`

	// PoolSize 连接池最大连接数
	// optional default 0（go-redis 默认 10 * runtime.GOMAXPROCS）
	PoolSize int `mapstructure:"PoolSize"`

	// MinIdleConns 最小空闲连接数，保持热连接避免冷启动延迟
	// optional default 5
	MinIdleConns int `mapstructure:"MinIdleConns"`

	// MaxIdleConns 最大空闲连接数
	// optional default 0（go-redis 默认无限制）
	MaxIdleConns int `mapstructure:"MaxIdleConns"`

	// MaxActiveConns 最大活跃连接数
	// optional default 0（go-redis 默认无限制）
	MaxActiveConns int `mapstructure:"MaxActiveConns"`

	// PoolTimeout 从连接池获取连接的超时时间，无空闲连接时会触发等待
	// optional default "1s"
	PoolTimeout string `mapstructure:"PoolTimeout"`

	// ConnMaxIdleTime 空闲连接最大存活时间
	// optional default "5m"
	ConnMaxIdleTime string `mapstructure:"ConnMaxIdleTime"`

	// ConnMaxLifetime 连接最大存活时间，定期刷新连接有利于负载均衡重新分配
	// optional default "5m"
	ConnMaxLifetime string `mapstructure:"ConnMaxLifetime"`

	// MaxRetries 最大重试次数
	// optional default 0（go-redis 默认 3 次，设置 -1 禁用重试）
	MaxRetries int `mapstructure:"MaxRetries"`

	// MinRetryBackoff 最小重试退避时间
	// optional default ""（go-redis 默认 8ms，设置 "-1" 禁用退避）
	MinRetryBackoff string `mapstructure:"MinRetryBackoff"`

	// MaxRetryBackoff 最大重试退避时间
	// optional default ""（go-redis 默认 512ms，设置 "-1" 禁用退避）
	MaxRetryBackoff string `mapstructure:"MaxRetryBackoff"`

	// Name 用于区分多 client 配置时的唯一身份
	// optional default ""
	Name string `mapstructure:"Name"`
}

func configMergeDefault(c *Config) *Config {
	if c == nil {
		c = &Config{}
	}
	if c.Addr == "" {
		c.Addr = "localhost:6379"
	}
	if c.DialTimeout == "" {
		c.DialTimeout = "500ms"
	}
	if c.ReadTimeout == "" {
		c.ReadTimeout = "500ms"
	}
	if c.WriteTimeout == "" {
		c.WriteTimeout = "500ms"
	}
	// PoolSize 不设默认值，0 值由 go-redis 处理为 10 * runtime.GOMAXPROCS
	if c.MinIdleConns <= 0 {
		c.MinIdleConns = 5
	}
	if c.PoolTimeout == "" {
		c.PoolTimeout = "1s"
	}
	if c.ConnMaxIdleTime == "" {
		c.ConnMaxIdleTime = "5m"
	}
	if c.ConnMaxLifetime == "" {
		c.ConnMaxLifetime = "5m"
	}
	// MaxRetries/MinRetryBackoff/MaxRetryBackoff 不设默认值
	// go-redis 内部处理：0 → 使用默认值（3次/8ms/512ms），-1 → 禁用
	return c
}

// sanitizeConfigForLog 创建配置的脱敏副本用于日志输出（隐藏密码）
func sanitizeConfigForLog(c *Config) *Config {
	sc := *c
	if sc.Password != "" {
		sc.Password = "***"
	}
	return &sc
}

// sanitizeConfigsForLog 创建多个配置的脱敏副本用于日志输出
func sanitizeConfigsForLog(configs []*Config) []*Config {
	result := make([]*Config, len(configs))
	for i, c := range configs {
		result[i] = sanitizeConfigForLog(c)
	}
	return result
}
