package xhook

import "time"

const defaultHookTimeout = 10 * time.Second

// Order 设置 Hook 执行顺序，数值越小越先执行
func Order(order int) Option {
	return func(o *options) {
		o.Order = order
	}
}

// MustInvokeSuccess 设置 Hook 执行失败是否终止流程
func MustInvokeSuccess(success bool) Option {
	return func(o *options) {
		o.MustInvokeSuccess = success
	}
}

// Timeout 设置单个 Hook 的超时时间，默认 10s
func Timeout(d time.Duration) Option {
	return func(o *options) {
		if d > 0 {
			o.Timeout = d
		}
	}
}

// Option Hook 配置选项函数类型
type Option func(*options)

type options struct {
	Order             int
	MustInvokeSuccess bool
	Timeout           time.Duration // 单个 Hook 超时时间
}

func defaultOptions() *options {
	return &options{
		Order:             100,
		MustInvokeSuccess: true,
		Timeout:           defaultHookTimeout,
	}
}
