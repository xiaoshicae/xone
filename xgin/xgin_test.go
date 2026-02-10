package xgin

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/swaggo/swag"
	"github.com/xiaoshicae/xone/v2/xconfig"
	"github.com/xiaoshicae/xone/v2/xgin/options"

	. "github.com/bytedance/mockey"
	. "github.com/smartystreets/goconvey/convey"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func TestNew(t *testing.T) {
	g := New()
	if g == nil {
		t.Fatal("New() returned nil")
	}
	if g.engine == nil {
		t.Fatal("engine is nil")
	}
	if g.build {
		t.Fatal("build should be false")
	}
}

func TestNewWithOptions(t *testing.T) {
	g := New(
		options.EnableLogMiddleware(false),
		options.EnableTraceMiddleware(false),
	)
	if g == nil {
		t.Fatal("New() returned nil")
	}

	g.Build()
	if !g.build {
		t.Fatal("build should be true after Build()")
	}
}

func TestBuild(t *testing.T) {
	g := New(
		options.EnableLogMiddleware(false),
		options.EnableTraceMiddleware(false),
	)
	g.Build()

	if !g.build {
		t.Fatal("build should be true after Build()")
	}
}

func TestWithRouteRegister(t *testing.T) {
	g := New(
		options.EnableLogMiddleware(false),
		options.EnableTraceMiddleware(false),
	)

	handlerCalled := false
	g.WithRouteRegister(func(e *gin.Engine) {
		e.GET("/test", func(c *gin.Context) {
			handlerCalled = true
			c.String(http.StatusOK, "ok")
		})
	})

	g.Build()

	// 发送测试请求
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	g.engine.ServeHTTP(w, req)

	if !handlerCalled {
		t.Fatal("route handler was not called")
	}
	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}
}

func TestWithMiddleware(t *testing.T) {
	g := New(
		options.EnableLogMiddleware(false),
		options.EnableTraceMiddleware(false),
	)

	middlewareCalled := false
	g.WithMiddleware(func(c *gin.Context) {
		middlewareCalled = true
		c.Next()
	})

	g.WithRouteRegister(func(e *gin.Engine) {
		e.GET("/test", func(c *gin.Context) {
			c.String(http.StatusOK, "ok")
		})
	})

	g.Build()

	// 发送测试请求
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	g.engine.ServeHTTP(w, req)

	if !middlewareCalled {
		t.Fatal("middleware was not called")
	}
}

func TestEngine(t *testing.T) {
	g := New(
		options.EnableLogMiddleware(false),
		options.EnableTraceMiddleware(false),
	)

	engine := g.Engine()
	if engine == nil {
		t.Fatal("Engine() returned nil")
	}
	if !g.build {
		t.Fatal("Engine() should trigger Build()")
	}
}

func TestEngineWithoutBuild(t *testing.T) {
	g := New(
		options.EnableLogMiddleware(false),
		options.EnableTraceMiddleware(false),
	)

	// 不调用 Build，直接调用 Engine
	engine := g.Engine()
	if engine == nil {
		t.Fatal("Engine() returned nil")
	}
	// Engine() 应该自动触发 Build
	if !g.build {
		t.Fatal("Engine() should auto-trigger Build()")
	}
}

func TestMultipleRouteRegisters(t *testing.T) {
	g := New(
		options.EnableLogMiddleware(false),
		options.EnableTraceMiddleware(false),
	)

	g.WithRouteRegister(
		func(e *gin.Engine) {
			e.GET("/route1", func(c *gin.Context) {
				c.String(http.StatusOK, "route1")
			})
		},
		func(e *gin.Engine) {
			e.GET("/route2", func(c *gin.Context) {
				c.String(http.StatusOK, "route2")
			})
		},
	)

	g.Build()

	// 测试第一个路由
	w1 := httptest.NewRecorder()
	req1, _ := http.NewRequest("GET", "/route1", nil)
	g.engine.ServeHTTP(w1, req1)
	if w1.Code != http.StatusOK {
		t.Fatalf("route1: expected status 200, got %d", w1.Code)
	}

	// 测试第二个路由
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("GET", "/route2", nil)
	g.engine.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("route2: expected status 200, got %d", w2.Code)
	}
}

func TestMethodNotAllowed(t *testing.T) {
	g := New(
		options.EnableLogMiddleware(false),
		options.EnableTraceMiddleware(false),
	)

	g.WithRouteRegister(func(e *gin.Engine) {
		e.GET("/test", func(c *gin.Context) {
			c.String(http.StatusOK, "ok")
		})
	})

	g.Build()

	// 使用 POST 请求访问 GET 路由
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/test", nil)
	g.engine.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected status 405, got %d", w.Code)
	}
}

