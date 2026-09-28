package web

import (
	"errors"
	"net"
	"os"
	"strings"
	"testing"
)

func TestNormalizeMethod(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"GET", "GET"}, {"POST", "POST"}, {"PATCH", "PATCH"}, {"DELETE", "DELETE"},
		{"CONNECT", "CONNECT"}, {"OPTIONS", "OPTIONS"}, {"TRACE", "TRACE"}, {"HEAD", "HEAD"},
		{"CUSTOM1", "OTHER"}, {"FOOBAR", "OTHER"}, {"get", "OTHER"}, {"", "OTHER"},
	} {
		if got := NormalizeMethod(c.in); got != c.want {
			t.Errorf("NormalizeMethod(%q)=%q want %q", c.in, got, c.want)
		}
	}
}

func TestIsBrokenPipe(t *testing.T) {
	// 客户端提前断开不算故障，不值得打一份完整栈
	broken := &net.OpError{Err: &os.SyscallError{Syscall: "write", Err: errors.New("broken pipe")}}
	if !IsBrokenPipe(broken) {
		t.Error("broken pipe should be recognized")
	}
	reset := &net.OpError{Err: &os.SyscallError{Syscall: "read", Err: errors.New("connection reset by peer")}}
	if !IsBrokenPipe(reset) {
		t.Error("connection reset should be recognized")
	}
	if IsBrokenPipe("plain panic") || IsBrokenPipe(&net.OpError{Err: errors.New("other")}) {
		t.Error("other errors are not a broken connection")
	}
}

func TestStack_CapturesCurrentGoroutineWithinLimit(t *testing.T) {
	s := Stack()
	if !strings.Contains(s, "TestStack_CapturesCurrentGoroutineWithinLimit") {
		t.Errorf("stack should contain the caller, got:\n%s", s)
	}
	if len(s) > maxStack {
		t.Errorf("stack longer than the %d byte limit: %d", maxStack, len(s))
	}
}
