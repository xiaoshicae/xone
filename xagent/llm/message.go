package llm

import (
	"encoding/json"
	"strings"
)

// Role 消息的发出方。
type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool" // 一次工具调用的结果
)

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