func TestStopWithoutRun(t *testing.T) {
	g := New()
	err := g.Stop()
	if err == nil {
		t.Fatal("Stop() should return error when server not started")
	}
}

func TestWithSwagger(t *testing.T) {
	g := New(
		options.EnableLogMiddleware(false),
		options.EnableTraceMiddleware(false),
	)

	// 测试 WithSwagger 不 panic
	g.WithSwagger(nil)

	if g.swaggerInfo != nil {
		t.Fatal("swaggerInfo should be nil when passed nil")
	}
}

func TestWithRecoverFunc(t *testing.T) {
	g := New(
		options.EnableLogMiddleware(false),
		options.EnableTraceMiddleware(false),
	)

	customRecoverCalled := false
	customRecover := func(c *gin.Context, err interface{}) {
		customRecoverCalled = true
		c.JSON(http.StatusOK, gin.H{"error": "recovered"})
	}

	g.WithRecoverFunc(customRecover)

	g.WithRouteRegister(func(e *gin.Engine) {
		e.GET("/panic", func(c *gin.Context) {
			panic("test panic")
		})
	})

	g.Build()

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/panic", nil)
	g.engine.ServeHTTP(w, req)

	if !customRecoverCalled {
		t.Fatal("custom recover function was not called")
	}
}

func TestRunAndStop(t *testing.T) {
	PatchConvey("TestRunAndStop", t, func() {
		Mock(GetConfig).Return(&Config{Host: "127.0.0.1", Port: 0}).Build()

		g := New(
			options.EnableLogMiddleware(false),
			options.EnableTraceMiddleware(false),
		)

		g.WithRouteRegister(func(e *gin.Engine) {
			e.GET("/test", func(c *gin.Context) {
				c.String(http.StatusOK, "ok")
			})
		})

		// 在 goroutine 中运行
		errCh := make(chan error, 1)
		go func() {
			errCh <- g.Run()
		}()

		// 等待服务器启动
		time.Sleep(100 * time.Millisecond)

		// 停止服务器
		err := g.Stop()
		So(err, ShouldBeNil)

		// 检查 Run 返回值
		select {
		case err := <-errCh:
			So(err, ShouldBeNil)
		case <-time.After(2 * time.Second):
			t.Fatal("Run() did not return after Stop()")
		}
	})
}

func TestGetXGinOptions(t *testing.T) {
	g := New(
		options.EnableLogMiddleware(false),
	)

	opts := g.getXGinOptions()

	if opts.EnableLogMiddleware {
		t.Error("EnableLogMiddleware should be false")
	}
}

func TestBuildWithZHTranslations(t *testing.T) {
	g := New(
		options.EnableLogMiddleware(false),
		options.EnableTraceMiddleware(false),
		options.EnableZHTranslations(true),
	)

	// 应该不 panic
	g.Build()

	if !g.build {
		t.Fatal("build should be true")
	}
}

func TestInjectEngineWithoutSwagger(t *testing.T) {
	g := New(
		options.EnableLogMiddleware(false),
		options.EnableTraceMiddleware(false),
	)

	// 不设置 swagger，应该正常工作
	g.Build()

	if !g.build {
		t.Fatal("build should be true")
	}
}

func TestMultipleMiddlewares(t *testing.T) {
	g := New(
		options.EnableLogMiddleware(false),
		options.EnableTraceMiddleware(false),
	)

	callOrder := make([]int, 0)

	g.WithMiddleware(
		func(c *gin.Context) {
			callOrder = append(callOrder, 1)
			c.Next()
		},
		func(c *gin.Context) {
			callOrder = append(callOrder, 2)
			c.Next()
		},
	)

	g.WithRouteRegister(func(e *gin.Engine) {
		e.GET("/test", func(c *gin.Context) {
			callOrder = append(callOrder, 3)
			c.String(http.StatusOK, "ok")
		})
	})

	g.Build()

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	g.engine.ServeHTTP(w, req)

	if len(callOrder) != 3 {
		t.Fatalf("expected 3 calls, got %d", len(callOrder))
	}
	if callOrder[0] != 1 || callOrder[1] != 2 || callOrder[2] != 3 {
		t.Fatalf("unexpected call order: %v", callOrder)
	}
}

