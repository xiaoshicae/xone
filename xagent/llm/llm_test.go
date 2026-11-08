package llm

import "testing"

func TestMessage_ToolCallsAndTextKeepOrder(t *testing.T) {
	m := Message{Role: RoleAssistant, Parts: []Part{
		{Text: "let me "},
		{ToolCall: &ToolCall{ID: "1", Name: "a"}},
		{Text: "check"},
		{ToolCall: &ToolCall{ID: "2", Name: "b"}},
	}}
	calls := m.ToolCalls()
	if len(calls) != 2 || calls[0].ID != "1" || calls[1].ID != "2" {
		t.Fatalf("calls = %+v", calls)
	}
	if got := m.Text(); got != "let me check" {
		t.Fatalf("text = %q", got)
	}
	if UserText("hi").Role != RoleUser {
		t.Fatal("UserText should be a user message")
	}
}
