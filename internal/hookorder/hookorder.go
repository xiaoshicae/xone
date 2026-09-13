// Package hookorder 定义 xone 框架保留的 Hook 层级。
//
// 本包位于 internal 下，外部模块无法导入，因而无法构造 Token，
// 也就无法把自己的 Hook 注册进框架保留区（Order < 0）。
// 业务 Hook 只能通过 xhook.Order 注册，其取值被限定为非负数。
package hookorder

const (
	// Config 配置模块层级：必须最先启动
	//
	// 其他模块的 BeforeStart 都要读配置，用户自己的包也可能在 init 中注册 Hook，
	// 而 Go 的 init 顺序按 import path 字典序排，模块名排在 xone 之前的用户包
	// 会先注册。没有这一层，它们会在配置加载完成前执行。
	Config = -100

	// Log 日志模块层级：次先启动，最后关闭
	//
	// 启动时先于一切业务模块就绪，关闭时晚于一切业务模块，
	// 使任何模块在启停两端打的日志都能落盘。
	Log = -50
)

// Token 内部调用凭证，零值即有效。
//
// 它的唯一作用是让 xhook.ReservedOrder 只能被 xone 模块内部调用：
// 外部模块引用不到本包中的类型，也就无法提供这个参数。
type Token struct{}
