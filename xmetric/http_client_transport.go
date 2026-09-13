package xmetric

import (
	"net/http"
	"strconv"
	"time"

	"github.com/xiaoshicae/xone/v2/xutil"

	"github.com/prometheus/client_golang/prometheus"
)

// 出站 HTTP 指标的固定标签与缓存键
var httpClientLabels = []string{"method", "host", "status"}

// httpClientCollectors 取出站请求的两个 collector
//
// 走统一的 collector 缓存而非 sync.Once：Once 一旦执行就把 Namespace、
// ConstLabels 和桶边界永久冻结在首次配置上，重新初始化后新配置不再生效。
func httpClientCollectors() (*prometheus.CounterVec, *prometheus.HistogramVec) {
	const (
		counterName   = "http_client_requests_total"
		histogramName = "http_client_request_duration_ms"
	)

	counter := getOrCreateCollector(buildCacheKey(kindCounter, counterName, httpClientLabels), counterName,
		func() *prometheus.CounterVec {
			return prometheus.NewCounterVec(prometheus.CounterOpts{
				Namespace:   getNamespace(),
				Name:        counterName,
				Help:        "HTTP 出站请求总数",
				ConstLabels: getConstLabels(),
			}, httpClientLabels)
		})

	histogram := getOrCreateCollector(buildCacheKey(kindHistogram, histogramName, httpClientLabels), histogramName,
		func() *prometheus.HistogramVec {
			return prometheus.NewHistogramVec(prometheus.HistogramOpts{
				Namespace:   getNamespace(),
				Name:        histogramName,
				Help:        "HTTP 出站请求耗时分布（毫秒）",
				Buckets:     getHttpDurationBuckets(),
				ConstLabels: getConstLabels(),
			}, httpClientLabels)
		})

	return counter, histogram
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
	counterVec, histogramVec := httpClientCollectors()

	var exemplar prometheus.Labels
	if req != nil {
		exemplar = buildHTTPExemplar(req)
	}
	counter := counterVec.WithLabelValues(method, host, status)
	histogram := histogramVec.WithLabelValues(method, host, status)

	addWithExemplar(counter, exemplar)
	observeWithExemplar(histogram, durationMs, exemplar)
}

// addWithExemplar 计数 +1，可用时附带 exemplar
func addWithExemplar(c prometheus.Counter, exemplar prometheus.Labels) {
	adder, ok := c.(prometheus.ExemplarAdder)
	if !ok || exemplar == nil {
		c.Inc()
		return
	}
	safeExemplar(func() { adder.AddWithExemplar(1, exemplar) })
}

// observeWithExemplar 记录观测值，可用时附带 exemplar
func observeWithExemplar(o prometheus.Observer, v float64, exemplar prometheus.Labels) {
	observer, ok := o.(prometheus.ExemplarObserver)
	if !ok || exemplar == nil {
		o.Observe(v)
		return
	}
	safeExemplar(func() { observer.ObserveWithExemplar(v, exemplar) })
}

// safeExemplar 执行附带 exemplar 的指标记录
//
// Prometheus client 在 exemplar 验证失败时会 panic（如超 128 rune 上限）。
// 此时指标值已由底层 Add/Observe 记录完成，只是 exemplar 没挂上，降级继续即可；
// 但不能一声不吭地吞掉——那样 exemplar 长期失效也无人知晓。
func safeExemplar(fn func()) {
	defer func() {
		if r := recover(); r != nil {
			xutil.WarnIfEnableDebug("XOne xmetric exemplar rejected, metric value still recorded, %v", r)
		}
	}()
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
	traceID, spanID := xutil.GetTraceAndSpanIDFromCtx(req.Context())

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
