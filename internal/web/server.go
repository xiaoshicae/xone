//go:build go1.24

package web

// 核心模块的下限是 Go 1.23，而 h2c 要用 1.24 起才有的 http.Protocols（理由见 protocols）。
// Web 集成的下限都在 1.24 之上，只有它们用得到 Server；1.23 编核心时这个文件不编进去，
// 本包其余的部分照常可用。

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/xiaoshicae/xone/xerror"
)

// Server 一个 HTTP 服务的启停：只起一次，可以早于 Start 就 Stop，
// Stop 返回 nil 时所有 handler 都已经返回。
//
// 零值加上 Module 就能用，不可复制。Start / Stop 的语义写在集成公开的方法上
// （xgin.XGin.Start / Stop），使用者读的是那里。
type Server struct {
	// Module 报错和日志里的模块名（"xgin"）：错误出自谁的 Start / Stop，就算谁报的
	Module string

	mu       sync.Mutex
	srv      *http.Server
	stopping bool

	// running 还没返回的 handler 数，见 track。Stop 靠它知道断连之后
	// handler 是不是真的停下来了：连接断了不等于 handler 返回了
	running atomic.Int64
}

// Start 按 c 监听、用 h 处理请求，阻塞到服务停止。
//
// TLS 的设置不对（读不出 ClientCAFile）时返回 config 错误、不监听；
// Stop 早于它到达时不监听、直接返回 nil。
func (s *Server) Start(c ServerConfig, h http.Handler) error {
	tlsCfg, err := c.serverTLS()
	if err != nil {
		return xerror.New(s.Module, "config", err)
	}
	addr := net.JoinHostPort(c.Host, strconv.Itoa(c.Port))
	srv := s.newServer(c, addr, h)
	srv.TLSConfig = tlsCfg // ListenAndServeTLS 在它的副本上补证书和 h2

	s.mu.Lock()
	if s.stopping {
		s.mu.Unlock()
		// 退出信号早于启动到达。照常监听的话，服务会在「已经收到停止信号」
		// 之后才起来，然后一直跑到框架等超时为止
		slog.Warn(s.Module + " received the shutdown signal before starting, the server will not start")
		return nil
	}
	if s.srv != nil {
		s.mu.Unlock()
		return xerror.Newf(s.Module, "start", "server is already running on %s", s.srv.Addr)
	}
	s.srv = srv
	s.mu.Unlock()

	slog.Info(s.Module+" listening", "addr", addr, "tls", c.tlsEnabled(), "mtls", c.tlsEnabled() && c.ClientCAFile != "",
		"h2c", c.UseH2C && !c.tlsEnabled())

	if c.tlsEnabled() {
		err = srv.ListenAndServeTLS(c.CertFile, c.KeyFile)
	} else {
		err = srv.ListenAndServe()
	}
	if err == nil || errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return xerror.Newf(s.Module, "start", "listen on %s failed: %w", addr, err)
}

// Stop 优雅关闭：等在途请求做完，最多等到 ctx 的截止时间；
// 到点还有请求没做完时强制断开所有连接，再等 handler 返回，并返回错误。
func (s *Server) Stop(ctx context.Context) error {
	s.mu.Lock()
	s.stopping = true // 先置位：Start 若还没开始监听，到达时会直接返回
	srv := s.srv
	s.mu.Unlock()

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
			slog.Warn(s.Module+" force close failed", "error", cerr)
		}
	}

	// 连接断了不等于 handler 返回了：Close 只关连接、取消请求的 ctx，
	// handler 所在的协程照跑。不等它们的话，框架紧接着去关数据库和缓存，
	// 还没返回的 handler 会摸到已经关掉的连接池。
	// Shutdown 成功时也要等：被劫持走的连接（WebSocket）Shutdown 不等，Close 也断不掉
	if n := s.waitHandlers(ctx); n > 0 {
		return xerror.Newf(s.Module, "stop", "%d handler(s) still running when the shutdown deadline passed: %w", n, ctx.Err())
	}
	if err != nil {
		return xerror.Newf(s.Module, "stop", "graceful shutdown timed out, connections were force closed: %w", err)
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
// 包在 handler 最外面，使用者看不到它
func (s *Server) track(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.running.Add(1)
		defer s.running.Add(-1) // defer：handler 以 panic 结束（比如 http.ErrAbortHandler）时也要减回去
		h.ServeHTTP(w, r)
	})
}

// waitHandlers 等所有 handler 返回，直到 ctx 结束。返回那时还没返回的个数。
//
// 轮询而不是等通知：只在退出时跑这一次，net/http 的 Shutdown 自己也是轮询的
func (s *Server) waitHandlers(ctx context.Context) int64 {
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for {
		n := s.running.Load()
		if n == 0 {
			return 0
		}
		select {
		case <-ctx.Done():
			return s.running.Load()
		case <-tick.C:
		}
	}
}

// newServer 按配置构建 http.Server
func (s *Server) newServer(c ServerConfig, addr string, h http.Handler) *http.Server {
	// 超时必须显式设置：零值是「永不超时」，慢客户端可以一直占着连接，
	// 连接数打满之后服务整体不可用
	return &http.Server{
		Addr:              addr,
		Handler:           s.track(h),
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
func protocols(c ServerConfig) *http.Protocols {
	p := new(http.Protocols)
	p.SetHTTP1(true)
	p.SetHTTP2(true) // 只对 TLS 生效，明文连接上它不起作用
	p.SetUnencryptedHTTP2(c.UseH2C && !c.tlsEnabled())
	return p
}
