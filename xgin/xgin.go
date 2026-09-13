package xgin

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/xiaoshicae/xone/v2/xconfig"
	"github.com/xiaoshicae/xone/v2/xerror"
	"github.com/xiaoshicae/xone/v2/xgin/middleware"
	"github.com/xiaoshicae/xone/v2/xgin/options"
	"github.com/xiaoshicae/xone/v2/xgin/swagger"
	"github.com/xiaoshicae/xone/v2/xgin/trans"
	"github.com/xiaoshicae/xone/v2/xhook"
	"github.com/xiaoshicae/xone/v2/xserver"
	"github.com/xiaoshicae/xone/v2/xutil"

	"github.com/gin-gonic/gin"
	"github.com/swaggo/swag"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
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

	buildMu sync.Mutex // 保护 build 与 engine 的中间件/路由注册
	build   bool       // XGin实例是否已经build完成

	srvMu       sync.Mutex    // 保护 srv / stopped / stopTimeout 的并发访问
	srv         *http.Server  // 对gin进行包装后的http server
	stopped     bool          // Stop() 是否已被调用
	stopTimeout time.Duration // 优雅退出超时，Run 时从配置读入
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
	// Build 会向 engine 注册中间件和路由，并发调用会重复注册：
	// 同一个中间件被 Use 多次，意味着每个请求打多条重复日志、指标被重复计数
	g.buildMu.Lock()
	defer g.buildMu.Unlock()

	if g.build {
		return g
	}

	ginXOptions := g.getXGinOptions()

	// 注册middleware
	g.registerMiddleware(ginXOptions)

	// 注册路由
	g.registerRoute()

	// 向*gin.Engine注入swagger配置
	if g.swaggerInfo != nil {
		injectSwaggerInfo(g.swaggerInfo, g.engine, g.swaggerOpts...)
	}

	// 注册中文翻译器
	if ginXOptions.EnableZHTranslations {
		if err := trans.RegisterZHTranslations(); err != nil {
			xutil.WarnIfEnableDebug("register zh translations failed: %v", err)
		}
	}

	g.build = true
	return g
}

func (g *XGin) Engine() *gin.Engine {
	g.Build() // Build 内部已做幂等与加锁
	return g.engine
}

// Start 启动服务：执行 BeforeStart Hook，运行服务，等待退出信号后执行 BeforeStop Hook
//
// 这是启动 XGin 的唯一方式。服务本身的启停（xserver.Server 接口）由内部的
// ginServer 实现，不对外暴露：它不跑任何 Hook，只有在初始化完成之后调用才成立，
// 而它曾经叫 Run——与 Start 是英文同义词，选错的代价却是隐形的，
// 服务会带着一份空配置起在默认端口上，日志、链路、数据库客户端一个都没配置。
func (g *XGin) Start() error {
	return xserver.Run(&ginServer{g: g})
}

// ginServer 把 XGin 适配为 xserver.Server
//
// 独立成一个不导出的类型，而不是直接在 XGin 上挂 Run/Stop：
// 后者会让「跳过初始化直接起服务」重新变成一次方法调用的距离。
type ginServer struct {
	g *XGin
}

func (s *ginServer) Run() error  { return s.g.run() }
func (s *ginServer) Stop() error { return s.g.stop() }

// run 启动 http server，由 xserver 在 BeforeStart Hook 之后调用
func (g *XGin) run() error {
	// 前置条件断言。公开 API 上已经没有入口能绕过初始化，
	// 这里兜住的是日后新增调用路径时的疏漏——代价只是一次原子读
	if !xhook.BeforeStartInvoked() {
		return xerror.Newf("xgin", "run", "server started before BeforeStart hooks were invoked, use Start()")
	}

	g.Build() // Build 内部已做幂等与加锁

	// 从 xconfig 读取配置（此时 xconfig 已通过 BeforeStart hook 初始化）
	// 启动路径上配置解析失败必须直接失败：静默回退默认端口会让服务起在
	// 一个没人预期的端口上，而排查时配置文件看着是对的
	ginConfig, err := getConfig()
	if err != nil {
		return xerror.Newf("xgin", "run", "get config failed, err=[%v]", err)
	}

	// 校验 TLS 配置完整性
	if (ginConfig.CertFile == "") != (ginConfig.KeyFile == "") {
		return xerror.Newf("xgin", "run", "TLS config incomplete: CertFile and KeyFile must be both set or both empty")
	}

	// 填充 swagger 配置
	if g.swaggerInfo != nil {
		setGinSwaggerInfo(g.swaggerInfo)
	}

	addr := net.JoinHostPort(ginConfig.Host, strconv.Itoa(ginConfig.Port))

	PrintBanner()

	xutil.InfoIfEnableDebug("gin server listen on: %s", addr)

	srv := g.newHTTPServer(ginConfig, addr)

	g.srvMu.Lock()
	if g.stopped {
		g.srvMu.Unlock()
		// 退出信号早于此处到达。若照常 ListenAndServe，服务会在"已收到停止信号"
		// 之后才起来，并一直运行到 xserver 等待超时为止
		xutil.WarnIfEnableDebug("XGin Run called after Stop, server will not start")
		return nil
	}
	if g.srv != nil {
		g.srvMu.Unlock()
		return xerror.Newf("xgin", "run", "server already running, addr=[%s]", g.srv.Addr)
	}
	g.srv = srv
	g.stopTimeout = xutil.ToDuration(ginConfig.GracefulStopTimeout)
	g.srvMu.Unlock()

	// 根据 TLS 配置决定启动方式
	if ginConfig.CertFile != "" && ginConfig.KeyFile != "" {
		xutil.InfoIfEnableDebug("gin server use TLS, cert=[%s], key=[%s]", ginConfig.CertFile, ginConfig.KeyFile)
		err = srv.ListenAndServeTLS(ginConfig.CertFile, ginConfig.KeyFile)
	} else {
		err = srv.ListenAndServe()
	}

	if err == nil || errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return xerror.New("xgin", "run", err)
}

