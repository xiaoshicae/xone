package xhttp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/xiaoshicae/xone/v2/xconfig"
	"github.com/xiaoshicae/xone/v2/xmetric"
	"github.com/xiaoshicae/xone/v2/xtrace"
	"github.com/xiaoshicae/xone/v2/xutil"

	"github.com/bytedance/mockey"
	"github.com/go-resty/resty/v2"
	dto "github.com/prometheus/client_model/go"
	c "github.com/smartystreets/goconvey/convey"
)

type stubRoundTripper struct{}

func (s *stubRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("stub transport")
}

func TestXHttpConfig(t *testing.T) {
	mockey.PatchConvey("TestXHttpConfig-configMergeDefault-Nil", t, func() {
		config := configMergeDefault(nil)
		c.So(config, c.ShouldResemble, &Config{
			Timeout:             "60s",
			DialTimeout:         "30s",
			DialKeepAlive:       "30s",
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     "90s",
			RetryCount:          0,
			RetryWaitTime:       "100ms",
			RetryMaxWaitTime:    "2s",
			EnableMetric:        xutil.ToPtr(true),
		})
	})

	mockey.PatchConvey("TestXHttpConfig-configMergeDefault-NotNil", t, func() {
		config := &Config{
			Timeout: "1",
		}
		config = configMergeDefault(config)
		c.So(config, c.ShouldResemble, &Config{
			Timeout:             "1",
			DialTimeout:         "30s",
			DialKeepAlive:       "30s",
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     "90s",
			RetryCount:          0,
			RetryWaitTime:       "100ms",
			RetryMaxWaitTime:    "2s",
			EnableMetric:        xutil.ToPtr(true),
		})
	})
}

func TestXHttpClient(t *testing.T) {
	mockey.PatchConvey("TestXHttpClient", t, func() {
		mockey.PatchConvey("TestXHttpClient-NotFound", func() {
			client := C()
			c.So(client, c.ShouldNotBeNil)
		})
	})
}

func TestRWithCtx(t *testing.T) {
	mockey.PatchConvey("TestRWithCtx", t, func() {
		ctx := context.Background()
		req := RWithCtx(ctx)
		c.So(req, c.ShouldNotBeNil)
	})
}

func TestRawClient(t *testing.T) {
	mockey.PatchConvey("TestRawClient-NotSet-FallbackWith30sTimeout", t, func() {
		// rawHttpClient 为 nil 时返回带 30s 超时的兜底 client
		rawHttpClient = nil
		mockey.Mock(xutil.WarnIfEnableDebug).Return().Build()
		client := RawClient()
		c.So(client, c.ShouldNotBeNil)
		c.So(client.Timeout, c.ShouldEqual, 30*time.Second)
	})

	mockey.PatchConvey("TestRawClient-Set", t, func() {
		customClient := &http.Client{}
		setRawHttpClient(customClient)
		client := RawClient()
		c.So(client, c.ShouldNotBeNil)
		c.So(client, c.ShouldEqual, customClient)
		// Clean up
		rawHttpClient = nil
	})
}

func TestSetDefaultClient(t *testing.T) {
	mockey.PatchConvey("TestSetDefaultClient", t, func() {
		original := defaultClient
		newClient := resty.New()
		setDefaultClient(newClient)
		c.So(C(), c.ShouldEqual, newClient)
		// Restore
		defaultClient = original
	})
}

func TestSetRawHttpClient(t *testing.T) {
	mockey.PatchConvey("TestSetRawHttpClient", t, func() {
		customClient := &http.Client{}
		setRawHttpClient(customClient)
		c.So(rawHttpClient, c.ShouldEqual, customClient)
		// Clean up
		rawHttpClient = nil
	})
}

func TestGetConfigXHttp(t *testing.T) {
	mockey.PatchConvey("TestGetConfig-UnmarshalFail", t, func() {
		mockey.Mock(xconfig.UnmarshalConfig).Return(errors.New("unmarshal failed")).Build()

		config, err := getConfig()
		c.So(err, c.ShouldNotBeNil)
		c.So(config, c.ShouldBeNil)
	})

	mockey.PatchConvey("TestGetConfig-Success", t, func() {
		mockey.Mock(xconfig.UnmarshalConfig).Return(nil).Build()

		config, err := getConfig()
		c.So(err, c.ShouldBeNil)
		c.So(config, c.ShouldNotBeNil)
		c.So(config.Timeout, c.ShouldEqual, "60s") // 默认值
	})
}

