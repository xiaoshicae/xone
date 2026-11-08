package xagent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/xiaoshicae/xone/xagent/llm"
	"github.com/xiaoshicae/xone/xagent/tool"
)

func TestRun_FeedsEachToolResultBackAsToolMessageInOrder(t *testing.T) {
	m := &scriptedModel{replies: []llm.Message{
		toolUseReply(
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

	// 第二次请求：用户输入、发起调用的 assistant 消息、两条 tool 消息
	second := m.reqs[1].Messages
	if len(second) != 4 || second[1].Role != llm.RoleAssistant {
		t.Fatalf("unexpected history on second request: %+v", second)
	}
	want := []struct{ id, name, text string }{{"c1", "a", "a:1"}, {"c2", "b", "b:2"}}
	for i, w := range want {
		got := second[2+i]
		if got.Role != llm.RoleTool || got.ToolCallID != w.id || got.ToolName != w.name || got.Text() != w.text || got.IsError {
			t.Errorf("result #%d = %+v, want %+v", i, got, w)
		}
	}
}

func TestRun_ToolFailuresBecomeErrorResultsAndLoopContinues(t *testing.T) {
	failing := funcTool{name: "fail", fn: func(context.Context, json.RawMessage) (string, error) {
		return "", errors.New("bad input")
	}}
	panicking := funcTool{name: "boom", fn: func(context.Context, json.RawMessage) (string, error) {
		panic("kaboom")
	}}
	m := &scriptedModel{replies: []llm.Message{
		toolUseReply(
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
	got := res.Messages[1:5]
	wantErr := map[string]string{"c1": "bad input", "c2": "panicked: kaboom", "c3": `unknown tool "missing"`}
	for _, r := range got[:3] {
		if r.Role != llm.RoleTool || !r.IsError || !strings.Contains(r.Text(), wantErr[r.ToolCallID]) {
			t.Errorf("result %s = %+v, want error containing %q", r.ToolCallID, r, wantErr[r.ToolCallID])
		}
	}
	if got[3].IsError || got[3].Text() != "ok:" {
		t.Errorf("healthy tool result = %+v", got[3])
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
	m := &scriptedModel{replies: []llm.Message{
		toolUseReply(llm.ToolCall{ID: "c1", Name: "cancel"}, llm.ToolCall{ID: "c2", Name: "never"}),
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
	got := lastToolResults(res.Messages)
	if len(got) != 2 || got[0].IsError || !got[1].IsError || got[1].ToolCallID != "c2" {
		t.Fatalf("every call needs a result even when canceled: %+v", got)
	}
}
