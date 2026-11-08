package xagent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/xiaoshicae/xone/xagent/llm"
	"github.com/xiaoshicae/xone/xagent/tool"
	"github.com/xiaoshicae/xone/xerror"
)

// scriptedModel 按顺序返回预设的回复，并记下每次收到的请求。
type scriptedModel struct {
	replies []*llm.Response
	reqs    []*llm.Request
}

func (m *scriptedModel) Generate(_ context.Context, req *llm.Request) (*llm.Response, error) {
	m.reqs = append(m.reqs, req)
	if len(m.reqs) > len(m.replies) {
		return nil, errors.New("script exhausted")
	}
	return m.replies[len(m.reqs)-1], nil
}

func say(text string) *llm.Response {
	return &llm.Response{
		Message:    llm.Message{Role: llm.RoleAssistant, Parts: []llm.Part{{Text: text}}},
		StopReason: llm.StopEndTurn,
	}
}

func callTools(calls ...llm.ToolCall) *llm.Response {
	msg := llm.Message{Role: llm.RoleAssistant}
	for i := range calls {
		msg.Parts = append(msg.Parts, llm.Part{ToolCall: &calls[i]})
	}
	return &llm.Response{Message: msg, StopReason: llm.StopToolUse}
}

// funcTool 用一个函数实现 tool.Tool。
type funcTool struct {
	name string
	fn   func(ctx context.Context, input json.RawMessage) (string, error)
}

func (t funcTool) Spec() tool.Spec { return tool.Spec{Name: t.name, Description: t.name} }
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

// toolResults 取出一条消息里的全部工具结果。
func toolResults(t *testing.T, m llm.Message) []llm.ToolResult {
	t.Helper()
	if m.Role != llm.RoleUser {
		t.Fatalf("tool results must be in a user message, got role %q", m.Role)
	}
	var out []llm.ToolResult
	for _, p := range m.Parts {
		if p.ToolResult == nil {
			t.Fatalf("unexpected non-result part in tool result message: %+v", p)
		}
		out = append(out, *p.ToolResult)
	}
	return out
}

func TestRun_ReturnsAnswerWhenModelCallsNoTool(t *testing.T) {
	m := &scriptedModel{replies: []*llm.Response{say("hello")}}
	a := mustNew(t, m, nil, WithSystem("be brief"))

	res, err := a.Run(context.Background(), []llm.Message{llm.UserText("hi")})
	if err != nil {
		t.Fatal(err)
	}
	if res.Text() != "hello" || res.Steps != 1 || len(res.Messages) != 2 {
		t.Fatalf("got text=%q steps=%d messages=%d", res.Text(), res.Steps, len(res.Messages))
	}
	if m.reqs[0].System != "be brief" {
		t.Fatalf("system prompt not passed: %q", m.reqs[0].System)
	}
}

func TestRun_FeedsAllToolResultsBackInOneMessageInOrder(t *testing.T) {
	m := &scriptedModel{replies: []*llm.Response{
		callTools(
			llm.ToolCall{ID: "c1", Name: "a", Input: json.RawMessage(`1`)},
			llm.ToolCall{ID: "c2", Name: "b", Input: json.RawMessage(`2`)},
		),
		say("done"),
	}}
	a := mustNew(t, m, []tool.Tool{echo("a"), echo("b")})

	res, err := a.Run(context.Background(), []llm.Message{llm.UserText("go")})
	if err != nil {
		t.Fatal(err)
	}
	if res.Steps != 2 || res.Text() != "done" {
		t.Fatalf("got steps=%d text=%q", res.Steps, res.Text())
	}

	// 第二次请求：用户输入、发起调用的 assistant 消息、一条装着两个结果的 user 消息
	second := m.reqs[1].Messages
	if len(second) != 3 || second[1].Role != llm.RoleAssistant {
		t.Fatalf("unexpected history on second request: %+v", second)
	}
	got := toolResults(t, second[2])
	want := []llm.ToolResult{{CallID: "c1", Content: "a:1"}, {CallID: "c2", Content: "b:2"}}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("tool results = %+v, want %+v", got, want)
	}
}

