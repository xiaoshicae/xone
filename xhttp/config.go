package xhttp

const XHttpConfigKey = "XHttp"

type Config struct {

	// Timeout http请求超时时间
	// optional default "60s"
	Timeout string `mapstructure:"Timeout"`
}

func configMergeDefault(c *Config) *Config {
	if c == nil {
		c = &Config{}
	}
	if c.Timeout == "" {
		c.Timeout = "60s"
	}
	return c
}
