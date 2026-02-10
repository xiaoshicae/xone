package xhttp

import (
	"net"
	"net/http"

	"github.com/go-resty/resty/v2"
	"github.com/xiaoshicae/xone/v2/xconfig"
	"github.com/xiaoshicae/xone/v2/xerror"
	"github.com/xiaoshicae/xone/v2/xhook"
	"github.com/xiaoshicae/xone/v2/xtrace"
	"github.com/xiaoshicae/xone/v2/xutil"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

func init() {
	xhook.BeforeStart(initHttpClient)
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
	var finalTransport http.RoundTripper = baseTransport
	if xtrace.EnableTrace() {
		opts := []otelhttp.Option{
			otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string {
				return r.Method + " " + r.URL.Path
			}),
		}
		finalTransport = otelhttp.NewTransport(baseTransport, opts...)
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

	setDefaultClient(restyClient)
	setRawHttpClient(rawHttpClient)

	return nil
}

func getConfig() (*Config, error) {
	c := &Config{}
	if err := xconfig.UnmarshalConfig(XHttpConfigKey, c); err != nil {
		return nil, err
	}
	c = configMergeDefault(c)
	return c, nil
}
