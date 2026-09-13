package xhttp

import (
	"errors"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/xiaoshicae/xone/v2/xconfig"
	"github.com/xiaoshicae/xone/v2/xerror"
	"github.com/xiaoshicae/xone/v2/xhook"
	"github.com/xiaoshicae/xone/v2/xmetric"
	"github.com/xiaoshicae/xone/v2/xtrace"
	"github.com/xiaoshicae/xone/v2/xutil"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

func init() {
	xhook.BeforeStart(initHttpClient)
	xhook.BeforeStop(closeHttpClient)
}

func closeHttpClient() error {
	clientMu.Lock()
	defer clientMu.Unlock()

	if rawHttpClient != nil {
		rawHttpClient.CloseIdleConnections()
		rawHttpClient = nil
	}
	// 回到带兜底超时的 client：关闭后仍可能有 BeforeStop hook 发请求，
	// 让它超时退出，而不是挂住整个关闭流程
	defaultClient = newFallbackRestyClient()
	return nil
}

func initHttpClient() error {
	c, err := getConfig()
	if err != nil {
		return xerror.Newf("xhttp", "init", "getConfig failed, err=[%v]", err)
	}
	xutil.InfoIfEnableDebug("XOne initHttpClient got config: %s", xutil.ToJsonString(c))

	rawHttpClient := &http.Client{
		Transport: buildTransport(c),
		Timeout:   xutil.ToDuration(c.Timeout),
	}

	restyClient := resty.NewWithClient(rawHttpClient)

	// 配置重试
	if c.RetryCount > 0 {
		restyClient.
			SetRetryCount(c.RetryCount).
			SetRetryWaitTime(xutil.ToDuration(c.RetryWaitTime)).
			SetRetryMaxWaitTime(xutil.ToDuration(c.RetryMaxWaitTime))

		if c.retryOnlyIdempotentEnabled() {
			restyClient.AddRetryCondition(retryOnlyIdempotent)
		}
	}

	// Resty 层记录 metric，只记录重试后的最终结果（不记录重试中间状态）
	if *c.EnableMetric {
		registerMetricHooks(restyClient)
	}

	setDefaultClient(restyClient)
	// 重复初始化时释放上一个 transport 的空闲连接，否则旧连接一直挂着
	if old := swapRawHttpClient(rawHttpClient); old != nil {
		old.CloseIdleConnections()
	}

	return nil
}

// idempotentMethods 可安全重试的 HTTP 方法（RFC 9110 幂等方法）
var idempotentMethods = map[string]struct{}{
	http.MethodGet:     {},
	http.MethodHead:    {},
	http.MethodOptions: {},
	http.MethodTrace:   {},
	http.MethodPut:     {},
	http.MethodDelete:  {},
}

// retryOnlyIdempotent 只允许幂等方法重试
//
// resty 默认的重试条件是「传输层出错就重试」，不看 HTTP 方法。
// 但超时无法区分「请求没到服务端」和「服务端处理完了但响应丢了」，
// 重发一个 POST 就可能变成重复下单
func retryOnlyIdempotent(resp *resty.Response, err error) bool {
	if err == nil {
		return false // 拿到响应就不重试，与 resty 默认条件一致
	}
	method := ""
	if resp != nil && resp.Request != nil {
		method = strings.ToUpper(resp.Request.Method)
	}
	_, ok := idempotentMethods[method]
	if !ok {
		xutil.WarnIfEnableDebug("XHttp skip retry for non-idempotent method=[%s], set XHttp.RetryOnlyIdempotent=false to allow", method)
	}
	return ok
}

// buildTransport 按配置构建出站 Transport 链
//
// HostAwareTransport 把目标 host 写入 ctx，使 HeaderPropagator 能按域名过滤透传 Header
//
//	trace 开启：client → HostAwareTransport → otelhttp.Transport → baseTransport
//	  otelhttp 负责注入 trace 与透传 Header
//	trace 关闭但配置了 Header 透传：client → HostAwareTransport → ForwardHeaderTransport → baseTransport
//	  链路关闭不应让已配置的 Header 透传静默失效
func buildTransport(c *Config) http.RoundTripper {
	base := tunedBaseTransport(c)
	switch {
	case xtrace.TraceEnabled():
		otelTransport := otelhttp.NewTransport(base, otelhttp.WithSpanNameFormatter(spanNameFormatter))
		return &xtrace.HostAwareTransport{Next: otelTransport}
	case xtrace.ForwardHeaderEnabled():
		return &xtrace.HostAwareTransport{Next: &xtrace.ForwardHeaderTransport{Next: base}}
	default:
		return base
	}
}

// tunedBaseTransport 基于 DefaultTransport 克隆，保留 TLS、HTTP/2、Dial 等默认配置
func tunedBaseTransport(c *Config) http.RoundTripper {
	transport, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		xutil.WarnIfEnableDebug("XOne initHttpClient http.DefaultTransport is %T, skip transport tuning", http.DefaultTransport)
		return http.DefaultTransport
	}

	transport = transport.Clone()
	transport.MaxIdleConns = c.MaxIdleConns
	transport.MaxIdleConnsPerHost = c.MaxIdleConnsPerHost
	transport.IdleConnTimeout = xutil.ToDuration(c.IdleConnTimeout)
	transport.DialContext = (&net.Dialer{
		Timeout:   xutil.ToDuration(c.DialTimeout),
		KeepAlive: xutil.ToDuration(c.DialKeepAlive),
	}).DialContext
	return transport
}

// spanNameFormatter otelhttp 的 span 命名格式：METHOD PATH
func spanNameFormatter(_ string, r *http.Request) string {
	return r.Method + " " + r.URL.Path
}

// registerMetricHooks 注册 Resty 中间件记录出站请求指标
// 使用 OnSuccess + OnError，它们在所有重试结束后只调用一次，不会因重试导致指标虚高
// 注意：OnInvalid（multipart+非 POST/PUT/PATCH）属于编程错误，不记录指标
// duration 为最终请求的耗时，不含重试等待时间（req.Time 在每次 execute 时重置）
func registerMetricHooks(client *resty.Client) {
	client.OnSuccess(metricOnSuccess)
	client.OnError(metricOnError)
}

// metricOnSuccess 成功回调：所有重试结束后，最终请求成功时调用一次
// 包括重试耗尽但网络正常的情况（如 500 重试耗尽，err==nil，走 OnSuccess）
func metricOnSuccess(_ *resty.Client, resp *resty.Response) {
	if resp == nil {
		return
	}
	raw := resp.Request.RawRequest
	if raw == nil {
		return
	}
	xmetric.RecordHTTPClientMetric(
		raw.Method,
		raw.URL.Host,
		strconv.Itoa(resp.StatusCode()),
		resp.Time(),
		raw,
	)
}

// metricOnError 失败回调：所有重试结束后，最终请求仍失败时调用一次（仅网络级错误）
func metricOnError(req *resty.Request, err error) {
	raw := req.RawRequest
	if raw == nil {
		return
	}
	status := "0"
	duration := time.Since(req.Time)

	// ResponseError 包含最终响应（如非重试条件的错误带有部分响应）
	var re *resty.ResponseError
	if errors.As(err, &re) && re.Response != nil {
		status = strconv.Itoa(re.Response.StatusCode())
		duration = re.Response.Time()
	}
	xmetric.RecordHTTPClientMetric(raw.Method, raw.URL.Host, status, duration, raw)
}

func getConfig() (*Config, error) {
	c := &Config{}
	if err := xconfig.UnmarshalConfig(XHttpConfigKey, c); err != nil {
		return nil, err
	}
	c = configMergeDefault(c)
	return c, nil
}
