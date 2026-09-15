package xhttp

import "github.com/xiaoshicae/xone/v3/xutil"

const XHttpConfigKey = "XHttp"

type Config struct {
	// Timeout HTTP 请求超时时间
	// optional default "60s"
	Timeout string `mapstructure:"Timeout"`

	// DialTimeout 建立 TCP 连接超时时间
	// optional default "30s"
	DialTimeout string `mapstructure:"DialTimeout"`

	// DialKeepAlive TCP keep-alive 探测间隔
	// optional default "30s"
	DialKeepAlive string `mapstructure:"DialKeepAlive"`

	// MaxIdleConns 最大空闲连接数
	// optional default 100
	MaxIdleConns int `mapstructure:"MaxIdleConns"`

	// MaxIdleConnsPerHost 每个 host 最大空闲连接数
	// optional default 10
	MaxIdleConnsPerHost int `mapstructure:"MaxIdleConnsPerHost"`

	// IdleConnTimeout 空闲连接超时时间
	// optional default "90s"
	IdleConnTimeout string `mapstructure:"IdleConnTimeout"`

	// RetryCount 重试次数
	// optional default 0 (不重试)
	RetryCount int `mapstructure:"RetryCount"`

	// RetryWaitTime 重试等待时间
	// optional default "100ms"
	RetryWaitTime string `mapstructure:"RetryWaitTime"`

	// RetryMaxWaitTime 最大重试等待时间
	// optional default "2s"
	RetryMaxWaitTime string `mapstructure:"RetryMaxWaitTime"`

	// RetryOnlyIdempotent 是否只对幂等方法（GET/HEAD/OPTIONS/TRACE/PUT/DELETE）重试
	//
	// 默认开启。传输层超时无法区分「请求没到服务端」和「服务端处理完了但响应丢了」，
	// 重发一个 POST 就可能变成重复下单、重复扣款。确认接口幂等（如带幂等键）
	// 再关掉它
	// optional default true
	RetryOnlyIdempotent *bool `mapstructure:"RetryOnlyIdempotent"`

	// EnableMetric 是否启用出站请求 Prometheus 指标采集
	// optional default true
	EnableMetric *bool `mapstructure:"EnableMetric"`
}

// retryOnlyIdempotentEnabled 返回是否只对幂等方法重试
// nil 视为启用，与文档中的默认值一致，避免直接构造 Config 的调用方踩空指针
func (c *Config) retryOnlyIdempotentEnabled() bool {
	return c.RetryOnlyIdempotent == nil || *c.RetryOnlyIdempotent
}

func configMergeDefault(c *Config) *Config {
	if c == nil {
		c = &Config{}
	}
	if c.Timeout == "" {
		c.Timeout = "60s"
	}
	if c.DialTimeout == "" {
		c.DialTimeout = "30s"
	}
	if c.DialKeepAlive == "" {
		c.DialKeepAlive = "30s"
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
	// RetryCount 默认 0，不需要特殊处理
	if c.RetryWaitTime == "" {
		c.RetryWaitTime = "100ms"
	}
	if c.RetryMaxWaitTime == "" {
		c.RetryMaxWaitTime = "2s"
	}
	if c.EnableMetric == nil {
		c.EnableMetric = xutil.ToPtr(true)
	}
	if c.RetryOnlyIdempotent == nil {
		c.RetryOnlyIdempotent = xutil.ToPtr(true)
	}
	return c
}
