package xgin

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/xiaoshicae/xone/xconfig"
	"github.com/xiaoshicae/xone/xerror"
	"github.com/xiaoshicae/xone/xgin/internal/peer"
	"github.com/xiaoshicae/xone/xgin/middleware"
	"github.com/xiaoshicae/xone/xgin/trans"
	"github.com/xiaoshicae/xone/xhook"
	"github.com/xiaoshicae/xone/xmetric"

	// 用了 xgin 就有链路：xtrace 装好全局的 TracerProvider 和 Propagator，
	// 中间件经 otel 的全局 API 用上它们。不要链路配 XTrace.Enable: false
	_ "github.com/xiaoshicae/xone/xtrace"
)

// XGin 一个待启动的 HTTP 服务。
//
// 它满足 xone.Runnable，直接交给 xone.Run 即可：
//
//	xone.MustRun(xgin.New().WithRoutes(register))
//
// 注意这里没有 import 根包——Go 的接口是结构化的，方法对得上就行。
// 这样「集成不依赖框架」这条在编译层面仍然成立。
type XGin struct {
	override *Config // 见 WithConfig

	routes  []func(*gin.Engine)
	extra   []gin.HandlerFunc
	recover gin.RecoveryFunc

	// 下面这几项由 build 一次写定，之后只读。读它们的要么先调 build，要么是
	// engine 上的 handler——而 engine 只能经 build 拿到。Once 保证写在读之前，不需要锁
	buildOnce sync.Once
	engine    *gin.Engine
	conf      Config         // 装配用的那份配置，Start 用的也是它
	confErr   error          // 配置不合法时的错误，此时 conf 是默认值
	trusted   []netip.Prefix // TrustedProxies 解出来的网段，见 markTrustedPeer

	mu       sync.Mutex
	srv      *http.Server
	stopping bool

	// running 还没返回的 handler 数，见 track。Stop 靠它知道断连之后
	// handler 是不是真的停下来了：连接断了不等于 handler 返回了
	running atomic.Int64
}

// New 创建一个 XGin。
//
// 开关、端口、超时都在配置文件的 XGin 块里；同一个进程里的第二个服务用 WithConfig。
// 这里什么都不读，配置在装配（Engine 或 Start）那一刻才取。
func New() *XGin { return &XGin{} }

// WithConfig 用这份配置起服务，不再读配置文件里的 XGin 块。
//
// 一个进程里起第二个服务时用它——比如对外的 API 一个端口、内部的管理端口
// 另一个。不给它的话两个实例读的是同一块配置，只能监听同一个端口。
//
// 传进来的是完整配置，不是补丁：想在文件配置的基础上改两项，先取
// CurrentConfig 再改（从头写的话从 DefaultConfig 开始，零值的超时是不合法的）：
//
//	c := xgin.CurrentConfig()
//	c.Host, c.Port = "127.0.0.1", 9090
//	admin := xgin.New().WithConfig(c).WithRoutes(adminRoutes)
//
// 它在装配时校验，不合法的话 Start 返回错误、不监听。
func (g *XGin) WithConfig(c Config) *XGin {
	g.override = &c
	return g
}

// CurrentConfig 返回配置文件里 XGin 那一块（拷贝），在 Start 之前任何时候调都行。
//
// 配合 WithConfig 用，见那里的例子。配置文件第一次读的时候才加载，
// 所以在 main 里、xone.Run 之前拿到的也是最终值。
//
// 那一块写得不合法时返回默认值：错误由 xone.Run 在启动时报出来，这里不报第二遍。
func CurrentConfig() Config {
	c, _ := fileConfig()
	return c
}

// WithRoutes 注册路由。可以调用多次，按调用顺序生效。
//
// 回调拿到的是原生 engine，想怎么设都行。它在配置落到 engine 上之后才跑，
// 所以回调里的设置盖得过配置：改 MaxMultipartMemory、调 SetTrustedProxies 都以回调为准。
//
// 只有一处跟不过去：透传 Header（XTrace.ForwardHeaders）认的可信对端只看配置里的
// TrustedProxies。gin 不暴露它当前的代理列表，回调里的 SetTrustedProxies 只管得到
// ClientIP。两边要一起改就改配置。
func (g *XGin) WithRoutes(f ...func(*gin.Engine)) *XGin {
	g.routes = append(g.routes, f...)
	return g
}

// WithMiddleware 追加自定义中间件，排在所有内置中间件之后。
func (g *XGin) WithMiddleware(m ...gin.HandlerFunc) *XGin {
	g.extra = append(g.extra, m...)
	return g
}

