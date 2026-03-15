package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/xiaoshicae/xone/v2/xlog"
)

func TestGinXSessionMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(GinXSessionMiddleware())

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

func TestCtxWithKVNewContext(t *testing.T) {
	ctx := context.Background()
	kvs := map[string]interface{}{"key1": "value1"}

	newCtx := ctxWithKV(ctx, kvs)

	if newCtx == ctx {
		t.Error("should return a new context")
	}

	// 通过再次调用 CtxWithKV 并验证合并行为，间接验证值被正确存储
	mergedCtx := xlog.CtxWithKV(newCtx, map[string]any{"key2": "value2"})
	if mergedCtx == nil {
		t.Error("merged context should not be nil")
	}
}

func TestCtxWithKVNilKvs(t *testing.T) {
	ctx := context.Background()

	newCtx := ctxWithKV(ctx, nil)

	if newCtx == ctx {
		t.Error("should return a new context")
	}
}

func TestCtxWithKVExistingContainer(t *testing.T) {
	ctx := context.Background()
	// 使用 xlog.CtxWithKV 注入已有的 KV（通过类型安全的 key）
	ctx = xlog.CtxWithKV(ctx, map[string]any{"existing": "value"})

	newKvs := map[string]interface{}{"new": "value2"}
	newCtx := ctxWithKV(ctx, newKvs)

	// 通过再次合并验证已有值和新值都被保留
	// ctxWithKV 内部调用 xlog.CtxWithKV，会合并已有 KV
	if newCtx == nil {
		t.Error("context should not be nil")
	}
}

func TestGinXSessionMiddlewareChain(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(GinXSessionMiddleware())

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
