package llm

import (
	"context"
)

// Model 调用一次大模型：给一段对话，拿回模型的下一条消息。
//
// 返回的消息 Role 为 RoleAssistant，并带上 StopReason 和 Usage。
// 实现应当是无状态的：对话历史全部在 req.Messages 里，由调用方维护。
// 这样同一个 Model 可以被多个 agent、多个并发请求共用。
type Model interface {
	Generate(ctx context.Context, req *Request) (Message, error)
}

// Request 一次调用的输入。
type Request struct {
	System   string    // 系统提示词
	Messages []Message // 对话历史，按时间顺序
	Tools    []ToolDef // 本次允许模型调用的工具；为空时模型只能回复文本
}
