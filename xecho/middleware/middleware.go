// Package middleware 提供 xecho 内置的中间件。
//
// 洋葱模型，自外向内的顺序是：
//
//	LogScope → Trace → Log → Metric → Recover → 用户中间件 → handler
//
// Recover 必须是框架中间件里最内层的一个：panic 一路向外抛，
// 在哪一层被兜住，比它更内层的中间件里 next(c) 之后的代码就都不执行。
// 放在最内层，外面几层的收尾（记指标、写访问日志）才都还跑得到。
//
// 和 gin 不同的一点：echo 里 handler 返回的错误要等整条链返回之后才由 e.HTTPErrorHandler
// 渲染成响应。Trace、Log、Metric 因此都在自己这一层就把错误交给 c.Error、再往外返回 nil，
// 否则它们记下的状态码全是 200，见 finish。单独用其中哪一个都成立。
package middleware

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/xiaoshicae/xone/internal/web"
	"github.com/xiaoshicae/xone/xlog"
)

// LogScope 为本次请求开一个日志 KV 作用域。
//
// 装上之后，业务代码在任意调用层级都可以 xlog.AddKV(ctx, ...) 补字段，
// 不用把新 context 逐层回传——调用栈深处拿不到 echo.Context，本来也没机会回传。
// 写进去的字段对整条请求可见，所以 Log 在 next(c) 之后打的访问日志也带得上。
//
// 必须排在所有中间件最前面：在它之后才有作用域可写。
func LogScope() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			r := c.Request()
			c.SetRequest(r.WithContext(xlog.CtxWithScope(r.Context())))
			return next(c)
		}
	}
}

// routeOf 取这个请求的路由模板，访问日志、指标、Span 共用。
//
// 没匹配上的记 web.RouteUnmatched（理由见那里），和 xgin 一样，包括两种 echo 给了模板的情况
// （实测 echo v4.16.0）：
//
//   - 没有任何路由匹配这个路径：c.Path() 是空串；
//   - 路径对、方法不对：c.Path() 是那条路由的模板，handler 是 echo 的 MethodNotAllowedHandler（405）；
//     方法是 OPTIONS 时 echo 自己回 204 和 Allow。这两种 router 都会在 c 上留下 Allow 的值
//     （echo.ContextKeyHeaderAllow），别的情况不留。
//
// 405 记成模板的话，xgin 服务和 xecho 服务的同一种请求在看板上是两个样子；
// 而且那条路由在这个方法上根本不存在
func routeOf(c echo.Context) string {
	if p := c.Path(); p != "" && c.Get(echo.ContextKeyHeaderAllow) == nil {
		return p
	}
	return web.RouteUnmatched
}

// errKey handler 返回的错误在 echo.Context 里的 key，见 finish
const errKey = "xone/xecho.error"

// finish 在这一层就把 handler 返回的错误渲染成响应，并把它记在 c 上。
//
// echo 在整条中间件链返回之后才调 e.HTTPErrorHandler 把错误写成响应（echo v4.16.0 的
// Echo.ServeHTTP）。链里的中间件拿到错误的那一刻响应还一个字节都没写：实测 404、405、500
// 都是 Status 200、Committed false、Size 0。照读的话每个出错的请求在访问日志、指标、链路里
// 都是一次 bytes_out 为 0 的 200。
//
// 所以在这里就交给 c.Error（它调的是 e.HTTPErrorHandler，WithRoutes 里换掉的也算），
// 再往外返回 nil：外面几层读到的是已经发出去的状态码和字节数，HTTPErrorHandler 也只跑这一次——
// 照样返回错误的话 echo 会再调它一遍，不查 Committed 的自定义错误处理会把错误响应写两遍。
//
// 错误本身记在 c 上：外面几层拿到的是 nil，访问日志的 errors、Span 的 echo.errors 靠 errorOf 取到它
func finish(c echo.Context, err error) {
	if err == nil {
		return
	}
	c.Set(errKey, err)
	c.Error(err)
}

// errorOf 这个请求登记下的错误：handler 返回的（见 finish），或者 Recover 登记的中止和断连
func errorOf(c echo.Context) error {
	err, _ := c.Get(errKey).(error)
	return err
}

// Recover 兜住 panic，把它变成一条错误日志和一个错误响应。
//
// handle 决定 panic 之后回什么：它返回的错误和 handler 返回的一样交给 e.HTTPErrorHandler。
// 为 nil 时返回 echo.ErrInternalServerError，也就是 echo 默认的
// 500 {"message":"Internal Server Error"}，和 handler 返回一个普通 error 时一个样子。
//
// echo 自己默认不兜 panic：实测连接被直接断掉（客户端读到 EOF），栈由 net/http 的日志写进 stderr，
// 不进 slog。http.ErrAbortHandler 不兜，原样抛给 net/http 去断开连接。
func Recover(handle func(c echo.Context, recovered any) error) echo.MiddlewareFunc {
	if handle == nil {
		handle = func(echo.Context, any) error { return echo.ErrInternalServerError }
	}

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) (err error) {
			defer func() {
				r := recover()
				if r == nil {
					return
				}
				// http.ErrAbortHandler 是 handler 主动要求「断掉这个连接」的约定写法
				// （httputil.ReverseProxy 在上游断开时也这么做）。net/http 接到它会
				// 静默断连、不打栈；在这里兜住的话，本该中止的响应会被写成 500 发出去，
				// 还多一份毫无意义的栈。所以原样抛回给它。
				//
				// 抛回之前登记下来：外面几层（访问日志、指标、链路）的收尾据此
				// 把它记成中止，见 status——它们读到的 Response().Status 往往是
				// 已经发出去的 200，一个被截断的响应就全都记成了成功
				if r == http.ErrAbortHandler {
					c.Set(errKey, http.ErrAbortHandler)
					panic(r)
				}

				// 连接断了不算故障，不值得打一份完整栈
				ctx := c.Request().Context()
				if web.IsBrokenPipe(r) {
					slog.ErrorContext(ctx, "connection broken", "error", r)
					c.Set(errKey, r.(error)) // web.IsBrokenPipe 保证它是 *net.OpError
					err = nil
					return
				}

				slog.ErrorContext(ctx, "panic while handling request",
					"error", r,
					"stack", web.Stack(),
					"path", c.Request().URL.Path,
					"method", c.Request().Method)

				if c.Response().Committed {
					// 响应已经开始往外写了，再改状态码只会得到一个半截的响应
					err = nil
					return
				}
				err = handle(c, r)
			}()
			return next(c)
		}
	}
}

// status 本次请求该记下的状态码。
//
// 不能直接读 Response().Status：中止的请求往往已经写出了 200 的响应头，
// 照读的话一个被截断的响应在日志、指标、链路里全都记成成功
func status(c echo.Context) int {
	if errors.Is(errorOf(c), http.ErrAbortHandler) { // 由 Recover 登记
		return web.StatusAborted
	}
	return c.Response().Status
}