// WithRecoverFunc 自定义 panic 之后的响应。默认返回 500。
func (g *XGin) WithRecoverFunc(f gin.RecoveryFunc) *XGin {
	g.recover = f
	return g
}

// Engine 返回原生的 *gin.Engine，需要做本包没覆盖的事情时用它。
//
// 调用它会触发装配，用的是 WithConfig 给的那份配置，或者配置文件里的 XGin 块——
// 配置文件第一次读的时候才加载，所以在 xone.Run 之前调也拿得到最终值。
// 装配只有一次：之后再 WithConfig / WithRoutes / WithMiddleware / WithRecoverFunc
// 就不生效了，Start 也沿用这一次的配置。
//
// 配置不合法时照样返回一个 engine，按默认值装配（不信任何代理、8MB 的 multipart 阈值，
// 都是偏安全的那一侧），并记一条告警。那个错误由 Start 返回，xone.Run 在启动时就会报出来。
func (g *XGin) Engine() *gin.Engine {
	g.build()
	return g.engine
}

// build 装配 engine，一个实例只装配一次。
//
// 用 Once 而不是布尔标志：并发调用时后者会把中间件注册两遍，
// 表现是每个请求打两条日志、指标翻倍。
//
// 顺序有讲究：先定 Mode，再建 engine、落配置，然后挂中间件和内置路由，最后才跑
// 使用者的 WithRoutes——所以回调里对 engine 的设置盖得过配置，见 WithRoutes。
func (g *XGin) build() {
	g.buildOnce.Do(func() {
		// 配置不合法时照样装配，按默认值：Engine() 的调用方总得拿到一个 engine，
		// 而默认值是偏安全的那一侧。错误记下来由 Start 返回；告警是给只用 Engine()、
		// 从不调 Start 的使用者的，否则他们拿到一份默认值却毫无迹象
		c, err := g.cfg()
		if err != nil {
			slog.Warn("xgin invalid config, assembling the engine with defaults; Start will return the error", "error", err)
		}
		g.conf, g.confErr = c, err

		// 在 gin.New 之前设：debug 模式下 gin.New 会打一段「正在以 debug 模式运行，
		// 线上请切到 release」的警告，之后注册的每条路由再各打一行（实测 v1.12.0）。
		// 先建后设的话，配成 release 的服务照样会在启动时打出那段警告。
		//
		// 注意 gin.SetMode 是进程级的，不是这个实例的：同一进程里用 WithConfig
		// 起两个 Mode 不同的服务，后装配的那个说了算。gin 没有实例级的 Mode，
		// 这里没法替它隔离，所以多实例时请让它们的 Mode 一致
		gin.SetMode(c.Mode)
		e := gin.New()
		e.HandleMethodNotAllowed = true // 不开的话，方法不对会返回 404 而不是 405
		applyConfig(e, c)
		g.trusted = prefixes(c.trustedProxies())

		// 洋葱模型，自外向内：
		//   LogScope → Trace → Log → Metric → Recover → 用户中间件 → handler
		//
		// Recover 必须是内置里最内层的：panic 在哪一层被兜住，
		// 比它更内层的中间件里 c.Next() 之后的代码就都不执行了。
		// 放最内层，外面几层的收尾（记指标、写访问日志）才还跑得到。
		if c.Log {
			e.Use(middleware.LogScope())
		}
		// 先判对端可不可信，Trace / Propagate 才知道收不收透传 Header 和 baggage。
		// Trace 只管 Span：关掉时照样接上游的链路标识和透传 Header，只是不开 Span
		if c.Trace {
			e.Use(g.markTrustedPeer, middleware.Trace())
		} else {
			e.Use(g.markTrustedPeer, middleware.Propagate())
		}
		if c.Log {
			skip := append([]string{}, c.LogSkipPaths...)
			if c.Metric {
				// 指标端点会被抓取系统按秒轮询，记日志纯属刷屏
				skip = append(skip, c.MetricPath)
			}
			e.Use(middleware.Log(
				middleware.WithSkipPaths(skip...),
				middleware.WithBody(c.LogRequestBody, c.LogResponseBody),
				middleware.WithQuery(c.LogQuery),
				middleware.WithResponseHeaders(c.LogResponseHeaders),
			))
		}
		if c.Metric {
			e.Use(middleware.Metric())
		}
		e.Use(middleware.Recover(g.recover))
		e.Use(g.extra...)

		// 路由一律注册在所有中间件之后。
		//
		// gin 在注册路由的那一刻就把处理链定死了：那之后再 Use 的中间件
		// 对它不生效。指标端点原先注册在 g.extra 之前，于是使用者用
		// WithMiddleware 挂的统一鉴权对业务路由生效、对 /metrics 不生效——
		// 一个以为被保护起来的端点其实是敞开的。
		if c.Metric {
			e.GET(c.MetricPath, serveMetrics)
		}
		for _, f := range g.routes {
			f(e)
		}

		if c.ZHTranslations {
			if err := trans.RegisterZH(); err != nil {
				slog.Warn("xgin failed to register the zh validation translator", "error", err)
			}
		}

		g.engine = e
	})
}

