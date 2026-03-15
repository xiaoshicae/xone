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
// 注意：该 Transport 记录每次 HTTP 往返的指标，包括重试中间状态
// 如需只记录最终结果（跳过重试），请使用 RecordHTTPClientMetric 配合 Resty 中间件
func NewHTTPClientMetricTransport(next http.RoundTripper) *HTTPClientMetricTransport {
	if next == nil {
		next = http.DefaultTransport
	}
	return &HTTPClientMetricTransport{Next: next}
}

// RoundTrip 实现 http.RoundTripper 接口
func (t *HTTPClientMetricTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	start := time.Now()

	resp, err := t.Next.RoundTrip(req)

	status := "0"
	if resp != nil {
		status = strconv.Itoa(resp.StatusCode)
	}
	durationMs := float64(time.Since(start).Milliseconds())
	RecordHTTPClientMetric(req.Method, req.URL.Host, status, durationMs, req)

	return resp, err
}

// RecordHTTPClientMetric 记录 HTTP 出站请求指标
// 调用方决定记录时机（如 Resty OnSuccess/OnError 只记录最终结果）
func RecordHTTPClientMetric(method, host, status string, durationMs float64, req *http.Request) {
	initClientMetricCollectors()

	var exemplar prometheus.Labels
	if req != nil {
		exemplar = buildHTTPExemplar(req)
	}
	counter := clientRequestsTotal.WithLabelValues(method, host, status)
	histogram := clientRequestDuration.WithLabelValues(method, host, status)

	if exemplar != nil {
		if adder, ok := counter.(prometheus.ExemplarAdder); ok {
			safeExemplar(func() { adder.AddWithExemplar(1, exemplar) })
		} else {
			counter.Inc()
		}
		if observer, ok := histogram.(prometheus.ExemplarObserver); ok {
			safeExemplar(func() { observer.ObserveWithExemplar(durationMs, exemplar) })
		} else {
			histogram.Observe(durationMs)
		}
	} else {
		counter.Inc()
		histogram.Observe(durationMs)
	}
}

// safeExemplar 执行附带 exemplar 的指标记录
// Prometheus client 在 exemplar 验证失败时会 panic（如超 128 rune 上限）
// 此时指标值已由底层 Add/Observe 记录完成，仅 exemplar 附加失败，静默降级即可
func safeExemplar(fn func()) {
	defer func() { recover() }()
	fn()
}

// exemplar 标签总 rune 数上限为 128（Prometheus 硬性限制，超限会 panic）
// 预算：标签名 "path"(4) + "trace_id"(8) + "span_id"(7) = 19
//
//	trace_id 值(32) + span_id 值(16) = 48
//	path 值最大 = 128 - 19 - 48 = 61
const maxExemplarPathLen = 61

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
