package xtrace

import "github.com/xiaoshicae/xone/v2/xutil"

const (
	XTraceConfigKey = "XTrace"

	// defaultSampleRatio 默认采样率，全采样
	defaultSampleRatio = 1.0

	// defaultShutdownTimeoutStr 默认关闭超时
	//
	// 取值小于 xhook 单个 Hook 的默认超时（10s），确保导出端不可达时
	// 是本模块自己按时返回，而不是被 xhook 放弃等待后留下泄漏的 goroutine。
	defaultShutdownTimeoutStr = "5s"
)

// defaultShutdownTimeout 默认关闭超时，供初始化前的兜底使用
var defaultShutdownTimeout = xutil.ToDuration(defaultShutdownTimeoutStr)

// ForwardHeaderRule 按域名透传的 Header 规则
// 仅当请求目标域名匹配 Domains 中的任一模式时，才透传对应 Headers
type ForwardHeaderRule struct {
	// Domains 域名模式列表，支持精确匹配和通配符前缀（如 *.example.com）
	Domains []string `mapstructure:"Domains"`

	// Headers 该规则下要透传的 Header 列表
	Headers []string `mapstructure:"Headers"`
}

type Config struct {
	// Enable Trace 是否开启
	// optional default true
	Enable *bool `mapstructure:"Enable"`

	// EnableConsole Trace 内容是否打印到标准输出，仅用于本地调试
	// 关闭时不注册任何 SpanProcessor，Span 创建后直接丢弃，仅保留 TraceID/SpanID 与 Header 透传能力
	// optional default false
	EnableConsole bool `mapstructure:"EnableConsole"`

	// SampleRatio 采样率，取值 (0, 1]，>= 1 时全采样
	// 需要完全关闭链路请用 Enable=false，本项不接受 0（0 视为未配置，回落到默认值）
	// optional default 1.0
	SampleRatio float64 `mapstructure:"SampleRatio"`

	// ShutdownTimeout 关闭时等待 Span 导出完成的上限
	// 使用者通过 AddSpanProcessor 注册了上报处理器时，该值决定退出前最多等多久
	// optional default "5s"
	ShutdownTimeout string `mapstructure:"ShutdownTimeout"`

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

// forwardEnabled 是否配置了 Header 透传
func (c *Config) forwardEnabled() bool {
	return len(c.ForwardHeaders) > 0 || len(c.ForwardHeaderRules) > 0
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
	// 采样率非正数视为未配置：关闭链路请用 Enable=false
	if c.SampleRatio <= 0 {
		c.SampleRatio = defaultSampleRatio
	}
	if xutil.ToDuration(c.ShutdownTimeout) <= 0 {
		if c.ShutdownTimeout != "" {
			xutil.WarnIfEnableDebug("XOne xtrace ShutdownTimeout is invalid, fallback to %s, got=[%s]", defaultShutdownTimeoutStr, c.ShutdownTimeout)
		}
		c.ShutdownTimeout = defaultShutdownTimeoutStr
	}
	return c
}
