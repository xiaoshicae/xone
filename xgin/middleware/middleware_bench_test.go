package middleware

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bytedance/mockey"
	"github.com/gin-gonic/gin"
	"github.com/xiaoshicae/xone/v3/xlog"
)

// 各中间件的热路径基准
//
// 统一走完整的 gin 请求链路，测的是「加上这个中间件之后每个请求多付出多少」，
// 而不是单个函数的开销——后者容易在真实链路里被别的开销淹没，得出错误结论。

func init() { gin.SetMode(gin.ReleaseMode) }

// newEngine 构造只挂了指定中间件的最小引擎
func newEngine(mw ...gin.HandlerFunc) *gin.Engine {
	r := gin.New()
	for _, m := range mw {
		r.Use(m)
	}
	r.POST("/api/order/:id", func(c *gin.Context) {
		// 必须真的把 body 读掉：中间件降级路径下 body 是在下游读取时才被捕获的，
		// handler 不读就等于没有 body，脱敏与过滤那条路径根本跑不到
		_, _ = io.Copy(io.Discard, c.Request.Body)
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	r.GET("/api/order/:id", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	return r
}

func runN(b *testing.B, r *gin.Engine, newReq func() *http.Request) {
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, newReq())
	}
}

// jsonReq 构造带 JSON body 的请求，body 大小由 pad 控制
func jsonReq(pad int) func() *http.Request {
	body := `{"userId":"u-1","amount":99,"note":"` + strings.Repeat("x", pad) + `"}`
	return func() *http.Request {
		req := httptest.NewRequest(http.MethodPost, "/api/order/42", bytes.NewReader([]byte(body)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer token-abc")
		req.Header.Set("User-Agent", "bench/1.0")
		req.Header.Set("X-Forwarded-For", "203.0.113.7, 10.0.0.1")
		return req
	}
}

func getReq() *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/api/order/42", nil)
	req.Header.Set("User-Agent", "bench/1.0")
	return req
}

// BenchmarkBaseline 不挂任何中间件，作为其余项的减法基准
func BenchmarkBaseline(b *testing.B) {
	runN(b, newEngine(), jsonReq(0))
}

func BenchmarkLogScope(b *testing.B) {
	runN(b, newEngine(LogScope()), jsonReq(0))
}

func BenchmarkRecover(b *testing.B) {
	runN(b, newEngine(Recover(nil)), jsonReq(0))
}

func BenchmarkMetric(b *testing.B) {
	runN(b, newEngine(Metric()), jsonReq(0))
}

func BenchmarkTrace(b *testing.B) {
	runN(b, newEngine(Trace()), jsonReq(0))
}

// BenchmarkLog_SmallJSONBody 典型 API 请求：小 JSON body + 敏感头
func BenchmarkLog_SmallJSONBody(b *testing.B) {
	runN(b, newEngine(Log()), jsonReq(0))
}

// BenchmarkLog_LargeJSONBody body 变大时的开销增长
func BenchmarkLog_LargeJSONBody(b *testing.B) {
	runN(b, newEngine(Log()), jsonReq(64*1024))
}

// BenchmarkLog_NoBody GET 请求，隔离掉 body 处理的开销
func BenchmarkLog_NoBody(b *testing.B) {
	runN(b, newEngine(Log()), func() *http.Request { return getReq() })
}

// BenchmarkLog_Skipped 命中 skip 列表时应当接近 baseline
func BenchmarkLog_Skipped(b *testing.B) {
	r := gin.New()
	r.Use(Log(WithSkipPaths("/api/order/42")))
	r.POST("/api/order/:id", func(c *gin.Context) { c.Status(http.StatusOK) })
	runN(b, r, jsonReq(0))
}

// BenchmarkFullChain 生产默认配置：五个中间件全挂
func BenchmarkFullChain(b *testing.B) {
	runN(b, newEngine(LogScope(), Trace(), Log(), Metric(), Recover(nil)), jsonReq(0))
}

// BenchmarkLog_LevelDisabled 服务跑在 warn 级别时，Log 中间件还剩多少开销
//
// 级别关掉时 access log 不会输出，但中间件为它做的准备工作——body 快照、
// 包装 ResponseWriter 捕获响应、请求结束后的脱敏与序列化——是否也一并省掉，
// 决定了「调高日志级别」这个最常用的降噪手段能不能同时降开销
func BenchmarkLog_LevelDisabled(b *testing.B) {
	mock := mockey.Mock(xlog.Enabled).Return(false).Build()
	defer mock.UnPatch()

	// mockey 依赖 -gcflags="all=-N -l"，不带这个标志时 mock 静默失效，
	// 测出来的会是「级别开启」的数字而看不出任何异常。宁可跳过也不要给错数
	if xlog.Enabled(context.Background(), xlog.InfoLevel) {
		b.Skip(`mock 未生效，本用例需要 -gcflags="all=-N -l"`)
	}

	runN(b, newEngine(Log()), jsonReq(0))
}
