package xgin

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/xiaoshicae/xone/v2/xconfig"
	"github.com/xiaoshicae/xone/v2/xserver"
	"github.com/xiaoshicae/xone/v2/xutil"

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

// ginServer 基于 gin.Engine 的 HTTP 服务器
type ginServer struct {
	engine *gin.Engine
	srv    *http.Server
}

// NewServer 从 gin.Engine 创建 Server
func NewServer(engine *gin.Engine) xserver.Server {
	return &ginServer{engine: engine}
}

func (s *ginServer) Run() error {
	ginConfig := GetConfig()
	if ginConfig.UseHttp2 {
		s.engine.UseH2C = true
		xutil.InfoIfEnableDebug("gin server use http2")
	}

	addr := net.JoinHostPort(ginConfig.Host, strconv.Itoa(ginConfig.Port))
	xutil.InfoIfEnableDebug("gin server listen at: %s", addr)

	invokeEngineInjectFunc(s.engine)

	s.srv = &http.Server{
		Addr:    addr,
		Handler: s.engine.Handler(),
	}
	return s.srv.ListenAndServe()
}

func (s *ginServer) Stop() error {
	if s.srv == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), defaultWaitStopDuration)
	defer cancel()

	if err := s.srv.Shutdown(ctx); err != nil {
		return fmt.Errorf("gin server stop failed, err=[%v]", err)
	}
	return nil
}

// ginTLSServer 支持 HTTPS 的 Gin 服务器
type ginTLSServer struct {
	engine   *gin.Engine
	srv      *http.Server
	certFile string
	keyFile  string
}

// NewTLSServer 从 gin.Engine 创建 TLS Server
func NewTLSServer(engine *gin.Engine, certFile, keyFile string) xserver.Server {
	return &ginTLSServer{engine: engine, certFile: certFile, keyFile: keyFile}
}

func (s *ginTLSServer) Run() error {
	ginConfig := GetConfig()
	if ginConfig.UseHttp2 {
		s.engine.UseH2C = true
		xutil.InfoIfEnableDebug("gin server use http2")
	}

	addr := net.JoinHostPort(ginConfig.Host, strconv.Itoa(ginConfig.Port))
	xutil.InfoIfEnableDebug("gin server listen at: %s (TLS)", addr)

	invokeEngineInjectFunc(s.engine)

	s.srv = &http.Server{
		Addr:    addr,
		Handler: s.engine.Handler(),
	}
	return s.srv.ListenAndServeTLS(s.certFile, s.keyFile)
}

func (s *ginTLSServer) Stop() error {
	if s.srv == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), defaultWaitStopDuration)
	defer cancel()

	if err := s.srv.Shutdown(ctx); err != nil {
		return fmt.Errorf("gin server stop failed, err=[%v]", err)
	}
	return nil
}

// Run 便捷启动（创建 ginServer + 调用 xserver.Run）
func Run(engine *gin.Engine) error {
	return xserver.Run(NewServer(engine))
}

// RunTLS 便捷启动 HTTPS
func RunTLS(engine *gin.Engine, certFile, keyFile string) error {
	return xserver.Run(NewTLSServer(engine, certFile, keyFile))
}

func invokeEngineInjectFunc(engine *gin.Engine) {
	if f := engine.FuncMap[SwaggerInfoFuncKey]; f != nil {
		if ff, ok := f.(func() *swag.Spec); ok {
			if swaggerInfo := ff(); swaggerInfo != nil {
				setGinSwaggerInfo(swaggerInfo)
			}
		}
	}

	// 打印 Banner：优先使用 FuncMap 注入的函数，否则直接调用
	if f := engine.FuncMap[PrintBannerFuncKey]; f != nil {
		if ff, ok := f.(func()); ok {
			ff()
			return
		}
	}
	PrintBanner()
}

func setGinSwaggerInfo(swaggerInfo *swag.Spec) {
	ginSwaggerConfig := GetSwaggerConfig()
	swaggerInfo.Version = xconfig.GetServerVersion()
	swaggerInfo.Host = ginSwaggerConfig.Host
	swaggerInfo.BasePath = ginSwaggerConfig.BasePath
	swaggerInfo.Title = ginSwaggerConfig.Title
	swaggerInfo.Description = ginSwaggerConfig.Description
	swaggerInfo.Schemes = ginSwaggerConfig.Schemes
}
