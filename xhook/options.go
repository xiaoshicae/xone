package xhook

import "time"

const defaultHookTimeout = 10 * time.Second

// Order 设置 Hook 所处的资源层级，数值越小越底层：
// BeforeStart 越先执行，BeforeStop 越后执行（启停对称）。
// 相同 Order 的 Hook：BeforeStart 按注册顺序，BeforeStop 按注册逆序。
//
// 普通模块应保持默认值，依靠 import 顺序控制执行顺序，详见 README。
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
