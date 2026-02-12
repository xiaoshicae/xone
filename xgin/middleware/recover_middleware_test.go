package middleware

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
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

func TestGinXRecoverMiddlewareBrokenPipe(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(GinXRecoverMiddleware(nil))

	r.GET("/broken-pipe", func(c *gin.Context) {
		// 模拟 broken pipe 错误
		panic(&net.OpError{
			Op:  "write",
			Net: "tcp",
			Err: &os.SyscallError{
				Syscall: "write",
				Err:     fmt.Errorf("broken pipe"),
			},
		})
	})

	req := httptest.NewRequest("GET", "/broken-pipe", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// brokenPipe 时直接 abort，不写 response
	// status 默认 200 因为 httptest.NewRecorder 默认值
}

func TestGinXRecoverMiddlewareBrokenPipeNonError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(GinXRecoverMiddleware(nil))

	r.GET("/broken-pipe-str", func(c *gin.Context) {
		// 模拟 broken pipe 但 panic 值不是 error 类型的 OpError
		// 这里我们直接 panic 一个 *net.OpError，它本身实现了 error 接口
		// 所以走 brokenPipe=true, err.(error) 成功的分支
		panic(&net.OpError{
			Op:  "write",
			Net: "tcp",
			Err: &os.SyscallError{
				Syscall: "write",
				Err:     fmt.Errorf("connection reset by peer"),
			},
		})
	})

	req := httptest.NewRequest("GET", "/broken-pipe-str", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
}
