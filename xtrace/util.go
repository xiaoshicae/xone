package xtrace

import "sync/atomic"

// traceEnabled 链路是否开启，由 initXTrace 在 BeforeStart 阶段写入
//
// 不在此处直接读 xconfig：配置值在初始化时已确定，运行时不会变化，
// 散落读取会绕过 configMergeDefault 的默认值逻辑。
// 未初始化时返回 true，与"未配置 XTrace.Enable 即默认开启"一致。
var traceEnabled = func() *atomic.Bool {
	v := &atomic.Bool{}
	v.Store(true)
	return v
}()

// TraceEnabled 报告链路追踪是否开启
//
// 是查询不是开关。它一度叫 EnableTrace，读起来像"把链路打开"，而 Go 允许
// 丢弃返回值，于是 xtrace.EnableTrace() 单写一行能编译、能过 vet、什么也不做。
// 开关在配置里：XTrace.Enable。
//
// 调用方需在 xtrace 的 BeforeStart Hook 执行之后使用。框架内各模块
// （xhttp / xgorm / xredis）都 import xtrace，Go 保证 xtrace 的 init 先执行，
// 其 Hook 也因此先注册、先运行，读到的一定是初始化后的值。
func TraceEnabled() bool {
	return traceEnabled.Load()
}

// forwardHeaderEnabled 是否配置了 Header 透传，由 initXTrace 在 BeforeStart 阶段写入
var forwardHeaderEnabled atomic.Bool

// ForwardHeaderEnabled 报告是否配置了自定义 Header 透传
//
// 同 TraceEnabled，是查询不是开关。
// 链路关闭时 Header 透传仍然生效，调用方据此决定是否需要包装 Transport。
// 需在 xtrace 的 BeforeStart Hook 执行之后使用。
func ForwardHeaderEnabled() bool {
	return forwardHeaderEnabled.Load()
}
