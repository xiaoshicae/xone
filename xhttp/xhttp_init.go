package xhttp

import (
	"errors"
	"net"
	"net/http"
	"strconv"
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
	return nil
}

func initHttpClient() error {
	c, err := getConfig()
	if err != nil {
		return xerror.Newf("xhttp", "init", "getConfig failed, err=[%v]", err)
	}
	xutil.InfoIfEnableDebug("XOne initHttpClient got config: %s", xutil.ToJsonString(c))

	// 基于 DefaultTransport 克隆，保留 TLS、HTTP/2、Dial 等默认配置
	baseTransport := http.DefaultTransport
	if transport, ok := baseTransport.(*http.Transport); ok {
		transport = transport.Clone()
		transport.MaxIdleConns = c.MaxIdleConns
		transport.MaxIdleConnsPerHost = c.MaxIdleConnsPerHost
		transport.IdleConnTimeout = xutil.ToDuration(c.IdleConnTimeout)
		transport.DialContext = (&net.Dialer{
			Timeout:   xutil.ToDuration(c.DialTimeout),
			KeepAlive: xutil.ToDuration(c.DialKeepAlive),
		}).DialContext
		baseTransport = transport
	} else {
		xutil.WarnIfEnableDebug("XOne initHttpClient http.DefaultTransport is %T, skip transport tuning", baseTransport)
	}

	// 根据是否启用 trace 选择 Transport
	// 链路：client → HostAwareTransport（设置目标 host 到 ctx）→ otelhttp.Transport → baseTransport
	// HostAwareTransport 使 HeaderPropagator 能按域名过滤透传 Header
	var finalTransport http.RoundTripper = baseTransport
	if xtrace.EnableTrace() {
		opts := []otelhttp.Option{
			otelhttp.WithSpanNameFormatter(spanNameFormatter),
		}
		otelTransport := otelhttp.NewTransport(baseTransport, opts...)
		finalTransport = &xtrace.HostAwareTransport{Next: otelTransport}
	}

	rawHttpClient := &http.Client{
		Transport: finalTransport,
		Timeout:   xutil.ToDuration(c.Timeout),
	}

	restyClient := resty.NewWithClient(rawHttpClient)

	// 配置重试
	if c.RetryCount > 0 {
		restyClient.
			SetRetryCount(c.RetryCount).
			SetRetryWaitTime(xutil.ToDuration(c.RetryWaitTime)).
			SetRetryMaxWaitTime(xutil.ToDuration(c.RetryMaxWaitTime))
	}

	// Resty 层记录 metric，只记录重试后的最终结果（不记录重试中间状态）
	if *c.EnableMetric {
		registerMetricHooks(restyClient)
	}

	setDefaultClient(restyClient)
	setRawHttpClient(rawHttpClient)

	return nil
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
		float64(resp.Time().Milliseconds()),
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
	durationMs := float64(time.Since(req.Time).Milliseconds())

	// ResponseError 包含最终响应（如非重试条件的错误带有部分响应）
	var re *resty.ResponseError
	if errors.As(err, &re) && re.Response != nil {
		status = strconv.Itoa(re.Response.StatusCode())
		durationMs = float64(re.Response.Time().Milliseconds())
	}
	xmetric.RecordHTTPClientMetric(raw.Method, raw.URL.Host, status, durationMs, raw)
}

func getConfig() (*Config, error) {
	c := &Config{}
	if err := xconfig.UnmarshalConfig(XHttpConfigKey, c); err != nil {
		return nil, err
	}
	c = configMergeDefault(c)
	return c, nil
}
