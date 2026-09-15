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

// TestSessionKVReachesLaterMiddleware handler 写入的 KV 必须能被 c.Next() 之后的中间件看到
//
// 这是整个中间件存在的理由：Log 中间件在 c.Next() 之后打访问日志，
// 若 handler 的 KV 到不了那里，"请求入口装容器、后续一路带下去"就是句空话。
// 旧实现注入的是不可变快照，handler 里 CtxWithKV 得到的是新 ctx，
// 不显式写回 c.Request 就丢了——而调用栈深处根本拿不到 *gin.Context。
func TestSessionKVReachesLaterMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(Session())

	var seenAfterNext map[string]any
	r.Use(func(c *gin.Context) {
		c.Next() // 模拟 Log 中间件：在 handler 之后读 ctx
		seenAfterNext = xlog.KVFromCtx(c.Request.Context())
	})

	r.GET("/test", func(c *gin.Context) {
		// 业务代码的自然写法：只拿 ctx，不碰 *gin.Context
		deepInBusinessCode(c.Request.Context())
		c.String(http.StatusOK, "ok")
	})

	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/test", nil))

	if seenAfterNext["userId"] != "u-1" {
		t.Errorf("handler 写入的 KV 应能被后置中间件看到，实际为 %v", seenAfterNext)
	}
}

// deepInBusinessCode 模拟调用栈深处的业务函数，只有 ctx 可用
func deepInBusinessCode(ctx context.Context) {
	xlog.AddKV(ctx, "userId", "u-1")
}

func TestSessionOpensKVScope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(Session())

	var scopeOpened bool
	r.GET("/test", func(c *gin.Context) {
		// 没有作用域时 AddKV 只能丢弃，所以中间件必须先把它装上
		scopeOpened = xlog.KVFromCtx(c.Request.Context()) != nil
		c.String(http.StatusOK, "ok")
	})

	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/test", nil))

	if !scopeOpened {
		t.Error("session 中间件应在请求开始时开启 KV 作用域")
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
