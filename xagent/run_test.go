package xagent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/xiaoshicae/xone/xagent/llm"
	"github.com/xiaoshicae/xone/xagent/tool"
	"github.com/xiaoshicae/xone/xerror"
)

func TestRun_ReturnsAnswerWhenModelCallsNoTool(t *testing.T) {
	m := &scriptedModel{replies: []llm.Message{say("hello")}}
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

func TestRun_SendsToolDefinitionsToModel(t *testing.T) {
	m := &scriptedModel{replies: []llm.Message{say("ok")}}
	a := mustNew(t, m, []tool.Tool{echo("a"), echo("b")})

	if _, err := a.Run(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	defs := m.reqs[0].Tools
	if len(defs) != 2 || defs[0].Name != "a" || defs[1].Name != "b" {
		t.Fatalf("tool defs = %+v", defs)
	}
}

func TestRun_SumsUsageAcrossSteps(t *testing.T) {
	first := toolUseReply(llm.ToolCall{ID: "c", Name: "a"})
	first.Usage = llm.Usage{InputTokens: 10, OutputTokens: 3}
	second := say("done")
	second.Usage = llm.Usage{InputTokens: 20, OutputTokens: 5}
	m := &scriptedModel{replies: []llm.Message{first, second}}
	a := mustNew(t, m, []tool.Tool{echo("a")})

	res, err := a.Run(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if want := (llm.Usage{InputTokens: 30, OutputTokens: 8}); res.Usage != want {
		t.Fatalf("usage = %+v, want %+v", res.Usage, want)
	}
	// 每一步的明细留在对应的 assistant 消息上
	if res.Messages[0].Usage != first.Usage {
		t.Fatalf("per-step usage lost: %+v", res.Messages[0].Usage)
	}
}

func TestRun_StopsAtMaxStepsWithValidHistory(t *testing.T) {
	loop := toolUseReply(llm.ToolCall{ID: "c", Name: "a"})
	m := &scriptedModel{replies: []llm.Message{loop, loop, loop, loop}}
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
	if len(res.Messages) != 4 || len(lastToolResults(res.Messages)) != 1 {
		t.Fatalf("history should end with tool results: %+v", res.Messages)
	}
}

func TestRun_TruncatedReplyIsAnErrorAndNotKept(t *testing.T) {
	truncated := toolUseReply(llm.ToolCall{ID: "c", Name: "a", Input: json.RawMessage(`{"q":`)})
	truncated.StopReason = llm.StopMaxTokens
	truncated.Usage = llm.Usage{InputTokens: 7, OutputTokens: 100}
	m := &scriptedModel{replies: []llm.Message{truncated}}
	a := mustNew(t, m, []tool.Tool{echo("a")})

	res, err := a.Run(context.Background(), []llm.Message{llm.UserText("hi")})
	if err == nil || !strings.Contains(err.Error(), "truncated") {
		t.Fatalf("want truncation error, got %v", err)
	}
	if len(res.Messages) != 1 {
		t.Fatalf("truncated reply must not enter history: %+v", res.Messages)
	}
	// 回复没进历史，但 token 已经花掉了，照样计入
	if res.Usage.OutputTokens != 100 {
		t.Fatalf("usage of truncated reply should still count: %+v", res.Usage)
	}
}

func TestRun_RecordsReplyAsAssistantEvenIfModelOmitsRole(t *testing.T) {
	reply := say("hi")
	reply.Role = "" // 适配层漏填了角色
	m := &scriptedModel{replies: []llm.Message{reply}}
	a := mustNew(t, m, nil)

	res, err := a.Run(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Messages[0].Role != llm.RoleAssistant || res.Text() != "hi" {
		t.Fatalf("reply should be recorded as assistant: %+v", res.Messages[0])
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
	m := &scriptedModel{replies: []llm.Message{say("a"), say("b")}}
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
