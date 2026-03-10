package xtrace

import "github.com/xiaoshicae/xone/v2/xutil"

const (
	XTraceConfigKey = "XTrace"
)

// ForwardHeaderRule 按域名透传的 Header 规则
// 仅当请求目标域名匹配 Domains 中的任一模式时，才透传对应 Headers
type ForwardHeaderRule struct {
	// Domains 域名模式列表，支持精确匹配和通配符前缀（如 *.example.com）
	Domains []string `mapstructure:"Domains"`

	// Headers 该规则下要透传的 Header 列表
	Headers []string `mapstructure:"Headers"`
}

type Config struct {
	// Enable Trace是否开启
	// optional default true
	Enable *bool `mapstructure:"Enable"`

	// Console 内容是否需要在控制台打印
	// optional default false
	Console bool `mapstructure:"Console"`

	// ForwardHeaders 需要在链路中透传的自定义 HTTP Header 列表（全局，向所有域名透传）
	// 配置后会自动注册 HeaderPropagator，从上游请求 Extract 并向下游请求 Inject
	// optional default nil（不注册）
	ForwardHeaders []string `mapstructure:"ForwardHeaders"`

	// ForwardHeaderRules 按域名透传的 Header 规则列表
	// 仅当请求目标域名匹配规则中的 Domains 时才透传对应 Headers
	// 域名支持精确匹配（如 api.example.com）和通配符前缀（如 *.example.com）
	// optional default nil
	ForwardHeaderRules []ForwardHeaderRule `mapstructure:"ForwardHeaderRules"`
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
