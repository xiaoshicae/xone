package xhook

import (
	"fmt"
	"time"

	"github.com/xiaoshicae/xone/v2/internal/hookorder"
)

const (
	defaultHookTimeout = 10 * time.Second

	// minOrder 业务 Hook 允许的最小 Order
	//
	// 负值区为框架保留（xconfig / xlog），业务 Hook 无法进入，
	// 因此不可能先于配置启动，也不可能晚于日志关闭。
	minOrder = 0
)

// Order 设置 Hook 所处的资源层级，数值越小越底层：
// BeforeStart 越先执行，BeforeStop 越后执行（启停对称）。
// 相同 Order 的 Hook：BeforeStart 按注册顺序，BeforeStop 按注册逆序。
//
// 取值必须 >= 0，负值为框架保留区，传入负值直接 panic。
// 普通模块应保持默认值（100），详见 README。
func Order(order int) Option {
	if order < minOrder {
		panic(fmt.Sprintf("XOne hook order can not be less than %d, negative order is reserved for the framework", minOrder))
	}
	return func(o *options) {
		o.Order = order
	}
}

// ReservedOrder 设置框架保留层级，取值见 internal/hookorder。
//
// 参数类型位于 internal 包，外部模块无法构造，因此本函数仅 xone 内部可用。
func ReservedOrder(_ hookorder.Token, order int) Option {
	return func(o *options) {
		o.Order = order
	}
}

// MustSucceed 设置 Hook 执行失败时是否终止流程，默认 true
//
//	xhook.BeforeStart(initFoo, xhook.MustSucceed(false)) // foo 起不来也继续启动
func MustSucceed(must bool) Option {
	return func(o *options) {
		o.MustSucceed = must
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
	Order       int
	MustSucceed bool
	Timeout     time.Duration // 单个 Hook 超时时间
}

func defaultOptions() *options {
	return &options{
		Order:       100,
		MustSucceed: true,
		Timeout:     defaultHookTimeout,
	}
}
