package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/xiaoshicae/xone/v2/xlog"
)

func TestSession(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(Session())

	var ctxReceived context.Context
	r.GET("/test", func(c *gin.Context) {
		ctxReceived = c.Request.Context()
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	// 验证 context 被正确注入
	if ctxReceived == nil {
		t.Error("context should not be nil")
	}
}

func TestSessionInjectsKVContainer(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(Session())

	var injected bool
	var downstream map[string]any
	r.GET("/test", func(c *gin.Context) {
		ctx := c.Request.Context()
		// 中间件应提前注入空的 KV 容器
		injected = xlog.KVFromCtx(ctx) != nil
		// 后续 handler 注入的 KV 应能累积并被读取
		ctx = xlog.CtxWithKV(ctx, map[string]any{"userId": "u-1"})
		downstream = xlog.KVFromCtx(ctx)
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if !injected {
		t.Error("session 中间件应在请求开始时注入 KV 容器")
	}
	if downstream["userId"] != "u-1" {
		t.Errorf("后续注入的 KV 应可读取，实际为 %v", downstream)
	}
}

func TestSessionChain(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(Session())

	// 测试中间件链
	var middlewareCalled bool
	r.Use(func(c *gin.Context) {
		middlewareCalled = true
		c.Next()
	})

	r.GET("/test", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if !middlewareCalled {
		t.Error("subsequent middleware should be called")
	}
}
