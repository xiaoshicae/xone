package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/xiaoshicae/xone/v2/xlog"
)

// TestMiddlewareLogsGoThroughXLog 回归防护：
// 中间件日志必须经由 xlog 输出，与业务日志共用同一套格式与输出目标。
// 此前这两处直接使用全局 logrus，xlog 改用私有实例后日志被静默旁路。
func TestMiddlewareLogsGoThroughXLog(t *testing.T) {
	var records []xlog.Record
	xlog.AddObserver(func(_ context.Context, r xlog.Record) {
		records = append(records, r)
	})

	gin.SetMode(gin.TestMode)

	t.Run("日志中间件", func(t *testing.T) {
		records = nil
		r := gin.New()
		r.Use(LogMiddleware())
		r.GET("/ok", func(c *gin.Context) { c.String(http.StatusOK, "ok") })

		r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/ok", nil))

		if len(records) == 0 {
			t.Fatal("请求日志未经由 xlog 输出")
		}
		if records[0].Level != xlog.InfoLevel {
			t.Errorf("期望 Info 级别，实际 %s", records[0].Level)
		}
	})

	t.Run("Recover中间件", func(t *testing.T) {
		records = nil
		r := gin.New()
		r.Use(GinXRecoverMiddleware(nil))
		r.GET("/panic", func(c *gin.Context) { panic("boom") })

		r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/panic", nil))

		if len(records) == 0 {
			t.Fatal("panic 日志未经由 xlog 输出")
		}
		if records[0].Level != xlog.ErrorLevel {
			t.Errorf("期望 Error 级别，实际 %s", records[0].Level)
		}
	})
}