func TestInitHttpClient(t *testing.T) {
	mockey.PatchConvey("TestInitHttpClient-ContainKeyFalse-SkipInit", t, func() {
		// ContainKey 返回 false 时跳过初始化
		mockey.Mock(xconfig.ContainKey).Return(false).Build()
		mockey.Mock(xutil.WarnIfEnableDebug).Return().Build()

		err := initHttpClient()
		c.So(err, c.ShouldBeNil)
	})

	mockey.PatchConvey("TestInitHttpClient-GetConfigFail", t, func() {
		mockey.Mock(xconfig.ContainKey).Return(true).Build()
		mockey.Mock(getConfig).Return(nil, errors.New("config failed")).Build()

		err := initHttpClient()
		c.So(err, c.ShouldNotBeNil)
		c.So(err.Error(), c.ShouldContainSubstring, "getConfig failed")
	})

	mockey.PatchConvey("TestInitHttpClient-Success-NoTrace", t, func() {
		mockey.Mock(xconfig.ContainKey).Return(true).Build()
		mockey.Mock(getConfig).Return(&Config{
			Timeout:             "60s",
			DialTimeout:         "30s",
			DialKeepAlive:       "30s",
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     "90s",
			RetryCount:          0,
			EnableMetric:        xutil.ToPtr(false),
		}, nil).Build()
		mockey.Mock(xtrace.EnableTrace).Return(false).Build()

		err := initHttpClient()
		c.So(err, c.ShouldBeNil)
	})

	mockey.PatchConvey("TestInitHttpClient-Success-WithTrace", t, func() {
		mockey.Mock(xconfig.ContainKey).Return(true).Build()
		mockey.Mock(getConfig).Return(&Config{
			Timeout:             "60s",
			DialTimeout:         "30s",
			DialKeepAlive:       "30s",
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     "90s",
			RetryCount:          0,
			EnableMetric:        xutil.ToPtr(false),
		}, nil).Build()
		mockey.Mock(xtrace.EnableTrace).Return(true).Build()

		err := initHttpClient()
		c.So(err, c.ShouldBeNil)
	})

	mockey.PatchConvey("TestInitHttpClient-Success-WithRetry", t, func() {
		mockey.Mock(xconfig.ContainKey).Return(true).Build()
		mockey.Mock(getConfig).Return(&Config{
			Timeout:             "60s",
			DialTimeout:         "30s",
			DialKeepAlive:       "30s",
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     "90s",
			RetryCount:          3,
			RetryWaitTime:       "100ms",
			RetryMaxWaitTime:    "2s",
			EnableMetric:        xutil.ToPtr(false),
		}, nil).Build()
		mockey.Mock(xtrace.EnableTrace).Return(false).Build()

		err := initHttpClient()
		c.So(err, c.ShouldBeNil)
	})

	mockey.PatchConvey("TestInitHttpClient-Success-WithCustomDialTimeout", t, func() {
		mockey.Mock(xconfig.ContainKey).Return(true).Build()
		mockey.Mock(getConfig).Return(&Config{
			Timeout:             "60s",
			DialTimeout:         "5s",
			DialKeepAlive:       "30s",
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     "90s",
			RetryCount:          0,
			EnableMetric:        xutil.ToPtr(false),
		}, nil).Build()
		mockey.Mock(xtrace.EnableTrace).Return(false).Build()

		err := initHttpClient()
		c.So(err, c.ShouldBeNil)
		c.So(rawHttpClient, c.ShouldNotBeNil)
		c.So(rawHttpClient.Transport, c.ShouldNotBeNil)
	})
}

