// Package llm 定义「调用一次大模型」的接口和与 provider 无关的消息类型。
//
// 这个包不认识 tool 包：模型只需要知道有哪些工具（名字、描述、参数 schema），
// 不需要知道工具怎么执行。两者在 xagent 里汇合。
//
// 各家 provider（Anthropic、OpenAI……）的适配各自实现 Model，
// 在适配层把这里的 Message 与它们自己的请求 / 响应格式互相转换。
package llm

import (
	"context"
	"encoding/json"
	"strings"
)

// Model 调用一次大模型：给一段对话，拿回模型的下一条消息。
//
// 实现应当是无状态的：对话历史全部在 req.Messages 里，由调用方维护。
// 这样同一个 Model 可以被多个 agent、多个并发请求共用。
type Model interface {
	Generate(ctx context.Context, req *Request) (*Response, error)
}

// Request 一次调用的输入。
type Request struct {
	System   string    // 系统提示词
	Messages []Message // 对话历史，按时间顺序
	Tools    []ToolDef // 本次允许模型调用的工具；为空时模型只能回复文本
}

// ToolDef 告诉模型「有这么一个工具可以用」。
//
// 和 tool.Spec 字段相同但刻意各定义一份：llm 和 tool 互不 import，转换只发生在 xagent 里。
type ToolDef struct {
	Name        string
	Description string
	InputSchema json.RawMessage // 参数的 JSON Schema
}

// Response 一次调用的输出。
type Response struct {
	Message    Message // 模型产出的消息，Role 恒为 RoleAssistant
	StopReason StopReason
}

// StopReason 模型为什么停下来。
type StopReason string

const (
	StopEndTurn   StopReason = "end_turn"   // 正常说完
	StopToolUse   StopReason = "tool_use"   // 要调用工具，等结果
	StopMaxTokens StopReason = "max_tokens" // 输出被长度上限截断，内容可能不完整
)

// Role 消息的发出方。
type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

// Message 对话里的一条消息。一条消息由若干 Part 组成：
// 模型的一次回复可以是「一段文字 + 两个工具调用」。
type Message struct {
	Role  Role
	Parts []Part
}

// Part 消息的一个片段。三个字段里恰好有一个有值。
type Part struct {
	Text       string
	ToolCall   *ToolCall   // 只出现在 assistant 消息里
	ToolResult *ToolResult // 只出现在 user 消息里
}

// ToolCall 模型要求调用一个工具。
type ToolCall struct {
	ID    string          // provider 生成的调用 ID，结果要用它对上
	Name  string          // 工具名
	Input json.RawMessage // 参数，JSON；是否合法由工具自己判断
}

// ToolResult 一次工具调用的结果，回给模型。
type ToolResult struct {
	CallID  string // 对应 ToolCall.ID
	Content string
	IsError bool // 为 true 时模型会把 Content 当作错误信息，通常会换个参数重试
}

// UserText 构造一条只有文本的用户消息。
func UserText(text string) Message {
	return Message{Role: RoleUser, Parts: []Part{{Text: text}}}
}

// ToolCalls 取出消息里的全部工具调用，保持原顺序。
func (m Message) ToolCalls() []ToolCall {
	var calls []ToolCall
	for _, p := range m.Parts {
		if p.ToolCall != nil {
			calls = append(calls, *p.ToolCall)
		}
	}
	return calls
}

// Text 把消息里的文本片段按顺序拼起来。
func (m Message) Text() string {
	var b strings.Builder
	for _, p := range m.Parts {
		b.WriteString(p.Text)
	}
	return b.String()
}
