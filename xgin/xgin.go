package xgin

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/xiaoshicae/xone/v2/xconfig"
	"github.com/xiaoshicae/xone/v2/xgin/middleware"
	"github.com/xiaoshicae/xone/v2/xgin/options"
	"github.com/xiaoshicae/xone/v2/xgin/swagger"
	"github.com/xiaoshicae/xone/v2/xgin/trans"
	"github.com/xiaoshicae/xone/v2/xserver"
	"github.com/xiaoshicae/xone/v2/xutil"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/swaggo/swag"
)

const (
	defaultWaitStopDuration = 30 * time.Second
)

// New 创建 XGin builder
func New(opts ...options.Option) *XGin {
	setGinMode()
	engine := gin.New()
	engine.HandleMethodNotAllowed = true // 允许处理405
	return &XGin{
		engine:          engine,
		opts:            opts,
		routerRegisters: make([]func(*gin.Engine), 0),
		middlewares:     make([]gin.HandlerFunc, 0),
		recoveryFunc:    nil,
		swaggerInfo:     nil,
		swaggerOpts:     make([]options.SwaggerOption, 0),
		build:           false,
	}
}

// XGin Gin Web 框架集成
type XGin struct {
	engine          *gin.Engine
	opts            []options.Option
	routerRegisters []func(*gin.Engine)
	middlewares     []gin.HandlerFunc
	recoveryFunc    gin.RecoveryFunc
	swaggerInfo     *swag.Spec
	swaggerOpts     []options.SwaggerOption

	srv   *http.Server // 对gin进行包装后的http server
	build bool         // XGin实例是否已经build完成
}

func (g *XGin) WithRouteRegister(f ...func(*gin.Engine)) *XGin {
	g.routerRegisters = append(g.routerRegisters, f...)
	return g
}

func (g *XGin) WithMiddleware(m ...gin.HandlerFunc) *XGin {
	g.middlewares = append(g.middlewares, m...)
	return g
}

func (g *XGin) WithSwagger(swaggerInfo *swag.Spec, opts ...options.SwaggerOption) *XGin {
	g.swaggerInfo = swaggerInfo
	g.swaggerOpts = opts
	return g
}

func (g *XGin) WithRecoverFunc(recoveryFunc gin.RecoveryFunc) *XGin {
	g.recoveryFunc = recoveryFunc
	return g
}

func (g *XGin) Build() *XGin {
	ginXOptions := g.getXGinOptions()

	// 注册middleware
	g.registerMiddleware(ginXOptions)

	// 注册路由
	g.registerRoute()

	// 向*gin.Engine注入swaggar配置
	if g.swaggerInfo != nil {
		injectSwaggerInfo(g.swaggerInfo, g.engine, g.swaggerOpts...)
	}

	// 注册中文翻译器
	if ginXOptions.EnableZHTranslations {
		if err := trans.RegisterZHTranslations(); err != nil {
			logrus.Warnf("register zh translations failed: %v", err)
		}
	}

	g.build = true
	return g
}

func (g *XGin) Engine() *gin.Engine {
	if !g.build {
		g.Build()
	}
	return g.engine
}

// Start 提供快捷启动方式
func (g *XGin) Start() error {
	return xserver.Run(g)
}

// Run 实现 xserver.Server 接口
func (g *XGin) Run() error {
	if !g.build {
		g.Build()
	}

	// 从 xconfig 读取配置（此时 xconfig 已通过 BeforeStart hook 初始化）
	ginConfig := GetConfig()

	// 启用 HTTP/2
	if ginConfig.UseHttp2 {
		g.engine.UseH2C = true
		xutil.InfoIfEnableDebug("gin server use http2")
	}

	// 填充 swagger 配置
	if g.swaggerInfo != nil {
		setGinSwaggerInfo(g.swaggerInfo)
	}

	addr := net.JoinHostPort(ginConfig.Host, strconv.Itoa(ginConfig.Port))

	PrintBanner()

	xutil.InfoIfEnableDebug("gin server listen on: %s", addr)

	g.srv = &http.Server{
		Addr:    addr,
		Handler: g.engine.Handler(),
	}

	// 根据 TLS 配置决定启动方式
	var err error
	if ginConfig.CertFile != "" && ginConfig.KeyFile != "" {
		xutil.InfoIfEnableDebug("gin server use TLS, cert=[%s], key=[%s]", ginConfig.CertFile, ginConfig.KeyFile)
		err = g.srv.ListenAndServeTLS(ginConfig.CertFile, ginConfig.KeyFile)
	} else {
		err = g.srv.ListenAndServe()
	}

	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

// Stop 实现 xserver.Server 接口
func (g *XGin) Stop() error {
	if g.srv == nil {
		return fmt.Errorf("server not started")
	}

	ctx, cancel := context.WithTimeout(context.Background(), defaultWaitStopDuration)
	defer cancel()

	if err := g.srv.Shutdown(ctx); err != nil {
		logrus.WithContext(context.Background()).Errorf("XGin server stop failed, err:%v", err)
		return err
	}
	return nil
}

func (g *XGin) getXGinOptions() *options.Options {
	do := options.DefaultOptions()
	for _, opt := range g.opts {
		opt(do)
	}
	return do
}

func (g *XGin) registerMiddleware(do *options.Options) {
	// 提前注入一下 session 相关信息
	g.engine.Use(middleware.GinXSessionMiddleware())

	// 注册trace middleware，需要放在靠前的位置，保证traceid能提前生成，后续middleware和handler能正确获取到
	if do.EnableTraceMiddleware {
		g.engine.Use(middleware.GinXTraceMiddleware())
	}

	// 注册recover middleware，需要放在除trace外其它middleware前，保证发生panic能及时recover
	g.engine.Use(middleware.GinXRecoverMiddleware(g.recoveryFunc))

	// 注册log middleware
	if do.EnableLogMiddleware {
		g.engine.Use(middleware.LogMiddleware(middleware.WithSkipPaths(do.LogSkipPaths...)))
	}

	// 注册自定义的 middleware
	for _, m := range g.middlewares {
		g.engine.Use(m)
	}
}

func (g *XGin) registerRoute() {
	for _, register := range g.routerRegisters {
		register(g.engine)
	}
}

func setGinMode() {
	if strings.TrimSpace(os.Getenv(gin.EnvGinMode)) != "" {
		return
	}
	switch strings.ToLower(strings.TrimSpace(os.Getenv("SERVER_ENABLE_DEBUG"))) {
	case "true", "1", "t", "yes", "y", "on":
		gin.SetMode(gin.DebugMode)
	default:
		gin.SetMode(gin.ReleaseMode)
	}
}

func injectSwaggerInfo(swaggerInfo *swag.Spec, engine *gin.Engine, opts ...options.SwaggerOption) {
	if swaggerInfo == nil || engine == nil {
		return
	}

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

func setGinSwaggerInfo(swaggerInfo *swag.Spec) {
	ginSwaggerConfig := GetSwaggerConfig()
	swaggerInfo.Version = xconfig.GetServerVersion()
	swaggerInfo.Host = ginSwaggerConfig.Host
	swaggerInfo.BasePath = ginSwaggerConfig.BasePath
	swaggerInfo.Title = ginSwaggerConfig.Title
	swaggerInfo.Description = ginSwaggerConfig.Description
	swaggerInfo.Schemes = ginSwaggerConfig.Schemes
}