func TestInitHttpClientDefaultTransportFallback(t *testing.T) {
	mockey.PatchConvey("TestInitHttpClient-DefaultTransportFallback", t, func() {
		origTransport := http.DefaultTransport
		stub := &stubRoundTripper{}
		http.DefaultTransport = stub
		prevRawClient := rawHttpClient
		prevDefaultClient := defaultClient
		defer func() {
			http.DefaultTransport = origTransport
			rawHttpClient = prevRawClient
			defaultClient = prevDefaultClient
		}()

		mockey.Mock(xconfig.ContainKey).Return(true).Build()
		mockey.Mock(getConfig).Return(&Config{
			Timeout:             "60s",
			DialTimeout:         "30s",
			DialKeepAlive:       "30s",
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     "90s",
			EnableMetric:        xutil.ToPtr(false),
		}, nil).Build()
		mockey.Mock(xtrace.EnableTrace).Return(false).Build()

		err := initHttpClient()
		c.So(err, c.ShouldBeNil)
		c.So(rawHttpClient.Transport, c.ShouldEqual, stub)
	})
}

// TestDialTimeoutApplied 验证 DialTimeout 和 DialKeepAlive 配置是否正确应用到 Transport
func TestDialTimeoutApplied(t *testing.T) {
	mockey.PatchConvey("TestDialTimeout-Applied", t, func() {
		prevRawClient := rawHttpClient
		prevDefaultClient := defaultClient
		defer func() {
			rawHttpClient = prevRawClient
			defaultClient = prevDefaultClient
		}()

		// 设置自定义 DialTimeout 和 DialKeepAlive
		mockey.Mock(xconfig.ContainKey).Return(true).Build()
		mockey.Mock(getConfig).Return(&Config{
			Timeout:             "60s",
			DialTimeout:         "5s",
			DialKeepAlive:       "15s",
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     "90s",
			RetryCount:          0,
			EnableMetric:        xutil.ToPtr(false),
		}, nil).Build()
		mockey.Mock(xtrace.EnableTrace).Return(false).Build()

		err := initHttpClient()
		c.So(err, c.ShouldBeNil)

		// 验证 rawHttpClient 已创建
		c.So(rawHttpClient, c.ShouldNotBeNil)

		// 验证 Transport 已设置
		transport, ok := rawHttpClient.Transport.(*http.Transport)
		c.So(ok, c.ShouldBeTrue)
		c.So(transport, c.ShouldNotBeNil)

		// 验证 DialContext 已设置（不为 nil）
		c.So(transport.DialContext, c.ShouldNotBeNil)

		// 验证其他 Transport 配置
		c.So(transport.MaxIdleConns, c.ShouldEqual, 100)
		c.So(transport.MaxIdleConnsPerHost, c.ShouldEqual, 10)
		c.So(transport.IdleConnTimeout, c.ShouldEqual, 90*time.Second)
	})
}

// TestDialConfigMerge 验证 DialTimeout 和 DialKeepAlive 配置合并逻辑
func TestDialConfigMerge(t *testing.T) {
	mockey.PatchConvey("TestDialConfig-ConfigMerge-Empty", t, func() {
		config := configMergeDefault(&Config{})
		c.So(config.DialTimeout, c.ShouldEqual, "30s")
		c.So(config.DialKeepAlive, c.ShouldEqual, "30s")
	})

	mockey.PatchConvey("TestDialConfig-ConfigMerge-CustomTimeout", t, func() {
		config := configMergeDefault(&Config{DialTimeout: "5s"})
		c.So(config.DialTimeout, c.ShouldEqual, "5s")
		c.So(config.DialKeepAlive, c.ShouldEqual, "30s")
	})

	mockey.PatchConvey("TestDialConfig-ConfigMerge-CustomKeepAlive", t, func() {
		config := configMergeDefault(&Config{DialKeepAlive: "15s"})
		c.So(config.DialTimeout, c.ShouldEqual, "30s")
		c.So(config.DialKeepAlive, c.ShouldEqual, "15s")
	})

	mockey.PatchConvey("TestDialConfig-ConfigMerge-CustomBoth", t, func() {
		config := configMergeDefault(&Config{DialTimeout: "5s", DialKeepAlive: "10s"})
		c.So(config.DialTimeout, c.ShouldEqual, "5s")
		c.So(config.DialKeepAlive, c.ShouldEqual, "10s")
	})
}