// serveMetrics 指标端点。每次请求再取 handler，不在装配时定死：装配可能发生在
// xmetric 初始化之前（在 xone.Run 之前调 Engine() 就会），那时拿到的是兜底 registry，
// /metrics 会一直是空的
func serveMetrics(c *gin.Context) { xmetric.Handler().ServeHTTP(c.Writer, c.Request) }

// applyConfig 把配置里管 engine 的那两项落到 e 上
func applyConfig(e *gin.Engine, c Config) {
	e.MaxMultipartMemory = c.MaxMultipartMemory

	// 默认只信私有网段。gin 的默认是全都信，于是任何人发一个
	// X-Forwarded-For 就能决定访问日志里的 client_ip 是什么。
	// 网段写错在 Validate 里就拦下了，这里的错误兜底成「谁都不信」
	if err := e.SetTrustedProxies(c.trustedProxies()); err != nil {
		slog.Warn("xgin invalid TrustedProxies, trusting none", "error", err)
		_ = e.SetTrustedProxies([]string{})
	}
}

// markTrustedPeer 直连的对端在 TrustedProxies 里时给链路中间件留个记号：
// 只有这样的对端发来的透传 Header（XTrace.ForwardHeaders）才会被收下。
//
// 透传是「把上游给的值原样带给下游」。照单全收的话，公网客户端发一个
// X-Tenant-Id / X-Internal-Token，就被当成自己人给的，带进内网的每一次调用。
// 「谁是自己人」用的是信任代理的那张表，不另设开关——同一件事只该有一个地方说。
//
// 判的是直连的对端（RemoteAddr），不是 ClientIP：后者正是从这些可伪造的头里推出来的。
func (g *XGin) markTrustedPeer(c *gin.Context) {
	if trustedAddr(g.trusted, c.RemoteIP()) {
		c.Set(peer.TrustedKey, true)
	}
	c.Next()
}

// prefixes 把 TrustedProxies 解成网段，单个 IP 当作只含它自己的网段。
// 写错的在 Validate 里就拦下了，这里解不出的直接跳过——拿不准就不信
func prefixes(list []string) []netip.Prefix {
	out := make([]netip.Prefix, 0, len(list))
	for _, s := range list {
		if p, err := netip.ParsePrefix(s); err == nil {
			out = append(out, p.Masked())
		} else if a, err := netip.ParseAddr(s); err == nil {
			a = a.Unmap()
			out = append(out, netip.PrefixFrom(a, a.BitLen()))
		}
	}
	return out
}

// trustedAddr ip 是否落在任一网段里。
// Unmap 是为了 ::ffff:10.0.0.1 这种写法的 IPv4 也能对上 10.0.0.0/8
func trustedAddr(ps []netip.Prefix, ip string) bool {
	a, err := netip.ParseAddr(ip)
	if err != nil {
		return false
	}
	a = a.Unmap()
	for _, p := range ps {
		if p.Contains(a) {
			return true
		}
	}
	return false
}

// cfg 这个实例该用的配置：WithConfig 给的那份，不给就是配置文件里的 XGin 块。
// 不合法时返回默认值和那个错误，见 build
func (g *XGin) cfg() (Config, error) {
	if g.override == nil {
		return fileConfig()
	}
	if err := g.override.Validate(); err != nil {
		return DefaultConfig(), fmt.Errorf("invalid config passed to WithConfig: %w", err)
	}
	return *g.override, nil
}

// fileConfig 配置文件里的 XGin 块，没写的字段是默认值。
//
// 在 Start 之前任何时候调都行：配置文件第一次读的时候才加载，读到的永远是最终值。
// xconfig.Unmarshal 解完会调 Validate；解不出来或者不合法时返回默认值和那个错误
func fileConfig() (Config, error) {
	c := DefaultConfig()
	if err := xconfig.Unmarshal(ConfigKey, &c); err != nil {
		return DefaultConfig(), err
	}
	return c, nil
}

