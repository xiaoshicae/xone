package xagent

// DefaultMaxSteps 默认最多调用模型的次数。防止模型反复调用工具停不下来。
const DefaultMaxSteps = 10

// Option 配置 Agent。
type Option func(*Agent)

// WithSystem 设置系统提示词。
func WithSystem(prompt string) Option {
	return func(a *Agent) { a.system = prompt }
}

// WithMaxSteps 设置最多调用模型的次数，默认 DefaultMaxSteps。
func WithMaxSteps(n int) Option {
	return func(a *Agent) { a.maxSteps = n }
}