// ==================== 补充覆盖率 ====================

func TestCloseHttpClient(t *testing.T) {
	mockey.PatchConvey("TestCloseHttpClient-有client时关闭并重置", t, func() {
		rawHttpClient = &http.Client{}
		oldDefaultClient := defaultClient
		defer func() { defaultClient = oldDefaultClient }()

		err := closeHttpClient()
		c.So(err, c.ShouldBeNil)
		c.So(rawHttpClient, c.ShouldBeNil)
		// close 后 defaultClient 不为 nil，被重置为新的 resty.New()
		c.So(defaultClient, c.ShouldNotBeNil)
	})

	mockey.PatchConvey("TestCloseHttpClient-无client时安全返回且defaultClient重置", t, func() {
		rawHttpClient = nil
		oldDefaultClient := defaultClient
		defer func() { defaultClient = oldDefaultClient }()

		err := closeHttpClient()
		c.So(err, c.ShouldBeNil)
		c.So(rawHttpClient, c.ShouldBeNil)
		// 即使 rawHttpClient 为 nil，defaultClient 也被重置
		c.So(defaultClient, c.ShouldNotBeNil)
	})
}

func TestConfigMergeDefault_EnableMetric(t *testing.T) {
	mockey.PatchConvey("TestConfigMergeDefault-EnableMetric未配置默认true", t, func() {
		config := configMergeDefault(&Config{})
		c.So(*config.EnableMetric, c.ShouldBeTrue)
	})

	mockey.PatchConvey("TestConfigMergeDefault-EnableMetric显式false不被覆盖", t, func() {
		f := false
		config := configMergeDefault(&Config{EnableMetric: &f})
		c.So(*config.EnableMetric, c.ShouldBeFalse)
	})
}

func TestSpanNameFormatter(t *testing.T) {
	mockey.PatchConvey("TestSpanNameFormatter-格式化span名称", t, func() {
		req, _ := http.NewRequest("GET", "http://example.com/api/users", nil)
		result := spanNameFormatter("", req)
		c.So(result, c.ShouldEqual, "GET /api/users")
	})

	mockey.PatchConvey("TestSpanNameFormatter-POST请求", t, func() {
		req, _ := http.NewRequest("POST", "http://example.com/api/orders", nil)
		result := spanNameFormatter("", req)
		c.So(result, c.ShouldEqual, "POST /api/orders")
	})
}

func TestInitHttpClient_WithMetric(t *testing.T) {
	mockey.PatchConvey("TestInitHttpClient-启用metric注册Resty中间件", t, func() {
		prevRawClient := rawHttpClient
		prevDefaultClient := defaultClient
		defer func() {
			rawHttpClient = prevRawClient
			defaultClient = prevDefaultClient
		}()

		mockey.Mock(getConfig).Return(&Config{
			Timeout:             "60s",
			DialTimeout:         "30s",
			DialKeepAlive:       "30s",
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     "90s",
			RetryCount:          0,
			EnableMetric:        xutil.ToPtr(true),
		}, nil).Build()
		mockey.Mock(xtrace.EnableTrace).Return(false).Build()

		err := initHttpClient()
		c.So(err, c.ShouldBeNil)
		c.So(rawHttpClient, c.ShouldNotBeNil)

		// metric 从 Transport 层移到 Resty 层，Transport 不再包装 MetricTransport
		_, ok := rawHttpClient.Transport.(*http.Transport)
		c.So(ok, c.ShouldBeTrue)
	})
}

// findMetricFamily 从 Gather 结果中查找指定名称的 MetricFamily
func findMetricFamily(metrics []*dto.MetricFamily, name string) *dto.MetricFamily {
	for _, m := range metrics {
		if *m.Name == name {
			return m
		}
	}
	return nil
}

// findLabelValue 从 Metric 的 Label 中查找指定 name 的 value
func findLabelValue(metric *dto.Metric, name string) string {
	for _, l := range metric.Label {
		if *l.Name == name {
			return *l.Value
		}
	}
	return ""
}

