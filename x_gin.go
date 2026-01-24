package xone

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/xiaoshicae/xone/xconfig"
	"github.com/xiaoshicae/xone/xutil"

	"github.com/gin-gonic/gin"
	"github.com/swaggo/swag"
)

const (
	defaultWaitStopDuration = 30 * time.Second

	// SwaggerInfoFuncKey 用于在 gin.Engine.FuncMap 中注入 Swagger 信息获取函数
	// 函数签名: func() *swag.Spec
	SwaggerInfoFuncKey = "__swagger_info__func__"

	// PrintBannerFuncKey 用于在 gin.Engine.FuncMap 中注入启动 Banner 打印函数
	// 函数签名: func()
	PrintBannerFuncKey = "__print_banner__func__"
)

type ginServer struct {
	srv *http.Server
}

func newGinServer(engine *gin.Engine) *ginServer {
	ginConfig := xconfig.GetGinConfig()
	if ginConfig.UseHttp2 {
		engine.UseH2C = true
		xutil.InfoIfEnableDebug("gin server use http2")
	}

	addr := net.JoinHostPort(ginConfig.Host, strconv.Itoa(ginConfig.Port))
	xutil.InfoIfEnableDebug("gin server listen at: %s", addr)

	invokeEngineInjectFunc(engine)

	// 包装一下engine，为后续Run()和Stop()作准备
	srv := &http.Server{
		Addr:    addr,
		Handler: engine.Handler(),
	}
	return &ginServer{srv: srv}
}

func (s *ginServer) Run() error {
	return s.srv.ListenAndServe()
}

func (s *ginServer) Stop() error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultWaitStopDuration)
	defer cancel()

	if err := s.srv.Shutdown(ctx); err != nil {
		return fmt.Errorf("gin server stop failed, err=[%v]", err)
	}
	return nil
}

func invokeEngineInjectFunc(engine *gin.Engine) {
	if f := engine.FuncMap[SwaggerInfoFuncKey]; f != nil {
		if ff, ok := f.(func() *swag.Spec); ok {
			if swaggerInfo := ff(); swaggerInfo != nil {
				setGinSwaggerInfo(swaggerInfo)
			}
		}
	}

	if f := engine.FuncMap[PrintBannerFuncKey]; f != nil {
		if ff, ok := f.(func()); ok {
			ff()
		}
	}
}

func setGinSwaggerInfo(swaggerInfo *swag.Spec) {
	ginSwaggerConfig := xconfig.GetGinSwaggerConfig()
	swaggerInfo.Version = xconfig.GetServerVersion()
	swaggerInfo.Host = ginSwaggerConfig.Host
	swaggerInfo.BasePath = ginSwaggerConfig.BasePath
	swaggerInfo.Title = ginSwaggerConfig.Title
	swaggerInfo.Description = ginSwaggerConfig.Description
	swaggerInfo.Schemes = ginSwaggerConfig.Schemes
}
