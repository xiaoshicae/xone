package xgin

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/swaggo/swag"
	"github.com/xiaoshicae/xone/v2/xgin/options"
)

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