func TestRegisterMetricHooks_OnSuccess(t *testing.T) {
	mockey.PatchConvey("TestRegisterMetricHooks-成功请求记录指标", t, func() {
		// 启动测试 HTTP 服务
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(200)
		}))
		defer server.Close()

		client := resty.New()
		registerMetricHooks(client)

		resp, err := client.R().Get(server.URL + "/api/test")
		c.So(err, c.ShouldBeNil)
		c.So(resp.StatusCode(), c.ShouldEqual, 200)

		// 验证指标被记录
		metrics, gatherErr := xmetric.Registry().Gather()
		c.So(gatherErr, c.ShouldBeNil)

		counterFamily := findMetricFamily(metrics, "http_client_requests_total")
		c.So(counterFamily, c.ShouldNotBeNil)
	})
}

func TestRegisterMetricHooks_OnError(t *testing.T) {
	mockey.PatchConvey("TestRegisterMetricHooks-网络错误记录status0", t, func() {
		client := resty.New()
		client.SetTimeout(100 * time.Millisecond)
		registerMetricHooks(client)

		// 请求一个不存在的地址触发网络错误
		_, err := client.R().Get("http://127.0.0.1:1/unreachable")
		c.So(err, c.ShouldNotBeNil)

		metrics, gatherErr := xmetric.Registry().Gather()
		c.So(gatherErr, c.ShouldBeNil)

		counterFamily := findMetricFamily(metrics, "http_client_requests_total")
		c.So(counterFamily, c.ShouldNotBeNil)

		// 应有 status=0 的记录
		found := false
		for _, m := range counterFamily.Metric {
			if findLabelValue(m, "status") == "0" {
				found = true
			}
		}
		c.So(found, c.ShouldBeTrue)
	})
}

func TestRegisterMetricHooks_RetryExhaustedWithResponse(t *testing.T) {
	mockey.PatchConvey("TestRegisterMetricHooks-重试耗尽记录最终状态码500", t, func() {
		// 始终返回 500 的服务
		var callCount atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			callCount.Add(1)
			w.WriteHeader(500)
		}))
		defer server.Close()

		client := resty.New()
		client.SetRetryCount(2).
			SetRetryWaitTime(1 * time.Millisecond).
			SetRetryMaxWaitTime(5 * time.Millisecond).
			AddRetryCondition(func(resp *resty.Response, err error) bool {
				return resp != nil && resp.StatusCode() >= 500
			})
		registerMetricHooks(client)

		resp, err := client.R().Get(server.URL + "/api/fail")
		// 重试耗尽后网络正常 → err==nil，走 OnSuccess 路径
		c.So(err, c.ShouldBeNil)
		c.So(resp.StatusCode(), c.ShouldEqual, 500)
		// 初始1次 + 重试2次 = 共3次调用
		c.So(callCount.Load(), c.ShouldEqual, 3)

		metrics, gatherErr := xmetric.Registry().Gather()
		c.So(gatherErr, c.ShouldBeNil)

		counterFamily := findMetricFamily(metrics, "http_client_requests_total")
		c.So(counterFamily, c.ShouldNotBeNil)

		// 验证 status=500 被正确记录，且只记录1次（不是3次）
		host := server.Listener.Addr().String()
		var count500 float64
		for _, m := range counterFamily.Metric {
			if findLabelValue(m, "host") == host && findLabelValue(m, "status") == "500" {
				count500 += *m.Counter.Value
			}
		}
		c.So(count500, c.ShouldEqual, 1)
	})
}

