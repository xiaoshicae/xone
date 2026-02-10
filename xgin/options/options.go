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

func Addr(addr string) Option {
	return func(o *Options) {
		o.Addr = addr
	}
}

type Option func(*Options)

type Options struct {
	EnableLogMiddleware   bool
	EnableTraceMiddleware bool
	EnableZHTranslations  bool
	Addr                  string
	LogSkipPaths          []string // 日志中间件忽略的路由列表
}

func DefaultOptions() *Options {
	return &Options{
		EnableLogMiddleware:   true,
		EnableTraceMiddleware: true,
		EnableZHTranslations:  false,
		LogSkipPaths:          make([]string, 0),
		Addr:                  "0.0.0.0:8080",
	}
}
