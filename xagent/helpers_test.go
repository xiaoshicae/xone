package xagent

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/xiaoshicae/xone/xagent/llm"
	"github.com/xiaoshicae/xone/xagent/tool"
)

// scriptedModel 按顺序返回预设的回复，并记下每次收到的请求。
type scriptedModel struct {
	replies []llm.Message
	reqs    []*llm.Request
}

func (m *scriptedModel) Generate(_ context.Context, req *llm.Request) (llm.Message, error) {
	m.reqs = append(m.reqs, req)
	if len(m.reqs) > len(m.replies) {
		return llm.Message{}, errors.New("script exhausted")
	}
	return m.replies[len(m.reqs)-1], nil
}

func say(text string) llm.Message {
	return llm.Message{Role: llm.RoleAssistant, Parts: []llm.Part{{Text: text}}, StopReason: llm.StopEndTurn}
}

func toolUseReply(calls ...llm.ToolCall) llm.Message {
	msg := llm.Message{Role: llm.RoleAssistant, StopReason: llm.StopToolUse}
	for i := range calls {
		msg.Parts = append(msg.Parts, llm.Part{ToolCall: &calls[i]})
	}
	return msg
}

// funcTool 用一个函数实现 tool.Tool。
type funcTool struct {
	name string
	fn   func(ctx context.Context, input json.RawMessage) (string, error)
}

func (t funcTool) Def() llm.ToolDef { return llm.ToolDef{Name: t.name, Description: t.name} }

func (t funcTool) Call(ctx context.Context, in json.RawMessage) (string, error) {
	return t.fn(ctx, in)
}

func echo(name string) funcTool {
	return funcTool{name: name, fn: func(_ context.Context, in json.RawMessage) (string, error) {
		return name + ":" + string(in), nil
	}}
}

func mustNew(t *testing.T, m llm.Model, tools []tool.Tool, opts ...Option) *Agent {
	t.Helper()
	a, err := New(m, tools, opts...)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

// lastToolResults 取出对话末尾连续的 tool 消息。
func lastToolResults(msgs []llm.Message) []llm.Message {
	i := len(msgs)
	for i > 0 && msgs[i-1].Role == llm.RoleTool {
		i--
	}
	return msgs[i:]
}
