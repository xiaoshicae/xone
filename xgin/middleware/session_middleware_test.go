package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/xiaoshicae/xone/xlog"
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

	// 验证值被存储
	stored := newCtx.Value(xlog.XLogCtxKVContainerKey)
	if stored == nil {
		t.Error("should store the kvs in context")
	}

	storedMap, ok := stored.(map[string]interface{})
	if !ok {
		t.Error("stored value should be a map")
	}

	if storedMap["key1"] != "value1" {
		t.Error("stored value should contain the key1")
	}
}

func TestCtxWithKVNilKvs(t *testing.T) {
	ctx := context.Background()

	newCtx := ctxWithKV(ctx, nil)

	if newCtx == ctx {
		t.Error("should return a new context")
	}

	stored := newCtx.Value(xlog.XLogCtxKVContainerKey)
	if stored == nil {
		t.Error("should store empty map in context")
	}
}

func TestCtxWithKVExistingContainer(t *testing.T) {
	ctx := context.Background()
	existingKvs := map[string]interface{}{"existing": "value"}
	ctx = context.WithValue(ctx, xlog.XLogCtxKVContainerKey, existingKvs)

	newKvs := map[string]interface{}{"new": "value2"}
	newCtx := ctxWithKV(ctx, newKvs)

	// 应该返回原 context，但更新了 map
	stored := newCtx.Value(xlog.XLogCtxKVContainerKey).(map[string]interface{})

	if stored["existing"] != "value" {
		t.Error("existing value should be preserved")
	}
	if stored["new"] != "value2" {
		t.Error("new value should be added")
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
