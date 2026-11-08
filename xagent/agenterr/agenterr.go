// Package agenterr 定义 xagent 各层共用的错误原因（哨兵错误），调用方用 errors.Is 判断。
//
// 它不另造错误类型：xagent 对外返回的错误仍然是 *xerror.Error（模块名 xagent），
// 这里的哨兵用 %w 包在里面，errors.Is 能穿过 xerror 找到它：
//
//	res, err := agent.Run(ctx, msgs)
//	switch {
//	case errors.Is(err, agenterr.ErrMaxSteps):       // 调大 WithMaxSteps，或者看看工具结果是不是让模型陷入了重试
//	case errors.Is(err, agenterr.ErrRateLimited):    // 退避后重试
//	case errors.Is(err, agenterr.ErrContextTooLong): // 压缩历史后重试
//	case errors.Is(err, context.Canceled):           // 取消和超时直接用 context 的错误，这里不重复定义
//	}
//
// 这个包在最底层，只依赖标准库：llm 的适配层、工具、循环都可以用它。
package agenterr

import "errors"

// 循环自己产生的错误。
var (
	// ErrMaxSteps 调用模型的次数达到上限，模型还在调用工具。
	ErrMaxSteps = errors.New("max steps exceeded")

	// ErrTruncated 模型的输出被长度上限截断。被截断的回复不进对话历史。
	ErrTruncated = errors.New("output truncated by max tokens")
)

// 模型调用的错误：由 llm.Model 的实现（各家 provider 的适配层）用 %w 包装后返回，
// 循环原样透传给调用方。适配层应当把 provider 的错误尽量归到这几类里，归不进去的保持原样。
var (
	// ErrRateLimited 被 provider 限流（如 HTTP 429）。可以退避后重试。
	ErrRateLimited = errors.New("rate limited")

	// ErrUnavailable provider 暂时不可用：过载、5xx、网络中断。可以重试。
	ErrUnavailable = errors.New("model unavailable")

	// ErrContextTooLong 对话超出了模型的上下文窗口。重试不会好转，要先缩短历史。
	ErrContextTooLong = errors.New("context too long")

	// ErrAuth 鉴权失败：API key 无效、过期或没有权限。重试不会好转。
	ErrAuth = errors.New("authentication failed")
)