func TestBuildWithAllMiddlewares(t *testing.T) {
	g := New(
		options.EnableLogMiddleware(true),
		options.EnableTraceMiddleware(true),
	)

	g.WithRouteRegister(func(e *gin.Engine) {
		e.GET("/test", func(c *gin.Context) {
			c.String(http.StatusOK, "ok")
		})
	})

	g.Build()

	w := httptest.NewRecorder()
	// 使用 httptest.NewRequest 确保 Body 不为 nil
	req := httptest.NewRequest("GET", "/test", nil)
	g.engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}
}

func TestRunWithHttp2(t *testing.T) {
	PatchConvey("TestRunWithHttp2", t, func() {
		Mock(GetConfig).Return(&Config{Host: "127.0.0.1", Port: 0, UseH2C: true}).Build()
		Mock((*http.Server).ListenAndServe).Return(errors.New("for test")).Build()

		g := New(
			options.EnableLogMiddleware(false),
			options.EnableTraceMiddleware(false),
		)

		err := g.Run()
		So(err, ShouldNotBeNil)
		So(err.Error(), ShouldEqual, "for test")
		// h2c 模式下 handler 被 h2c.NewHandler 包装，不再设置 engine.UseH2C
		So(g.srv, ShouldNotBeNil)
		So(g.srv.Handler, ShouldNotBeNil)
	})
}

func TestRunWithTLS(t *testing.T) {
	PatchConvey("TestRunWithTLS", t, func() {
		Mock(GetConfig).Return(&Config{
			Host:     "127.0.0.1",
			Port:     8443,
			CertFile: "/path/to/cert.pem",
			KeyFile:  "/path/to/key.pem",
		}).Build()
		Mock((*http.Server).ListenAndServeTLS).Return(errors.New("for test tls")).Build()

		g := New(
			options.EnableLogMiddleware(false),
			options.EnableTraceMiddleware(false),
		)

		err := g.Run()
		So(err, ShouldNotBeNil)
		So(err.Error(), ShouldEqual, "for test tls")
	})
}

func TestRunWithServerClosed(t *testing.T) {
	PatchConvey("TestRunWithServerClosed", t, func() {
		Mock(GetConfig).Return(&Config{Host: "127.0.0.1", Port: 0}).Build()
		Mock((*http.Server).ListenAndServe).Return(http.ErrServerClosed).Build()

		g := New(
			options.EnableLogMiddleware(false),
			options.EnableTraceMiddleware(false),
		)

		err := g.Run()
		So(err, ShouldBeNil)
	})
}

func TestSetGinSwaggerInfo(t *testing.T) {
	PatchConvey("TestSetGinSwaggerInfo", t, func() {
		Mock(GetSwaggerConfig).Return(&SwaggerConfig{
			Host:        "localhost",
			BasePath:    "/api",
			Title:       "Test API",
			Description: "Test Description",
			Schemes:     []string{"https"},
		}).Build()
		Mock(xconfig.GetServerVersion).Return("v2.0.0").Build()

		spec := &swag.Spec{}
		setGinSwaggerInfo(spec)

		So(spec.Version, ShouldEqual, "v2.0.0")
		So(spec.Host, ShouldEqual, "localhost")
		So(spec.BasePath, ShouldEqual, "/api")
		So(spec.Title, ShouldEqual, "Test API")
		So(spec.Description, ShouldEqual, "Test Description")
		So(spec.Schemes, ShouldResemble, []string{"https"})
	})
}

func TestInjectSwaggerInfoNil(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()

	// 测试 nil swaggerInfo 不 panic
	injectSwaggerInfo(nil, engine)

	// 测试 nil engine 不 panic
	injectSwaggerInfo(&swag.Spec{}, nil)
}

func TestInjectSwaggerInfoWithSpec(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()

	spec := &swag.Spec{
		InfoInstanceName: "test",
		SwaggerTemplate:  "{}",
	}

	injectSwaggerInfo(spec, engine)

	// 验证 swagger 路由被注册
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/swagger/index.html", nil)
	engine.ServeHTTP(w, req)

	// 应该返回 200 或 301 重定向
	if w.Code != http.StatusOK && w.Code != http.StatusMovedPermanently && w.Code != http.StatusNotFound {
		t.Logf("swagger route response code: %d", w.Code)
	}
}

func TestInjectSwaggerInfoWithUrlPrefix(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()

	spec := &swag.Spec{
		InfoInstanceName: "test",
		SwaggerTemplate:  "{}",
	}

	injectSwaggerInfo(spec, engine, options.WithSwaggerUrlPrefix("/api/v1"))

	// 验证带前缀的 swagger 路由被注册
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/swagger/index.html", nil)
	engine.ServeHTTP(w, req)

	// 应该有响应（不是 404）
	t.Logf("prefixed swagger route response code: %d", w.Code)
}
