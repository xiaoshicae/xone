//go:build go1.24

package web

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/xiaoshicae/xone/xerror"
)

// 启停的完整用例（超时强断、等 handler、h2c、TLS）在 xgin 里，经真实的 gin 服务跑；这里只钉几条边界

func TestServer_StopBeforeStartSkipsListening(t *testing.T) {
	s := &Server{}
	if err := s.Stop(context.Background(), "xtest"); err != nil {
		t.Fatalf("Stop before Start should be a no-op, got %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- s.Start("xtest", ServerConfig{Host: "127.0.0.1", Port: 1}, http.NotFoundHandler()) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Start after Stop should return nil without listening, got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Start after Stop should return immediately")
	}
}

func TestServer_ErrorsCarryTheIntegrationsModule(t *testing.T) {
	// 错误出自谁的 Start，就算谁报的：调用方 xerror.Is(err, "xgin") 要成立
	s := &Server{}
	err := s.Start("xtest", ServerConfig{CertFile: "c", KeyFile: "k", ClientCAFile: "/nonexistent/ca.pem"}, http.NotFoundHandler())
	if !xerror.Is(err, "xtest") {
		t.Fatalf("want an xerror from module xtest, got %v", err)
	}
}

func TestShutdownCtx_LeavesTailForHandlers(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	shut, cancelShut := shutdownCtx(ctx)
	defer cancelShut()
	outer, _ := ctx.Deadline()
	inner, _ := shut.Deadline()
	if tail := outer.Sub(inner); tail < 300*time.Millisecond || tail > time.Second {
		t.Errorf("Shutdown should end ~20%% (max 1s) before the caller's deadline, tail=%v", tail)
	}

	shut, cancelShut = shutdownCtx(context.Background())
	defer cancelShut()
	if _, ok := shut.Deadline(); ok {
		t.Error("no caller deadline means no Shutdown deadline either")
	}
}
