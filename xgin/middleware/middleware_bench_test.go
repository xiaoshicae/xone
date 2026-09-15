package middleware

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bytedance/mockey"
	"github.com/gin-gonic/gin"
	"github.com/xiaoshicae/xone/v2/xlog"
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

// BenchmarkLog_InfoMocked 把 xlog.Info 换成空操作后，Log 中间件还剩多少开销
//
// 日志级别关闭时 xlog.Info 在内部检查后即返回，等价于空操作。
// 所以这里剩下的就是「无论日志写不写都会白付」的构建开销：
// requestInfo 是 Info 的入参，级别检查发生在它构建完之后
func BenchmarkLog_InfoMocked(b *testing.B) {
	mock := mockey.Mock(xlog.Info).Return().Build()
	defer mock.UnPatch()

	runN(b, newEngine(Log()), jsonReq(0))
}
