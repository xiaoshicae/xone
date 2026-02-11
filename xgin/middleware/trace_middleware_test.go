package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func TestGinXTraceMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(GinXTraceMiddleware())

	r.GET("/test", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestGinXTraceMiddlewareWithErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(GinXTraceMiddleware())

	r.GET("/error", func(c *gin.Context) {
		c.Error(http.ErrAbortHandler)
		c.String(http.StatusBadRequest, "error")
	})

	req := httptest.NewRequest("GET", "/error", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

func TestGinXTraceMiddlewareNoMatchRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(GinXTraceMiddleware())

	// 不注册路由，测试 404 情况（fullPath 为空）
	r.NoRoute(func(c *gin.Context) {
		c.String(http.StatusNotFound, "not found")
	})

	req := httptest.NewRequest("GET", "/not-exist", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", w.Code)
	}
}

func TestGinXTraceMiddlewareWithPropagation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(GinXTraceMiddleware())

	r.GET("/propagate", func(c *gin.Context) {
		// 验证 context 被正确传递
		ctx := c.Request.Context()
		if ctx == nil {
			t.Error("context should not be nil")
		}
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest("GET", "/propagate", nil)
	// 模拟带 trace header 的请求
	req.Header.Set("traceparent", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestGinXTraceMiddlewareStatusCodes(t *testing.T) {
	tests := []struct {
		name         string
		statusCode   int
		expectedCode int
	}{
		{"success", http.StatusOK, http.StatusOK},
		{"created", http.StatusCreated, http.StatusCreated},
		{"not found", http.StatusNotFound, http.StatusNotFound},
		{"internal error", http.StatusInternalServerError, http.StatusInternalServerError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			r := gin.New()
			r.Use(GinXTraceMiddleware())

			r.GET("/test", func(c *gin.Context) {
				c.Status(tt.statusCode)
			})

			req := httptest.NewRequest("GET", "/test", nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != tt.expectedCode {
				t.Errorf("expected status %d, got %d", tt.expectedCode, w.Code)
			}
		})
	}
}

func TestGinXTraceMiddleware_ValidSpan(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// 设置真实的 TracerProvider，使 span 具有有效的 SpanContext
	tp := sdktrace.NewTracerProvider()
	defer func() { _ = tp.Shutdown(context.Background()) }()

	origTP := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	defer otel.SetTracerProvider(origTP)

	r := gin.New()
	r.Use(GinXTraceMiddleware())

	r.GET("/trace-valid", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest("GET", "/trace-valid", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	// 验证 X-Trace-Id header 被设置
	traceID := w.Header().Get("X-Trace-Id")
	if traceID == "" {
		t.Error("X-Trace-Id header should be set when span is valid")
	}
}

func TestTracerNameConstant(t *testing.T) {
	expected := "github.com/xiaoshicae/xone/v2/xgin"
	if tracerName != expected {
		t.Errorf("tracerName should be %s, got %s", expected, tracerName)
	}
}
