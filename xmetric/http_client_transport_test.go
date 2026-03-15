package xmetric

import (
	"errors"
	"net/http"
	"sync"
	"testing"

	. "github.com/bytedance/mockey"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/xiaoshicae/xone/v2/xutil"
)

// resetClientMetricState 重置 HTTP client metric 相关全局状态
func resetClientMetricState() {
	resetState()
	clientMetricOnce = sync.Once{}
	clientRequestsTotal = nil
	clientRequestDuration = nil
}

// mockRoundTripper 模拟 HTTP RoundTripper
type mockRoundTripper struct {
	resp *http.Response
	err  error
}

func (m *mockRoundTripper) RoundTrip(_ *http.Request) (*http.Response, error) {
	return m.resp, m.err
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

func TestNewHTTPClientMetricTransport(t *testing.T) {
	PatchConvey("TestNewHTTPClientMetricTransport-正常创建", t, func() {
		resetClientMetricState()

		mock := &mockRoundTripper{}
		transport := NewHTTPClientMetricTransport(mock)

		So(transport, ShouldNotBeNil)
		So(transport.Next, ShouldEqual, mock)
		// collector 延迟到首次 RecordHTTPClientMetric 时初始化
		So(clientRequestsTotal, ShouldBeNil)
		So(clientRequestDuration, ShouldBeNil)
	})

	PatchConvey("TestNewHTTPClientMetricTransport-next为nil时使用DefaultTransport", t, func() {
		resetClientMetricState()

		transport := NewHTTPClientMetricTransport(nil)

		So(transport, ShouldNotBeNil)
		So(transport.Next, ShouldNotBeNil)
		So(transport.Next, ShouldEqual, http.DefaultTransport)
	})

	PatchConvey("TestNewHTTPClientMetricTransport-带Namespace", t, func() {
		resetClientMetricState()
		registryMu.Lock()
		metricConfig = &Config{Namespace: "myapp"}
		registryMu.Unlock()

		mock := &mockRoundTripper{}
		transport := NewHTTPClientMetricTransport(mock)
		So(transport, ShouldNotBeNil)

		// 发一个请求使指标产生数据
		req, _ := http.NewRequest("GET", "http://example.com/test", nil)
		transport.RoundTrip(req)

		metrics, err := defaultRegistry.Gather()
		So(err, ShouldBeNil)

		found := findMetricFamily(metrics, "myapp_http_client_requests_total")
		So(found, ShouldNotBeNil)
	})
}

func TestHTTPClientMetricTransport_RoundTrip_Success(t *testing.T) {
	PatchConvey("TestRoundTrip-成功请求记录200", t, func() {
		resetClientMetricState()

		mock := &mockRoundTripper{
			resp: &http.Response{StatusCode: 200},
		}
		transport := NewHTTPClientMetricTransport(mock)

		req, _ := http.NewRequest("GET", "http://api.example.com/users", nil)
		resp, err := transport.RoundTrip(req)

		So(err, ShouldBeNil)
		So(resp.StatusCode, ShouldEqual, 200)

		// 验证 counter
		metrics, gatherErr := defaultRegistry.Gather()
		So(gatherErr, ShouldBeNil)

		counterFamily := findMetricFamily(metrics, "http_client_requests_total")
		So(counterFamily, ShouldNotBeNil)
		So(len(counterFamily.Metric), ShouldEqual, 1)
		So(*counterFamily.Metric[0].Counter.Value, ShouldEqual, 1)
		So(findLabelValue(counterFamily.Metric[0], "method"), ShouldEqual, "GET")
		So(findLabelValue(counterFamily.Metric[0], "host"), ShouldEqual, "api.example.com")
		So(findLabelValue(counterFamily.Metric[0], "status"), ShouldEqual, "200")

		// 验证 histogram
		histFamily := findMetricFamily(metrics, "http_client_request_duration_ms")
		So(histFamily, ShouldNotBeNil)
		So(*histFamily.Metric[0].Histogram.SampleCount, ShouldEqual, 1)
	})
}

func TestHTTPClientMetricTransport_RoundTrip_Error(t *testing.T) {
	PatchConvey("TestRoundTrip-请求失败status为0", t, func() {
		resetClientMetricState()

		mock := &mockRoundTripper{
			resp: nil,
			err:  errors.New("connection refused"),
		}
		transport := NewHTTPClientMetricTransport(mock)

		req, _ := http.NewRequest("POST", "http://api.example.com/orders", nil)
		resp, err := transport.RoundTrip(req)

		So(resp, ShouldBeNil)
		So(err, ShouldNotBeNil)

		metrics, gatherErr := defaultRegistry.Gather()
		So(gatherErr, ShouldBeNil)

		counterFamily := findMetricFamily(metrics, "http_client_requests_total")
		So(counterFamily, ShouldNotBeNil)
		So(findLabelValue(counterFamily.Metric[0], "method"), ShouldEqual, "POST")
		So(findLabelValue(counterFamily.Metric[0], "status"), ShouldEqual, "0")
	})
}

func TestHTTPClientMetricTransport_RoundTrip_MultipleRequests(t *testing.T) {
	PatchConvey("TestRoundTrip-多次请求按标签聚合", t, func() {
		resetClientMetricState()

		mock200 := &mockRoundTripper{resp: &http.Response{StatusCode: 200}}
		mock500 := &mockRoundTripper{resp: &http.Response{StatusCode: 500}}

		transport200 := NewHTTPClientMetricTransport(mock200)
		transport500 := NewHTTPClientMetricTransport(mock500)

		// 3 次 GET 200
		for i := 0; i < 3; i++ {
			req, _ := http.NewRequest("GET", "http://api.example.com/users", nil)
			transport200.RoundTrip(req)
		}

		// 1 次 GET 500
		req, _ := http.NewRequest("GET", "http://api.example.com/users", nil)
		transport500.RoundTrip(req)

		metrics, err := defaultRegistry.Gather()
		So(err, ShouldBeNil)

		counterFamily := findMetricFamily(metrics, "http_client_requests_total")
		So(counterFamily, ShouldNotBeNil)
		So(len(counterFamily.Metric), ShouldEqual, 2) // 200 和 500 两组

		for _, m := range counterFamily.Metric {
			status := findLabelValue(m, "status")
			if status == "200" {
				So(*m.Counter.Value, ShouldEqual, 3)
			}
			if status == "500" {
				So(*m.Counter.Value, ShouldEqual, 1)
			}
		}

		// 验证 histogram 总计 4 次
		histFamily := findMetricFamily(metrics, "http_client_request_duration_ms")
		So(histFamily, ShouldNotBeNil)
		var totalCount uint64
		for _, m := range histFamily.Metric {
			totalCount += *m.Histogram.SampleCount
		}
		So(totalCount, ShouldEqual, 4)
	})
}

func TestHTTPClientMetricTransport_RoundTrip_DifferentHosts(t *testing.T) {
	PatchConvey("TestRoundTrip-不同host分开统计", t, func() {
		resetClientMetricState()

		mock := &mockRoundTripper{resp: &http.Response{StatusCode: 200}}
		transport := NewHTTPClientMetricTransport(mock)

		req1, _ := http.NewRequest("GET", "http://host-a.com/api", nil)
		transport.RoundTrip(req1)

		req2, _ := http.NewRequest("GET", "http://host-b.com/api", nil)
		transport.RoundTrip(req2)

		metrics, err := defaultRegistry.Gather()
		So(err, ShouldBeNil)

		counterFamily := findMetricFamily(metrics, "http_client_requests_total")
		So(counterFamily, ShouldNotBeNil)
		So(len(counterFamily.Metric), ShouldEqual, 2)

		hosts := make(map[string]float64)
		for _, m := range counterFamily.Metric {
			hosts[findLabelValue(m, "host")] = *m.Counter.Value
		}
		So(hosts["host-a.com"], ShouldEqual, 1)
		So(hosts["host-b.com"], ShouldEqual, 1)
	})
}

func TestHTTPClientMetricTransport_RoundTrip_DifferentMethods(t *testing.T) {
	PatchConvey("TestRoundTrip-不同HTTP方法分开统计", t, func() {
		resetClientMetricState()

		mock := &mockRoundTripper{resp: &http.Response{StatusCode: 200}}
		transport := NewHTTPClientMetricTransport(mock)

		reqGet, _ := http.NewRequest("GET", "http://example.com/api", nil)
		transport.RoundTrip(reqGet)

		reqPost, _ := http.NewRequest("POST", "http://example.com/api", nil)
		transport.RoundTrip(reqPost)

		reqPut, _ := http.NewRequest("PUT", "http://example.com/api", nil)
		transport.RoundTrip(reqPut)

		metrics, err := defaultRegistry.Gather()
		So(err, ShouldBeNil)

		counterFamily := findMetricFamily(metrics, "http_client_requests_total")
		So(counterFamily, ShouldNotBeNil)
		So(len(counterFamily.Metric), ShouldEqual, 3)

		methods := make(map[string]float64)
		for _, m := range counterFamily.Metric {
			methods[findLabelValue(m, "method")] = *m.Counter.Value
		}
		So(methods["GET"], ShouldEqual, 1)
		So(methods["POST"], ShouldEqual, 1)
		So(methods["PUT"], ShouldEqual, 1)
	})
}

func TestHTTPClientMetricTransport_RoundTrip_DurationRecorded(t *testing.T) {
	PatchConvey("TestRoundTrip-耗时记录到histogram", t, func() {
		resetClientMetricState()

		mock := &mockRoundTripper{resp: &http.Response{StatusCode: 200}}
		transport := NewHTTPClientMetricTransport(mock)

		req, _ := http.NewRequest("GET", "http://example.com/api", nil)
		transport.RoundTrip(req)

		metrics, err := defaultRegistry.Gather()
		So(err, ShouldBeNil)

		histFamily := findMetricFamily(metrics, "http_client_request_duration_ms")
		So(histFamily, ShouldNotBeNil)
		So(*histFamily.Metric[0].Histogram.SampleCount, ShouldEqual, 1)
		// 耗时应 >= 0（mockRoundTripper 几乎无延迟）
		So(*histFamily.Metric[0].Histogram.SampleSum, ShouldBeGreaterThanOrEqualTo, 0)
	})
}

func TestHTTPClientMetricTransport_RoundTrip_Exemplar(t *testing.T) {
	PatchConvey("TestRoundTrip-Exemplar携带path", t, func() {
		resetClientMetricState()

		mock := &mockRoundTripper{resp: &http.Response{StatusCode: 200}}
		transport := NewHTTPClientMetricTransport(mock)

		req, _ := http.NewRequest("GET", "http://api.example.com/users/123/orders", nil)
		transport.RoundTrip(req)

		metrics, err := defaultRegistry.Gather()
		So(err, ShouldBeNil)

		// 验证 counter exemplar 包含 path
		counterFamily := findMetricFamily(metrics, "http_client_requests_total")
		So(counterFamily, ShouldNotBeNil)
		exemplar := counterFamily.Metric[0].Counter.Exemplar
		So(exemplar, ShouldNotBeNil)

		exemplarLabels := make(map[string]string)
		for _, lp := range exemplar.Label {
			exemplarLabels[*lp.Name] = *lp.Value
		}
		So(exemplarLabels["path"], ShouldEqual, "/users/123/orders")
	})

	PatchConvey("TestRoundTrip-Exemplar无path无trace时exemplar为nil", t, func() {
		resetClientMetricState()

		mock := &mockRoundTripper{resp: &http.Response{StatusCode: 200}}
		transport := NewHTTPClientMetricTransport(mock)

		req, _ := http.NewRequest("GET", "http://api.example.com", nil)
		req.URL.Path = ""
		resp, err := transport.RoundTrip(req)

		So(err, ShouldBeNil)
		So(resp.StatusCode, ShouldEqual, 200)

		metrics, gatherErr := defaultRegistry.Gather()
		So(gatherErr, ShouldBeNil)

		counterFamily := findMetricFamily(metrics, "http_client_requests_total")
		So(counterFamily, ShouldNotBeNil)
		So(*counterFamily.Metric[0].Counter.Value, ShouldEqual, 1)
		So(counterFamily.Metric[0].Counter.Exemplar, ShouldBeNil)
	})
}

func TestBuildHTTPExemplar(t *testing.T) {
	PatchConvey("TestBuildHTTPExemplar-有path", t, func() {
		req, _ := http.NewRequest("GET", "http://example.com/api/users", nil)
		labels := buildHTTPExemplar(req)
		So(labels, ShouldNotBeNil)
		So(labels["path"], ShouldEqual, "/api/users")
	})

	PatchConvey("TestBuildHTTPExemplar-无path无trace返回nil", t, func() {
		req, _ := http.NewRequest("GET", "http://example.com", nil)
		req.URL.Path = ""
		labels := buildHTTPExemplar(req)
		So(labels, ShouldBeNil)
	})

	PatchConvey("TestBuildHTTPExemplar-长path截断到61字符", t, func() {
		longPath := "/api/v2/organizations/12345/projects/67890/resources/abcdef/actions/deploy/logs"
		req, _ := http.NewRequest("GET", "http://example.com"+longPath, nil)
		labels := buildHTTPExemplar(req)
		So(labels, ShouldNotBeNil)
		So(len([]rune(labels["path"])), ShouldBeLessThanOrEqualTo, maxExemplarPathLen)
		So(labels["path"], ShouldEqual, string([]rune(longPath)[:maxExemplarPathLen]))
	})

	PatchConvey("TestBuildHTTPExemplar-有traceID和spanID", t, func() {
		Mock(xutil.GetTraceIDFromCtx).Return("abc123def456").Build()
		Mock(xutil.GetSpanIDFromCtx).Return("span789").Build()

		req, _ := http.NewRequest("GET", "http://example.com/api", nil)
		labels := buildHTTPExemplar(req)
		So(labels, ShouldNotBeNil)
		So(labels["path"], ShouldEqual, "/api")
		So(labels["trace_id"], ShouldEqual, "abc123def456")
		So(labels["span_id"], ShouldEqual, "span789")
	})

	PatchConvey("TestBuildHTTPExemplar-长path加trace加span总rune不超128", t, func() {
		// W3C 标准：trace_id 32 hex chars, span_id 16 hex chars
		Mock(xutil.GetTraceIDFromCtx).Return("abcdef1234567890abcdef1234567890").Build()
		Mock(xutil.GetSpanIDFromCtx).Return("1234567890abcdef").Build()

		longPath := "/api/v2/organizations/12345/projects/67890/resources/abcdef/actions/deploy/logs"
		req, _ := http.NewRequest("GET", "http://example.com"+longPath, nil)
		labels := buildHTTPExemplar(req)
		So(labels, ShouldNotBeNil)

		// 计算总 rune 数，必须 <= 128 否则 Prometheus 会 panic
		var totalRunes int
		for k, v := range labels {
			totalRunes += len([]rune(k)) + len([]rune(v))
		}
		So(totalRunes, ShouldBeLessThanOrEqualTo, 128)
	})
}

func TestSafeExemplar_PanicRecovery(t *testing.T) {
	PatchConvey("TestSafeExemplar-panic时不崩溃", t, func() {
		So(func() {
			safeExemplar(func() { panic("exemplar validation failed") })
		}, ShouldNotPanic)
	})

	PatchConvey("TestSafeExemplar-正常执行", t, func() {
		called := false
		safeExemplar(func() { called = true })
		So(called, ShouldBeTrue)
	})
}

func TestRecordHTTPClientMetric_ExemplarOverLimit(t *testing.T) {
	PatchConvey("TestRecordHTTPClientMetric-exemplar超限不panic且指标正常记录", t, func() {
		resetClientMetricState()

		// 构造超长 trace_id 触发 exemplar 超 128 rune 上限
		Mock(xutil.GetTraceIDFromCtx).Return("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa").Build() // 52 chars
		Mock(xutil.GetSpanIDFromCtx).Return("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb").Build()                      // 32 chars

		req, _ := http.NewRequest("GET", "http://example.com/a]long/path/that/is/not/short", nil)
		So(func() {
			RecordHTTPClientMetric("GET", "example.com", "200", 50, req)
		}, ShouldNotPanic)

		// 指标应被正常记录（即使 exemplar 附加失败）
		metrics, err := defaultRegistry.Gather()
		So(err, ShouldBeNil)

		counterFamily := findMetricFamily(metrics, "http_client_requests_total")
		So(counterFamily, ShouldNotBeNil)
		So(*counterFamily.Metric[0].Counter.Value, ShouldEqual, 1)

		histFamily := findMetricFamily(metrics, "http_client_request_duration_ms")
		So(histFamily, ShouldNotBeNil)
		So(*histFamily.Metric[0].Histogram.SampleCount, ShouldEqual, 1)
	})
}

func TestInitClientMetricCollectors_Idempotent(t *testing.T) {
	PatchConvey("TestInitClientMetricCollectors-幂等性", t, func() {
		resetClientMetricState()

		// 多次调用不应 panic
		initClientMetricCollectors()
		first := clientRequestsTotal

		initClientMetricCollectors()
		second := clientRequestsTotal

		So(first, ShouldEqual, second) // sync.Once 保证同一实例
	})
}

func TestDefaultHttpDurationBuckets(t *testing.T) {
	PatchConvey("TestDefaultHttpDurationBuckets-桶边界正确", t, func() {
		expected := []float64{1, 5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000, 10000}
		So(defaultHttpDurationBuckets, ShouldResemble, expected)
	})
}

func TestSafeRegister_Export(t *testing.T) {
	PatchConvey("TestSafeRegister-导出函数正常工作", t, func() {
		resetClientMetricState()

		counter := prometheus.NewCounter(prometheus.CounterOpts{
			Name: "test_safe_register_counter",
			Help: "test",
		})

		// 第一次注册
		result := SafeRegister(counter)
		So(result, ShouldEqual, counter)

		// 重复注册应返回已有实例而非 panic
		result2 := SafeRegister(counter)
		So(result2, ShouldEqual, counter)
	})
}