// stop 优雅关闭 http server，由 xserver 在收到退出信号后调用
func (g *XGin) stop() error {
	g.srvMu.Lock()
	g.stopped = true // 先置标志：Run 若尚未开始监听，到达时会直接返回
	srv := g.srv
	timeout := g.stopTimeout
	g.srvMu.Unlock()

	if srv == nil {
		// 信号在 srv 赋值前到达。stopped 已置位，服务不会再起来
		xutil.WarnIfEnableDebug("XGin Stop called before server started, server will not start")
		return nil
	}

	if timeout <= 0 {
		timeout = xutil.ToDuration(defaultGracefulStopTimeout)
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		xutil.ErrorIfEnableDebug("XGin server stop failed, err=[%v]", err)
		return xerror.New("xgin", "stop", err)
	}
	return nil
}

// newHTTPServer 按配置构建 http.Server
func (g *XGin) newHTTPServer(c *Config, addr string) *http.Server {
	// 构建 handler，根据配置决定是否启用 h2c
	handler := g.engine.Handler()
	if c.UseH2C && c.CertFile == "" && c.KeyFile == "" {
		// 非 TLS 模式下使用 h2c（HTTP/2 Cleartext）
		handler = h2c.NewHandler(handler, &http2.Server{})
		xutil.InfoIfEnableDebug("gin server use h2c (HTTP/2 Cleartext)")
	}

	// 超时必须显式设置：零值是"永不超时"，慢客户端可以一直占着连接，
	// 连接数打满后服务整体不可用
	return &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: xutil.ToDuration(c.ReadHeaderTimeout),
		ReadTimeout:       xutil.ToDuration(c.ReadTimeout),
		WriteTimeout:      xutil.ToDuration(c.WriteTimeout),
		IdleTimeout:       xutil.ToDuration(c.IdleTimeout),
	}
}

func (g *XGin) getXGinOptions() *options.Options {
	do := options.DefaultOptions()
	for _, opt := range g.opts {
		opt(do)
	}
	return do
}

func (g *XGin) registerMiddleware(do *options.Options) {
	// 自动将 metrics 路径加入日志跳过列表，避免 Prometheus 抓取刷日志
	if do.EnableMetricMiddleware {
		do.LogSkipPaths = append(do.LogSkipPaths, do.MetricsPath)
	}

	// 中间件顺序（洋葱模型，自外向内）：
	//   session → trace → log → metric → recover → 用户中间件 → handler
	//
	// recover 必须是框架中间件里最内层的一个。panic 会一路向外抛，
	// 在哪一层被 recover 住，比它更内层的中间件里 c.Next() 之后的代码就都不执行。
	// 若 recover 在 log / metric 之外，handler panic 的请求既不写访问日志，
	// 也不计入 http_requests_total——而 panic 导致的 500 恰恰是最需要计入错误率的那一类。
	//
	// 进程安全不依赖这个顺序：net/http 对每个连接本就有兜底 recover，
	// 框架中间件自身 panic 不会拖垮进程。

	// 提前注入一下 session 相关信息
	g.engine.Use(middleware.Session())

	// 注册trace middleware，需要放在靠前的位置，保证traceid能提前生成，后续middleware和handler能正确获取到
	if do.EnableTraceMiddleware {
		g.engine.Use(middleware.Trace())
	}

	// 注册log middleware
	if do.EnableLogMiddleware {
		g.engine.Use(middleware.Log(middleware.WithSkipPaths(do.LogSkipPaths...)))
	}

	// 注册metric middleware
	if do.EnableMetricMiddleware {
		g.engine.Use(middleware.Metric())
	}

	// 注册recover middleware，放在框架中间件最内层，见上方说明
	g.engine.Use(middleware.Recover(g.recoveryFunc))

	// 注册 metrics 端点
	//
	// 刻意注册在用户中间件之前：/metrics 只被采集器访问，不应经过业务鉴权、
	// 限流等中间件，否则采集器会被 401 挡在外面。需要保护该端点时，
	// 用 options.MetricsPath 换一个不对外暴露的路径，或在网关层限制来源
	if do.EnableMetricMiddleware {
		g.engine.GET(do.MetricsPath, middleware.MetricsHandler())
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
	if xutil.EnableXOneDebug() {
		gin.SetMode(gin.DebugMode)
		return
	}
	gin.SetMode(gin.ReleaseMode)
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
