package xhttp

import (
	"fmt"
	"net/http"

	"github.com/go-resty/resty/v2"
	"github.com/xiaoshicae/xone/xconfig"
	"github.com/xiaoshicae/xone/xhook"
	"github.com/xiaoshicae/xone/xtrace"
	"github.com/xiaoshicae/xone/xutil"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

func init() {
	xhook.BeforeStart(initHttpClient)
}

func initHttpClient() error {
	c, err := getConfig()
	if err != nil {
		return fmt.Errorf("XOne initHttpClient invoke getConfig failed, err=[%v]", err)
	}
	xutil.InfoIfEnableDebug("XOne initHttpClient got config: %s", xutil.ToJsonString(c))

	// 基于 DefaultTransport 克隆，保留 TLS、HTTP/2、Dial 等默认配置
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConns = c.MaxIdleConns
	transport.MaxIdleConnsPerHost = c.MaxIdleConnsPerHost
	transport.IdleConnTimeout = xutil.ToDuration(c.IdleConnTimeout)

	// 根据是否启用 trace 选择 Transport
	var finalTransport http.RoundTripper = transport
	if xtrace.EnableTrace() {
		opts := []otelhttp.Option{
			otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string {
				return r.Method + " " + r.URL.Path
			}),
		}
		finalTransport = otelhttp.NewTransport(transport, opts...)
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