// Start 启动服务并阻塞到它停止。由 xone.Run 调用。
//
// 还没装配的话先装配。监听地址、超时、TLS 和 engine 上的中间件、信任的代理
// 出自同一份配置——装配时取的那份，这里不再重读。它不合法时直接返回错误，不监听。
func (g *XGin) Start(ctx context.Context) error {
	g.build()
	if g.confErr != nil {
		return xerror.New("xgin", "config", g.confErr)
	}
	c := g.conf

	tlsCfg, err := c.serverTLS()
	if err != nil {
		return xerror.New("xgin", "config", err)
	}
	addr := net.JoinHostPort(c.Host, strconv.Itoa(c.Port))
	srv := g.newServer(c, addr)
	srv.TLSConfig = tlsCfg // ListenAndServeTLS 在它的副本上补证书和 h2

	g.mu.Lock()
	if g.stopping {
		g.mu.Unlock()
		// 退出信号早于启动到达。照常监听的话，服务会在「已经收到停止信号」
		// 之后才起来，然后一直跑到框架等超时为止
		slog.Warn("xgin received the shutdown signal before starting, the server will not start")
		return nil
	}
	if g.srv != nil {
		g.mu.Unlock()
		return xerror.Newf("xgin", "start", "server is already running on %s", g.srv.Addr)
	}
	g.srv = srv
	g.mu.Unlock()

	slog.Info("xgin listening", "addr", addr, "tls", c.tlsEnabled(), "mtls", c.tlsEnabled() && c.ClientCAFile != "",
		"h2c", c.UseH2C && !c.tlsEnabled())

	if c.tlsEnabled() {
		err = srv.ListenAndServeTLS(c.CertFile, c.KeyFile)
	} else {
		err = srv.ListenAndServe()
	}
	if err == nil || errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return xerror.Newf("xgin", "start", "listen on %s failed: %w", addr, err)
}

// Stop 优雅关闭服务：等在途请求做完，最多等到 ctx 的截止时间。由 xone.Run 调用。
//
// 等多久只看 ctx，这里不另设上限。在 xone.Run 里它带的是服务那一段停止预算
// （WithStopTimeout 的 2/3，默认 10s）：整个退出流程只有这一份预算，
// 服务能占多少由框架分，不需要再配一个超时去和它对齐。
//
// 单独使用、不经过 xone.Run 时，截止时间由你传进来的 ctx 定：
//
//	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
//	defer cancel()
//	err := g.Stop(ctx)
//
// 不带截止时间的 ctx 就一直等到在途请求全部做完（实测一个 1.5s 的请求，
// Shutdown 等了 1.57s 才返回）——挂住的请求会让 Stop 一直不返回。
//
// 到点还有请求没做完时强制断开所有连接，再等 handler 返回，并返回错误。
//
// 框架能保证的是：返回 nil 时所有 handler 都已经返回；到点还有 handler
// 没返回时，返回的错误里写明还剩几个。它保证不了的是让一个不看请求 ctx 的
// handler 停下来——Go 没有从外面终止一个协程的办法，断开连接、取消
// 请求的 ctx 已经是能做的全部。handler 里的慢操作（查库、调下游）
// 要传 c.Request.Context()，断连之后才停得下来。
func (g *XGin) Stop(ctx context.Context) error {
	g.mu.Lock()
	g.stopping = true // 先置位：Start 若还没开始监听，到达时会直接返回
	srv := g.srv
	g.mu.Unlock()

	if srv == nil {
		return nil // 信号在服务起来之前就到了
	}

	shutCtx, cancel := shutdownCtx(ctx)
	defer cancel()
	err := srv.Shutdown(shutCtx)
	if err != nil {
		// 到点了还有请求没做完。Shutdown 只是返回错误，它不动那些连接，
		// 所以补一刀 Close()：断掉所有连接，在途请求的 ctx 随之取消。
		// 在途请求会失败，但那本来就是超时的含义
		if cerr := srv.Close(); cerr != nil {
			slog.Warn("xgin force close failed", "error", cerr)
		}
	}

	// 连接断了不等于 handler 返回了：Close 只关连接、取消请求的 ctx，
	// handler 所在的协程照跑。不等它们的话，框架紧接着去关数据库和缓存，
	// 还没返回的 handler 会摸到已经关掉的连接池。
	// Shutdown 成功时也要等：被劫持走的连接（WebSocket）Shutdown 不等，Close 也断不掉
	if n := g.waitHandlers(ctx); n > 0 {
		return xerror.Newf("xgin", "stop", "%d handler(s) still running when the shutdown deadline passed: %w", n, ctx.Err())
	}
	if err != nil {
		return xerror.Newf("xgin", "stop", "graceful shutdown timed out, connections were force closed: %w", err)
	}
	return nil
}

