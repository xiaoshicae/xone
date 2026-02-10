package xgin

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/xiaoshicae/xone/v2/xgin/middleware"
	"github.com/xiaoshicae/xone/v2/xgin/options"
	"github.com/xiaoshicae/xone/v2/xgin/trans"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/swaggo/swag"
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

	addr  string       // 服务监听地址(host+port)
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

	// 向*gin.Engine注入一些额外内容
	g.injectEngine()

	// 注册中文翻译器
	if ginXOptions.EnableZHTranslations {
		if err := trans.RegisterZHTranslations(); err != nil {
			logrus.Warnf("register zh translations failed: %v", err)
		}
	}

	g.addr = ginXOptions.Addr
	g.build = true
	return g
}

func (g *XGin) Engine() *gin.Engine {
	if !g.build {
		g.Build()
	}
	return g.engine
}

// Run 实现 xserver.Server 接口
func (g *XGin) Run() error {
	if !g.build {
		g.Build()
	}

	PrintBanner()

	g.srv = &http.Server{
		Addr:    g.addr,
		Handler: g.engine.Handler(),
	}

	err := g.srv.ListenAndServe()
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
	// 提前注入一下session相关信息
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

	// TODO: metrics middleware 待补充

	// 注册自定义middleware
	for _, m := range g.middlewares {
		g.engine.Use(m)
	}
}

func (g *XGin) registerRoute() {
	for _, register := range g.routerRegisters {
		register(g.engine)
	}
}

func (g *XGin) injectEngine() {
	// 向gin.Engine注入swagger信息
	injectSwaggerInfo(g.swaggerInfo, g.engine, g.swaggerOpts...)

	// 注入banner打印
	injectPrintBanner(g.engine)
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