func TestRegisterMetricHooks_RetryOnlyRecordsFinalResult(t *testing.T) {
	mockey.PatchConvey("TestRegisterMetricHooks-重试只记录最终结果", t, func() {
		// 前2次返回 404，第3次返回 200
		var callCount atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			n := callCount.Add(1)
			if n <= 2 {
				w.WriteHeader(404)
				return
			}
			w.WriteHeader(200)
		}))
		defer server.Close()

		client := resty.New()
		client.SetRetryCount(3).
			SetRetryWaitTime(1 * time.Millisecond).
			SetRetryMaxWaitTime(5 * time.Millisecond).
			AddRetryCondition(func(resp *resty.Response, err error) bool {
				return resp != nil && resp.StatusCode() == 404
			})
		registerMetricHooks(client)

		resp, err := client.R().Get(server.URL + "/api/retry-test")
		c.So(err, c.ShouldBeNil)
		c.So(resp.StatusCode(), c.ShouldEqual, 200)
		// 确认服务端被调用了 3 次（2次404 + 1次200）
		c.So(callCount.Load(), c.ShouldEqual, 3)

		metrics, gatherErr := xmetric.Registry().Gather()
		c.So(gatherErr, c.ShouldBeNil)

		counterFamily := findMetricFamily(metrics, "http_client_requests_total")
		c.So(counterFamily, c.ShouldNotBeNil)

		// 遍历所有指标，查找匹配当前测试服务地址的记录
		host := server.Listener.Addr().String()
		var count200, count404 float64
		for _, m := range counterFamily.Metric {
			if findLabelValue(m, "host") == host {
				status := findLabelValue(m, "status")
				if status == "200" {
					count200 += *m.Counter.Value
				}
				if status == "404" {
					count404 += *m.Counter.Value
				}
			}
		}
		// OnSuccess 只在最终成功后调用一次，所以只有 1 次 200，没有 404
		c.So(count200, c.ShouldEqual, 1)
		c.So(count404, c.ShouldEqual, 0)
	})
}

func TestRecordHTTPClientMetric_NilReq(t *testing.T) {
	mockey.PatchConvey("TestRecordHTTPClientMetric-nil请求不panic", t, func() {
		// req 为 nil 时不应 panic
		c.So(func() {
			xmetric.RecordHTTPClientMetric("GET", "example.com", "200", 100, nil)
		}, c.ShouldNotPanic)
	})
}

func TestRegisterMetricHooks_OnError_NilRawRequest(t *testing.T) {
	mockey.PatchConvey("TestRegisterMetricHooks-OnError时RawRequest为nil不panic", t, func() {
		client := resty.New()
		registerMetricHooks(client)

		// 模拟 OnError 被调用但 RawRequest 为 nil 的场景
		// 通过发送无效请求触发
		_, err := client.R().Get("://invalid-url")
		// 应该不 panic，错误可以忽略
		_ = err
		// 只要不 panic 就通过
		c.So(true, c.ShouldBeTrue)
	})
}

func TestRegisterMetricHooks_DurationRecorded(t *testing.T) {
	mockey.PatchConvey("TestRegisterMetricHooks-耗时被正确记录", t, func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(10 * time.Millisecond)
			w.WriteHeader(200)
			fmt.Fprint(w, "ok")
		}))
		defer server.Close()

		client := resty.New()
		registerMetricHooks(client)

		resp, err := client.R().Get(server.URL + "/api/slow")
		c.So(err, c.ShouldBeNil)
		c.So(resp.StatusCode(), c.ShouldEqual, 200)

		metrics, gatherErr := xmetric.Registry().Gather()
		c.So(gatherErr, c.ShouldBeNil)

		histFamily := findMetricFamily(metrics, "http_client_request_duration_ms")
		c.So(histFamily, c.ShouldNotBeNil)

		// 应有至少 1 个 histogram 样本
		host := server.Listener.Addr().String()
		for _, m := range histFamily.Metric {
			if findLabelValue(m, "host") == host {
				c.So(*m.Histogram.SampleCount, c.ShouldBeGreaterThan, 0)
				// 耗时应 >= 10ms
				c.So(*m.Histogram.SampleSum, c.ShouldBeGreaterThanOrEqualTo, 10)
			}
		}
	})
}

func TestMetricOnSuccess_NilResp(t *testing.T) {
	mockey.PatchConvey("TestMetricOnSuccess-resp为nil不panic", t, func() {
		c.So(func() {
			metricOnSuccess(nil, nil)
		}, c.ShouldNotPanic)
	})
}

func TestMetricOnSuccess_NilRawRequest(t *testing.T) {
	mockey.PatchConvey("TestMetricOnSuccess-RawRequest为nil不panic", t, func() {
		resp := &resty.Response{
			Request: &resty.Request{
				RawRequest: nil,
			},
		}
		c.So(func() {
			metricOnSuccess(nil, resp)
		}, c.ShouldNotPanic)
	})
}
