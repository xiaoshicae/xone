package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bytedance/mockey"
	"github.com/gin-gonic/gin"
	"github.com/smartystreets/goconvey/convey"
	"github.com/xiaoshicae/xone/v3/xlog"
)

func TestLogScope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(LogScope())

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

// TestLogScopeKVReachesLaterMiddleware handler 写入的 KV 必须能被 c.Next() 之后的中间件看到
//
// 这是整个中间件存在的理由：Log 中间件在 c.Next() 之后打访问日志，
// 若 handler 的 KV 到不了那里，"请求入口装容器、后续一路带下去"就是句空话。
// 旧实现注入的是不可变快照，handler 里 CtxWithKV 得到的是新 ctx，
// 不显式写回 c.Request 就丢了——而调用栈深处根本拿不到 *gin.Context。
func TestLogScopeKVReachesLaterMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(LogScope())

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

func TestLogScopeOpensKVScope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(LogScope())

	var scopeOpened bool
	r.GET("/test", func(c *gin.Context) {
		// 没有作用域时 AddKV 只能丢弃，所以中间件必须先把它装上
		scopeOpened = xlog.KVFromCtx(c.Request.Context()) != nil
		c.String(http.StatusOK, "ok")
	})

	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/test", nil))

	if !scopeOpened {
		t.Error("LogScope 中间件应在请求开始时开启 KV 作用域")
	}
}

func TestLogScopeChain(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(LogScope())

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

// TestLogSkippedWhenLevelDisabled 级别关闭时，中间件不应做任何为日志服务的准备
//
// body 快照、包装 ResponseWriter 捕获响应、请求结束后的脱敏与序列化，
// 存在的唯一目的就是拼出访问日志。级别关掉还照做，等于每个请求白付一遍
func TestLogSkippedWhenLevelDisabled(t *testing.T) {
	mockey.PatchConvey("TestLogSkippedWhenLevelDisabled", t, func() {
		mockey.Mock(xlog.Enabled).Return(false).Build()

		infoCalled := 0
		mockey.Mock(xlog.Info).To(func(context.Context, string, ...any) { infoCalled++ }).Build()

		parsed := 0
		mockey.Mock(ParseRequestInfoWithBody).To(func(*http.Request, []byte) map[string]any {
			parsed++
			return map[string]any{}
		}).Build()

		gin.SetMode(gin.TestMode)
		r := gin.New()
		r.Use(Log())

		var writerWrapped bool
		r.POST("/t", func(c *gin.Context) {
			_, writerWrapped = c.Writer.(*responseBodyWriter)
			c.String(http.StatusOK, "ok")
		})

		req := httptest.NewRequest(http.MethodPost, "/t", strings.NewReader(`{"password":"x"}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		convey.So(w.Code, convey.ShouldEqual, http.StatusOK)
		convey.So(infoCalled, convey.ShouldEqual, 0)
		convey.So(parsed, convey.ShouldEqual, 0)       // 没有构建 requestInfo
		convey.So(writerWrapped, convey.ShouldBeFalse) // 也没有包装 ResponseWriter
	})
}

// TestLogStillRunsWhenLevelEnabled 级别开启时行为不变
func TestLogStillRunsWhenLevelEnabled(t *testing.T) {
	mockey.PatchConvey("TestLogStillRunsWhenLevelEnabled", t, func() {
		mockey.Mock(xlog.Enabled).Return(true).Build()

		infoCalled := 0
		mockey.Mock(xlog.Info).To(func(context.Context, string, ...any) { infoCalled++ }).Build()

		gin.SetMode(gin.TestMode)
		r := gin.New()
		r.Use(Log())

		var writerWrapped bool
		r.POST("/t", func(c *gin.Context) {
			_, writerWrapped = c.Writer.(*responseBodyWriter)
			c.String(http.StatusOK, "ok")
		})

		req := httptest.NewRequest(http.MethodPost, "/t", strings.NewReader(`{"a":1}`))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(httptest.NewRecorder(), req)

		convey.So(infoCalled, convey.ShouldEqual, 1)
		convey.So(writerWrapped, convey.ShouldBeTrue)
	})
}

// TestGetSensitiveFieldFirstByte_LazyInit 缓存未建时应触发重建
func TestGetSensitiveFieldFirstByte_LazyInit(t *testing.T) {
	mockey.PatchConvey("TestGetSensitiveFieldFirstByte_LazyInit", t, func() {
		sensitiveMu.Lock()
		oldBytes, oldFirst := cachedFieldBytes, cachedFieldFirstByte
		cachedFieldBytes, cachedFieldFirstByte = nil, [256]bool{}
		sensitiveMu.Unlock()
		defer func() {
			sensitiveMu.Lock()
			cachedFieldBytes, cachedFieldFirstByte = oldBytes, oldFirst
			sensitiveMu.Unlock()
		}()

		first := getSensitiveFieldFirstByte()
		convey.So(first['p'], convey.ShouldBeTrue) // password
		convey.So(first['P'], convey.ShouldBeTrue) // 大小写两种形态都要在
		convey.So(first['z'], convey.ShouldBeFalse)
	})
}
