package xhttp

import (
	"errors"
	"log/slog"
	"net/url"
	"regexp"
	"time"

	"github.com/go-resty/resty/v2"
)

// installLog 每个逻辑请求记一条日志（Config.Log）。
//
// 挂在 OnSuccess / OnError 上，理由同指标：它们在所有重试结束之后只调一次
// （实测 resty v2.17.2，RetryCount=2、连接被掐断：发了 3 次，OnError 调 1 次，Attempt=3）。
// 记的是方法、host、路径、状态码、耗时、尝试次数；查询串、片段、userinfo、Header、body 都不记——
// 查询串里常有令牌和签名，与出站 Span 的 url.full 去掉查询串是同一条原则。
//
// resty 自己在重试路径上也打日志：每次失败的尝试一行 WARN（…, Attempt N），用完再一行 ERROR，
// 内容就是这里那一行的 error。Log 开着时这两种不再打（见 quietRequestLogger），
// 一个逻辑请求只有这里的一行；每次尝试的细节在出站 Span 里（一次尝试一个 Span）。
func installLog(client *resty.Client, slow time.Duration) {
	markStart(client)
	client.OnBeforeRequest(func(_ *resty.Client, req *resty.Request) error {
		req.SetLogger(quietRequestLogger{})
		return nil
	})
	client.OnSuccess(func(_ *resty.Client, resp *resty.Response) {
		if resp != nil && resp.Request != nil {
			logRequest(resp.Request, resp, nil, slow)
		}
	})
	client.OnError(func(req *resty.Request, err error) {
		if req == nil {
			return
		}
		// 拿到了响应之后才出的错（比如 SetResult 解析失败），resty 包成 ResponseError
		var resp *resty.Response
		var re *resty.ResponseError
		if errors.As(err, &re) {
			resp, err = re.Response, re.Err
		}
		logRequest(req, resp, err, slow)
	})
}

// logRequest 记一个逻辑请求。
//
// 失败 = 没拿到能用的响应（err 不为 nil），或者下游回了 5xx；4xx 是业务上的回答，照常记 INFO。
// ctx 是调用方的：出站 Span 开在 transport 里，这时已经结束，所以 span_id 是调用方的那个。
func logRequest(req *resty.Request, resp *resty.Response, err error, slow time.Duration) {
	ctx := req.Context()
	status, fallback := 0, time.Since(req.Time)
	if resp != nil {
		status, fallback = resp.StatusCode(), resp.Time()
	}
	took := elapsed(req, fallback)
	failed := err != nil || status >= 500
	isSlow := slow > 0 && took > slow
	if !failed && !isSlow && !slog.Default().Enabled(ctx, slog.LevelInfo) {
		return
	}

	host, path := target(req)
	attrs := []any{"method", req.Method, "host", host, "path", path, "status", status,
		"elapsed_ms", ms(took), "attempts", req.Attempt}
	if err != nil {
		attrs = append(attrs, "error", scrubErrorText(err.Error()))
	}

	switch {
	case failed:
		slog.WarnContext(ctx, "http request failed", attrs...)
	case isSlow:
		slog.WarnContext(ctx, "slow http request", append(attrs, "threshold_ms", ms(slow))...)
	default:
		slog.InfoContext(ctx, "http request", attrs...)
	}
}

// urlUserinfo 文本里 http(s) URL 的 userinfo（user@ 或 user:***@）
var urlUserinfo = regexp.MustCompile(`(https?://)[^\s"/?#@]*@`)

// scrubErrorText 错误原文里的 URL 只留 scheme、host、路径。
//
// *url.Error 的原文是 Get "http://user:***@host/x?token=…": …：整条 URL 连查询串都在里面，
// 标准库只把密码换成 ***（实测 Go 1.25），用户名照留
func scrubErrorText(s string) string { return urlUserinfo.ReplaceAllString(stripQuery(s), "$1") }

// target 最后一次尝试发出去的 host 和路径。还没拼出原生请求就失败的（请求前的中间件出错），
// 退回 resty 请求上的 URL 串。只取 Host 和 Path：userinfo、查询串、片段都不要
func target(req *resty.Request) (host, path string) {
	u := (*url.URL)(nil)
	if req.RawRequest != nil {
		u = req.RawRequest.URL
	} else if parsed, err := url.Parse(req.URL); err == nil {
		u = parsed
	}
	if u == nil {
		return "", ""
	}
	return u.Host, u.Path
}

// quietRequestLogger Log 开着时换到每个请求上的 resty logger：
// 不打重试路径上那两种日志，其余照 restyLogger。
//
// resty v2.17.2 在请求自己的 logger 上打的只有这几处（request.go、middleware.go）：
// 每次失败的尝试 Warnf("%v, Attempt %v")、重试用完 Errorf("%v")、明文 HTTP 上用 Basic Auth 的
// 那句 Warnf、调试模式的 Debugf。前两种就是 logRequest 那一行里的 error，
// 请求上的 Errorf 只有重试用完那一处，所以整个不打；Warnf 只挑掉 Attempt 那种。
type quietRequestLogger struct{ restyLogger }

func (quietRequestLogger) Errorf(string, ...any) {}

func (l quietRequestLogger) Warnf(format string, v ...any) {
	if format == "%v, Attempt %v" {
		return
	}
	l.restyLogger.Warnf(format, v...)
}

// ms 耗时换成毫秒，保留到微秒。字段名带单位，与 xgorm、xredis 的 elapsed_ms 一致
func ms(d time.Duration) float64 { return float64(d.Microseconds()) / 1000 }
