package llm

import (
	"context"
)

// Model 调用一次大模型：给一段对话，拿回模型的回复。
//
// 实现应当是无状态的：对话历史全部在 req.Messages 里，由调用方维护。
// 这样同一个 Model 可以被多个 agent、多个并发请求共用。
//
// 出错时应当用 %w 包装 agenterr 里对应的原因（限流、上下文超长……），调用方才能分类处理。
type Model interface {
	Generate(ctx context.Context, req *Request) (*Response, error)
}

// Request 一次调用的输入。
type Request struct {
	System   string    // 系统提示词
	Messages []Message // 对话历史，按时间顺序
	Tools    []ToolDef // 本次允许模型调用的工具；为空时模型只能回复文本
}

// Response 一次调用的输出。
//
// Message 是模型的回复，会原样进入对话历史，所以它带着 StopReason 和 Usage。
// 只属于这一次调用、不必进入历史的信息（实际服务的模型、provider 的请求 ID 等）以后加在 Response 上，
// 不加在 Message 上。
type Response struct {
	Message Message // Role 为 RoleAssistant
}
