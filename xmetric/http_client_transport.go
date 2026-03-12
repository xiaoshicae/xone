package xmetric

import (
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/xiaoshicae/xone/v2/xutil"
)

var (
	clientMetricOnce      sync.Once
	clientRequestsTotal   *prometheus.CounterVec
	clientRequestDuration *prometheus.HistogramVec
)

func initClientMetricCollectors() {
	clientMetricOnce.Do(func() {
		ns := GetConfig().Namespace

		cl := getConstLabels()

		counter := prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace:   ns,
			Name:        "http_client_requests_total",
			Help:        "HTTP 出站请求总数",
			ConstLabels: cl,
		}, []string{"method", "host", "status"})

		histogram := prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace:   ns,
			Name:        "http_client_request_duration_ms",
			Help:        "HTTP 出站请求耗时分布（毫秒）",
			Buckets:     getHttpDurationBuckets(),
			ConstLabels: cl,
		}, []string{"method", "host", "status"})

		if rc, ok := safeRegister(counter).(*prometheus.CounterVec); ok {
			counter = rc
		}
		if rh, ok := safeRegister(histogram).(*prometheus.HistogramVec); ok {
			histogram = rh
		}
		clientRequestsTotal = counter
		clientRequestDuration = histogram
	})
}

// HTTPClientMetricTransport 包装 http.RoundTripper，记录出站请求指标
type HTTPClientMetricTransport struct {
	// Next 是实际执行请求的 RoundTripper
	Next http.RoundTripper
}

// NewHTTPClientMetricTransport 创建出站请求 metric transport，注册指标采集器
func NewHTTPClientMetricTransport(next http.RoundTripper) *HTTPClientMetricTransport {
	initClientMetricCollectors()
	return &HTTPClientMetricTransport{Next: next}
}

// RoundTrip 实现 http.RoundTripper 接口
func (t *HTTPClientMetricTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	start := time.Now()
	method := req.Method
	host := req.URL.Host

	resp, err := t.Next.RoundTrip(req)

	durationMs := float64(time.Since(start).Milliseconds())
	status := "0"
	if resp != nil {
		status = strconv.Itoa(resp.StatusCode)
	}

	exemplar := buildHTTPExemplar(req)
	counter := clientRequestsTotal.WithLabelValues(method, host, status)
	histogram := clientRequestDuration.WithLabelValues(method, host, status)

	if exemplar != nil {
		if adder, ok := counter.(prometheus.ExemplarAdder); ok {
			adder.AddWithExemplar(1, exemplar)
		} else {
			counter.Inc()
		}
		if observer, ok := histogram.(prometheus.ExemplarObserver); ok {
			observer.ObserveWithExemplar(durationMs, exemplar)
		} else {
			histogram.Observe(durationMs)
		}
	} else {
		counter.Inc()
		histogram.Observe(durationMs)
	}

	return resp, err
}

// exemplar 标签总 rune 数上限为 128（Prometheus 硬性限制），path 需截断防止 panic
const maxExemplarPathLen = 64

// buildHTTPExemplar 从请求中提取 exemplar 标签（path + trace_id + span_id）
// 用于在 Grafana 中快速定位具体请求路径和链路
// 无数据时返回 nil，避免高频场景下的空 map 分配
func buildHTTPExemplar(req *http.Request) prometheus.Labels {
	path := req.URL.Path
	traceID := xutil.GetTraceIDFromCtx(req.Context())
	spanID := xutil.GetSpanIDFromCtx(req.Context())

	if path == "" && traceID == "" && spanID == "" {
		return nil
	}

	labels := make(prometheus.Labels, 3)
	if path != "" {
		if runeLen := len([]rune(path)); runeLen > maxExemplarPathLen {
			path = string([]rune(path)[:maxExemplarPathLen])
		}
		labels["path"] = path
	}
	if traceID != "" {
		labels["trace_id"] = traceID
	}
	if spanID != "" {
		labels["span_id"] = spanID
	}
	return labels
}
