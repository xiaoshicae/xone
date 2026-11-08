// Package llm 定义「调用一次大模型」的接口、与 provider 无关的消息类型，以及发给模型的工具声明。
//
// 这是 xagent 最底下的一层，不依赖 xagent 的其他包：tool 包依赖它（工具的声明就是 ToolDef），
// xagent 依赖它和 tool。
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

// ToolDef 工具的声明：告诉模型「有这么一个工具可以用」。只描述，不执行。
//
// 可执行的工具在 tool 包，它通过 Def() 返回这份声明。
type ToolDef struct {
	Name        string          // 模型用它来指定工具
	Description string          // 这个工具做什么、什么时候该用
	InputSchema json.RawMessage // 参数的 JSON Schema
}

// Role 消息的发出方。
type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool" // 一次工具调用的结果
)

// StopReason 模型为什么停下来。只出现在 assistant 消息上。
type StopReason string

const (
	StopEndTurn   StopReason = "end_turn"   // 正常说完
	StopToolUse   StopReason = "tool_use"   // 要调用工具，等结果
	StopMaxTokens StopReason = "max_tokens" // 输出被长度上限截断，内容可能不完整
)

// Usage 一次调用消耗的 token。只出现在 assistant 消息上。
type Usage struct {
	InputTokens  int
	OutputTokens int
}

// Message 对话里的一条消息。
//
// 不同角色用到的字段不同：
//   - user：Parts 里只有文本
//   - assistant：Parts 里是文本和工具调用；StopReason、Usage 记录这次调用的结果
//   - tool：一条消息对应一次工具调用的结果；ToolCallID、ToolName、IsError 说明是哪次调用、成功与否，Parts 是结果内容
//
// 调用的元数据直接挂在消息上，所以对话历史本身就记录着每一步的停止原因和用量。
type Message struct {
	Role  Role
	Parts []Part

	// 仅 assistant
	StopReason StopReason
	Usage      Usage

	// 仅 tool
	ToolCallID string // 对应 ToolCall.ID
	ToolName   string
	IsError    bool // 为 true 时模型会把内容当作错误信息，通常会换个参数重试
}

// Part 消息的一个片段：一段文本，或者一个工具调用。
type Part struct {
	Text     string
	ToolCall *ToolCall // 非 nil 时这个片段是工具调用，只出现在 assistant 消息里
}

// ToolCall 模型要求调用一个工具。
type ToolCall struct {
	ID    string          // provider 生成的调用 ID，结果消息要用它对上
	Name  string          // 工具名
	Input json.RawMessage // 参数，JSON；是否合法由工具自己判断
}

// UserText 构造一条只有文本的用户消息。
func UserText(text string) Message {
	return Message{Role: RoleUser, Parts: []Part{{Text: text}}}
}

// ToolResult 构造一条工具结果消息。
func ToolResult(call ToolCall, content string, isError bool) Message {
	return Message{
		Role:       RoleTool,
		Parts:      []Part{{Text: content}},
		ToolCallID: call.ID,
		ToolName:   call.Name,
		IsError:    isError,
	}
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
