package xhttp

import (
	"fmt"
	"net/http"

	"github.com/go-resty/resty/v2"
	"github.com/spf13/cast"
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
	transport.IdleConnTimeout = cast.ToDuration(c.IdleConnTimeout)

	var rawHttpClient *http.Client
	if xtrace.EnableTrace() {
		opts := []otelhttp.Option{
			otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string {
				return r.Method + " " + r.URL.Path
			}),
		}
		rawHttpClient = &http.Client{
			Transport: otelhttp.NewTransport(transport, opts...),
			Timeout:   cast.ToDuration(c.Timeout),
		}
	} else {
		rawHttpClient = &http.Client{
			Transport: transport,
			Timeout:   cast.ToDuration(c.Timeout),
		}
	}

	setDefaultClient(resty.NewWithClient(rawHttpClient))
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
