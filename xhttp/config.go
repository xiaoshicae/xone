package xhttp

const XHttpConfigKey = "XHttp"

type Config struct {
	// Timeout HTTP 请求超时时间
	// optional default "60s"
	Timeout string `mapstructure:"Timeout"`

	// MaxIdleConns 最大空闲连接数
	// optional default 100
	MaxIdleConns int `mapstructure:"MaxIdleConns"`

	// MaxIdleConnsPerHost 每个 host 最大空闲连接数
	// optional default 10
	MaxIdleConnsPerHost int `mapstructure:"MaxIdleConnsPerHost"`

	// IdleConnTimeout 空闲连接超时时间
	// optional default "90s"
	IdleConnTimeout string `mapstructure:"IdleConnTimeout"`
}

func configMergeDefault(c *Config) *Config {
	if c == nil {
		c = &Config{}
	}
	if c.Timeout == "" {
		c.Timeout = "60s"
	}
	if c.MaxIdleConns <= 0 {
		c.MaxIdleConns = 100
	}
	if c.MaxIdleConnsPerHost <= 0 {
		c.MaxIdleConnsPerHost = 10
	}
	if c.IdleConnTimeout == "" {
		c.IdleConnTimeout = "90s"
	}
	return c
}
