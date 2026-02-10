package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestGinXRecoverMiddlewareWithDefaultHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(GinXRecoverMiddleware(nil))

	r.GET("/panic", func(c *gin.Context) {
		panic("test panic")
	})

	req := httptest.NewRequest("GET", "/panic", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected status 500, got %d", w.Code)
	}
}

func TestGinXRecoverMiddlewareWithCustomHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	customHandlerCalled := false
	customHandler := func(c *gin.Context, err interface{}) {
		customHandlerCalled = true
		c.JSON(http.StatusOK, gin.H{"error": "recovered"})
	}

	r.Use(GinXRecoverMiddleware(customHandler))

	r.GET("/panic", func(c *gin.Context) {
		panic("test panic")
	})

	req := httptest.NewRequest("GET", "/panic", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if !customHandlerCalled {
		t.Error("custom handler should be called")
	}
	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestGinXRecoverMiddlewareNoPanic(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(GinXRecoverMiddleware(nil))

	r.GET("/normal", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest("GET", "/normal", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
	if w.Body.String() != "ok" {
		t.Errorf("expected body 'ok', got %s", w.Body.String())
	}
}

func TestGinXRecoverMiddlewareResponseAlreadyWritten(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(GinXRecoverMiddleware(nil))

	r.GET("/panic", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
		panic("test panic")
	})

	req := httptest.NewRequest("GET", "/panic", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
	if w.Body.String() != "ok" {
		t.Errorf("expected body 'ok', got %s", w.Body.String())
	}
}

func TestDefaultHandleRecovery(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	defaultHandleRecovery(c, "test error")

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected status 500, got %d", w.Code)
	}
}

func TestStackFunction(t *testing.T) {
	// 测试 stack 函数
	stackBytes := stack(0)
	if len(stackBytes) == 0 {
		t.Error("stack should not be empty")
	}
}

func TestSourceFunction(t *testing.T) {
	lines := [][]byte{
		[]byte("line 1"),
		[]byte("  line 2  "),
		[]byte("line 3"),
	}

	// 测试正常情况
	result := source(lines, 1)
	if string(result) != "line 1" {
		t.Errorf("expected 'line 1', got '%s'", string(result))
	}

	// 测试带空格的行
	result = source(lines, 2)
	if string(result) != "line 2" {
		t.Errorf("expected 'line 2', got '%s'", string(result))
	}

	// 测试越界情况
	result = source(lines, 0)
	if string(result) != "???" {
		t.Errorf("expected '???', got '%s'", string(result))
	}

	result = source(lines, 10)
	if string(result) != "???" {
		t.Errorf("expected '???', got '%s'", string(result))
	}
}

func TestFunctionName(t *testing.T) {
	// 获取当前函数的 PC
	result := function(0)
	// 即使 PC 为 0，也应该返回 dunno
	if result == nil {
		t.Error("should not return nil")
	}
}

func TestGinXRecoverMiddlewarePanicWithError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(GinXRecoverMiddleware(nil))

	r.GET("/panic-error", func(c *gin.Context) {
		panic(http.ErrAbortHandler)
	})

	req := httptest.NewRequest("GET", "/panic-error", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected status 500, got %d", w.Code)
	}
}
