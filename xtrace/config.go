package xtrace

import "github.com/xiaoshicae/xone/v2/xutil"

const (
	XTraceConfigKey = "XTrace"
)

type Config struct {
	// Enable Trace是否开启
	// optional default true
	Enable *bool `mapstructure:"Enable"`

	// Console 内容是否需要在控制台打印
	// optional default false
	Console bool `mapstructure:"Console"`

	// ForwardHeaders 需要在链路中透传的自定义 HTTP Header 列表
	// 配置后会自动注册 HeaderPropagator，从上游请求 Extract 并向下游请求 Inject
	// optional default nil（不注册）
	ForwardHeaders []string `mapstructure:"ForwardHeaders"`
}

func configMergeDefault(c *Config) *Config {
	if c == nil {
		c = &Config{}
	}
	// Enable 使用指针类型，区分"未配置"和"配置为false"
	// 未配置时默认开启，只有明确配置 Enable: false 才关闭
	if c.Enable == nil {
		c.Enable = xutil.ToPtr(true)
	}
	return c
}