func TestRun_SendsToolDefinitionsToModel(t *testing.T) {
	m := &scriptedModel{replies: []*llm.Response{say("ok")}}
	a := mustNew(t, m, []tool.Tool{echo("a"), echo("b")})

	if _, err := a.Run(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	defs := m.reqs[0].Tools
	if len(defs) != 2 || defs[0].Name != "a" || defs[1].Name != "b" {
		t.Fatalf("tool defs = %+v", defs)
	}
}

func TestRun_ToolFailuresBecomeErrorResultsAndLoopContinues(t *testing.T) {
	failing := funcTool{name: "fail", fn: func(context.Context, json.RawMessage) (string, error) {
		return "", errors.New("bad input")
	}}
	panicking := funcTool{name: "boom", fn: func(context.Context, json.RawMessage) (string, error) {
		panic("kaboom")
	}}
	m := &scriptedModel{replies: []*llm.Response{
		callTools(
			llm.ToolCall{ID: "c1", Name: "fail"},
			llm.ToolCall{ID: "c2", Name: "boom"},
			llm.ToolCall{ID: "c3", Name: "missing"},
			llm.ToolCall{ID: "c4", Name: "ok"},
		),
		say("recovered"),
	}}
	a := mustNew(t, m, []tool.Tool{failing, panicking, echo("ok")})

	res, err := a.Run(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Text() != "recovered" {
		t.Fatalf("loop did not continue after tool failures: %q", res.Text())
	}
	got := toolResults(t, res.Messages[1])
	wantErr := map[string]string{"c1": "bad input", "c2": "panicked: kaboom", "c3": `unknown tool "missing"`}
	if len(got) != 4 {
		t.Fatalf("want 4 results, got %+v", got)
	}
	for _, r := range got[:3] {
		if !r.IsError || !strings.Contains(r.Content, wantErr[r.CallID]) {
			t.Errorf("result %s = %+v, want error containing %q", r.CallID, r, wantErr[r.CallID])
		}
	}
	if got[3].IsError || got[3].Content != "ok:" {
		t.Errorf("healthy tool result = %+v", got[3])
	}
}

func TestRun_StopsAtMaxStepsWithValidHistory(t *testing.T) {
	loop := callTools(llm.ToolCall{ID: "c", Name: "a"})
	m := &scriptedModel{replies: []*llm.Response{loop, loop, loop, loop}}
	a := mustNew(t, m, []tool.Tool{echo("a")}, WithMaxSteps(2))

	res, err := a.Run(context.Background(), nil)
	if err == nil || !strings.Contains(err.Error(), "exceeded max steps 2") {
		t.Fatalf("want max steps error, got %v", err)
	}
	if !xerror.Is(err, "xagent") {
		t.Fatalf("error should be an xagent xerror: %v", err)
	}
	if res.Steps != 2 || len(m.reqs) != 2 {
		t.Fatalf("model called %d times, steps=%d, want 2", len(m.reqs), res.Steps)
	}
	// 最后一条是工具结果：历史可以直接拿去继续
	if len(res.Messages) != 4 || len(toolResults(t, res.Messages[3])) != 1 {
		t.Fatalf("history should end with tool results: %+v", res.Messages)
	}
}

func TestRun_CanceledDuringToolsStillAnswersEveryCall(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancelling := funcTool{name: "cancel", fn: func(context.Context, json.RawMessage) (string, error) {
		cancel()
		return "canceled it", nil
	}}
	called := false
	never := funcTool{name: "never", fn: func(context.Context, json.RawMessage) (string, error) {
		called = true
		return "", nil
	}}
	m := &scriptedModel{replies: []*llm.Response{
		callTools(llm.ToolCall{ID: "c1", Name: "cancel"}, llm.ToolCall{ID: "c2", Name: "never"}),
		say("unreachable"),
	}}
	a := mustNew(t, m, []tool.Tool{cancelling, never})

	res, err := a.Run(ctx, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("want context.Canceled, got %v", err)
	}
	if called {
		t.Fatal("tool after cancellation should not run")
	}
	if len(m.reqs) != 1 {
		t.Fatalf("model should not be called after cancellation, called %d times", len(m.reqs))
	}
	got := toolResults(t, res.Messages[len(res.Messages)-1])
	if len(got) != 2 || got[0].IsError || !got[1].IsError || got[1].CallID != "c2" {
		t.Fatalf("every call needs a result even when canceled: %+v", got)
	}
}

func TestRun_TruncatedReplyIsAnErrorAndNotKept(t *testing.T) {
	truncated := callTools(llm.ToolCall{ID: "c", Name: "a", Input: json.RawMessage(`{"q":`)})
	truncated.StopReason = llm.StopMaxTokens
	m := &scriptedModel{replies: []*llm.Response{truncated}}
	a := mustNew(t, m, []tool.Tool{echo("a")})

	res, err := a.Run(context.Background(), []llm.Message{llm.UserText("hi")})
	if err == nil || !strings.Contains(err.Error(), "truncated") {
		t.Fatalf("want truncation error, got %v", err)
	}
	if len(res.Messages) != 1 {
		t.Fatalf("truncated reply must not enter history: %+v", res.Messages)
	}
}

func TestRun_ModelErrorIsWrapped(t *testing.T) {
	m := &scriptedModel{} // 没有预设回复：第一次调用就报错
	a := mustNew(t, m, nil)

	_, err := a.Run(context.Background(), nil)
	if err == nil || !strings.Contains(err.Error(), "script exhausted") || !xerror.Is(err, "xagent") {
		t.Fatalf("want wrapped model error, got %v", err)
	}
}

func TestRun_DoesNotModifyCallerMessages(t *testing.T) {
	in := make([]llm.Message, 1, 10) // 留足容量：一旦直接 append，就会写进这个底层数组
	in[0] = llm.UserText("hi")
	m := &scriptedModel{replies: []*llm.Response{say("a"), say("b")}}
	a := mustNew(t, m, nil)

	if _, err := a.Run(context.Background(), in); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Run(context.Background(), in); err != nil {
		t.Fatal(err)
	}
	if got := in[:2][1]; got.Role != "" {
		t.Fatalf("Run wrote into the caller's backing array: %+v", got)
	}
}

func TestNew_RejectsInvalidArguments(t *testing.T) {
	m := &scriptedModel{}
	cases := map[string]func() (*Agent, error){
		"nil model":      func() (*Agent, error) { return New(nil, nil) },
		"zero max steps": func() (*Agent, error) { return New(m, nil, WithMaxSteps(0)) },
		"duplicate tool": func() (*Agent, error) { return New(m, []tool.Tool{echo("a"), echo("a")}) },
	}
	for name, fn := range cases {
		if _, err := fn(); err == nil || !xerror.Is(err, "xagent") {
			t.Errorf("%s: want xagent error, got %v", name, err)
		}
	}
}