// shutdownCtx 给 Shutdown 的截止时间比 ctx 的早一截：剩余时间的 20%，最多 1s。
//
// 留出来的这一截给断连之后等 handler 返回。Shutdown 用满全部时间的话，
// Close 那一刀落下时预算已经花完，看到 ctx 取消、正在收尾的 handler 没人等，
// 框架照样在它们返回之前就去关数据库了
func shutdownCtx(ctx context.Context) (context.Context, context.CancelFunc) {
	deadline, ok := ctx.Deadline()
	if !ok {
		return context.WithCancel(ctx) // 没有截止时间就一直等，也就不用留尾巴
	}
	tail := min(time.Second, time.Until(deadline)/5)
	return context.WithDeadline(ctx, deadline.Add(-tail))
}

// track 给每个请求计数，Stop 据此等 handler 真正返回。
// 包在 engine 最外面，使用者看不到它
func (g *XGin) track(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		g.running.Add(1)
		defer g.running.Add(-1) // defer：handler 以 panic 结束（比如 http.ErrAbortHandler）时也要减回去
		h.ServeHTTP(w, r)
	})
}

// waitHandlers 等所有 handler 返回，直到 ctx 结束。返回那时还没返回的个数。
//
// 轮询而不是等通知：只在退出时跑这一次，net/http 的 Shutdown 自己也是轮询的
func (g *XGin) waitHandlers(ctx context.Context) int64 {
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for {
		n := g.running.Load()
		if n == 0 {
			return 0
		}
		select {
		case <-ctx.Done():
			return g.running.Load()
		case <-tick.C:
		}
	}
}

// newServer 按配置构建 http.Server
func (g *XGin) newServer(c Config, addr string) *http.Server {
	// 超时必须显式设置：零值是「永不超时」，慢客户端可以一直占着连接，
	// 连接数打满之后服务整体不可用
	return &http.Server{
		Addr:              addr,
		Handler:           g.track(g.engine.Handler()),
		Protocols:         protocols(c),
		ReadHeaderTimeout: c.ReadHeaderTimeout,
		ReadTimeout:       c.ReadTimeout,
		WriteTimeout:      c.WriteTimeout,
		IdleTimeout:       c.IdleTimeout,
	}
}

// protocols 这个服务说哪几种协议。
//
// h2c 用标准库自己的 UnencryptedHTTP2（Go 1.24 起），不用 x/net 的
// h2c.NewHandler：后者把连接劫持走，http.Server 从此不认识它们——
// 实测一个 2s 的请求在途时，Shutdown 约 60µs 就返回 nil，Close() 同样
// 立即返回，那个请求又照跑了整整 2s，而框架紧接着就去关数据库了。
// 交给标准库之后，这些连接和 HTTP/1.1 的一样归 Shutdown / Close 管。
//
// 代价是不再支持 HTTP/1.1 的 Upgrade: h2c 握手，只认「先验知识」——
// 客户端一上来就发 HTTP/2 前言（gRPC、curl --http2-prior-knowledge 都是这样）。
// 发 Upgrade 的客户端不会失败，拿到的是一个普通的 HTTP/1.1 响应。
//
// HTTP/2 的参数用标准库默认（实测单连接最多 250 个并发流）。
func protocols(c Config) *http.Protocols {
	p := new(http.Protocols)
	p.SetHTTP1(true)
	p.SetHTTP2(true) // 只对 TLS 生效，明文连接上它不起作用
	p.SetUnencryptedHTTP2(c.UseH2C && !c.tlsEnabled())
	return p
}

// ---- 登记 ----

// 本包不登记停止钩子：服务本身由使用者交给 xone.Run 启停，
// 框架的 Runnable 只有一个，那个位置是使用者的。
func init() {
	xhook.BeforeStart(loadConfig, xhook.At(xhook.StageServer))
}

// loadConfig 在启动阶段把 XGin 块读一遍：认领它，并让配错的值在这里就失败。
//
// 读出来不存：XGin 在装配时自己去读（CurrentConfig 也是），读到的是同一份最终值。
// 少了这一步，这一块要到服务 Start 时才有人读——那已经在框架检查「没人认领的块」
// 之后了，它会被当成拼错的 key 让启动失败；进程里的服务都用 WithConfig 时更是
// 永远没人读。配错的值也要拖到 Start 才报出来。
func loadConfig(context.Context) error {
	_, err := fileConfig()
	return err
}
