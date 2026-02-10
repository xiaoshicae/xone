package xgin

import (
	"github.com/gin-gonic/gin"
	"github.com/swaggo/swag"

	"github.com/xiaoshicae/xone/xgin/options"
	"github.com/xiaoshicae/xone/xgin/swagger"
)

func injectSwaggerInfo(swaggerInfo *swag.Spec, engine *gin.Engine, opts ...options.SwaggerOption) {
	if swaggerInfo == nil || engine == nil {
		return
	}

	engine.FuncMap[SwaggerInfoFuncKey] = func() *swag.Spec { return swaggerInfo }

	dso := options.DefaultSwaggerOptions()
	for _, opt := range opts {
		opt(dso)
	}

	swaggerUrl := swagger.SwaggerUrl
	if dso.UrlPrefix != "" {
		swaggerUrl = dso.UrlPrefix + swaggerUrl
	}

	engine.GET(swaggerUrl, swagger.SwaggerHandler)
}

func injectPrintBanner(engine *gin.Engine) {
	engine.FuncMap[PrintBannerFuncKey] = PrintBanner
}
