package options

func EnableLogMiddleware(enableLogMiddleware bool) Option {
	return func(o *Options) {
		o.EnableLogMiddleware = enableLogMiddleware
	}
}

// LogSkipPaths 设置日志中间件忽略的路由
// 支持精确匹配和前缀匹配，例如：
//   - "/health" 精确匹配 /health
//   - "/health/" 前缀匹配 /health/live, /health/ready 等
func LogSkipPaths(paths ...string) Option {
	return func(o *Options) {
		o.LogSkipPaths = append(o.LogSkipPaths, paths...)
	}
}

func EnableTraceMiddleware(enableTraceMiddleware bool) Option {
	return func(o *Options) {
		o.EnableTraceMiddleware = enableTraceMiddleware
	}
}

func EnableZHTranslations(enableZHTranslations bool) Option {
	return func(o *Options) {
		o.EnableZHTranslations = enableZHTranslations
	}
}

func EnableMetricMiddleware(enableMetricMiddleware bool) Option {
	return func(o *Options) {
		o.EnableMetricMiddleware = enableMetricMiddleware
	}
}

// MetricsPath 设置 Prometheus /metrics 端点路径，默认 "/metrics"
// 启用 MetricMiddleware 时自动注册该路由，无需手动配置
func MetricsPath(path string) Option {
	return func(o *Options) {
		o.MetricsPath = path
	}
}

type Option func(*Options)

type Options struct {
	EnableLogMiddleware    bool
	EnableTraceMiddleware  bool
	EnableZHTranslations   bool
	EnableMetricMiddleware bool
	LogSkipPaths           []string // 日志中间件忽略的路由列表
	MetricsPath            string   // Prometheus metrics 端点路径，默认 "/metrics"
}

func DefaultOptions() *Options {
	return &Options{
		EnableLogMiddleware:    true,
		EnableTraceMiddleware:  true,
		EnableMetricMiddleware: true,
		EnableZHTranslations:   false,
		LogSkipPaths:           make([]string, 0),
		MetricsPath:            "/metrics",
	}
}
