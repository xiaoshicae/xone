package web

import (
	"errors"
	"net"
	"net/http"
	"os"
	"runtime"
	"strings"
)

// 访问日志、指标、Span 三处共用的几个取值约定。框架各自取出原始值（路由模板、方法、状态码），
// 按这里的规矩收敛之后再记：同一个服务换一个框架，看板和告警上的标签值不该变。

// RouteUnmatched 没匹配上任何路由时，访问日志的 route、指标的 route 标签、Span 名里记的值。
//
// 用路由模板而不是真实路径：/user/123 和 /user/456 是同一个接口，按真实路径记的话
// 指标标签和 Span 名的基数随 URL 里的 id 无限增长，Prometheus 会被撑垮。
// 没匹配上时填真实路径同样撑爆基数，日志里也分不出 /nope 是一个路由还是一次 404。
// 真实路径在访问日志的 path 里
const RouteUnmatched = "unmatched"

// StatusAborted handler 以 http.ErrAbortHandler 中止的请求，在访问日志、指标、链路里记成这个状态码。
//
// 借用的是 nginx 的 499：这个码不会真的发给客户端（连接直接断了），只是给
// 「没有正常结束的请求」一个固定的、查得到的值，不和任何真实的响应混在一起。
// 中止的请求往往已经写出了 200 的响应头，照读框架记下的状态码的话，
// 一个被截断的响应在三处都记成成功
const StatusAborted = 499

// knownMethods RFC 9110 定的那几个方法，加上 PATCH
var knownMethods = map[string]struct{}{
	http.MethodGet: {}, http.MethodHead: {}, http.MethodPost: {}, http.MethodPut: {},
	http.MethodPatch: {}, http.MethodDelete: {}, http.MethodConnect: {},
	http.MethodOptions: {}, http.MethodTrace: {},
}

// MethodOther 不认识的方法统一记成这个
const MethodOther = "OTHER"

// NormalizeMethod 把方法收敛到一个固定集合，指标的 method 标签和 Span 名用它。
//
// 路由已经用模板挡住了 URL 里的 id，方法这一维却是照抄请求的——而 HTTP 的
// 方法是一个自由 token，谁都可以发 CUSTOM1、CUSTOM2。每来一个新值就多一组
// 时间序列，没有淘汰机制：指标内存、抓取响应、监控存储一起涨。
// 就算最后返回 404 / 405 也已经记进去了。
func NormalizeMethod(m string) string {
	if _, ok := knownMethods[m]; ok {
		return m
	}
	return MethodOther
}

// IsBrokenPipe 判断 panic 出来的值是不是客户端提前断开连接（broken pipe / connection reset）。
// 连接断了不算故障，Recover 中间件据此不打栈
func IsBrokenPipe(err any) bool {
	ne, ok := err.(*net.OpError)
	if !ok {
		return false
	}
	var se *os.SyscallError
	if !errors.As(ne, &se) {
		return false
	}
	msg := strings.ToLower(se.Error())
	return strings.Contains(msg, "broken pipe") || strings.Contains(msg, "connection reset by peer")
}

// maxStack panic 栈信息的上限
const maxStack = 16 * 1024

// Stack 当前协程的栈，最多 16KB。
//
// 用 runtime.Stack 而不是逐帧读源码文件：panic 恢复期间去做文件 I/O，
// 磁盘一慢就把这条错误日志也拖住了。
func Stack() string {
	buf := make([]byte, maxStack)
	return string(buf[:runtime.Stack(buf, false)])
}
