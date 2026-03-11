package xmetric

import (
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// 默认出站请求耗时桶边界（毫秒）
var defaultClientDurationMsBuckets = []float64{1, 5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000, 10000}

var (
	clientMetricOnce      sync.Once
	clientRequestsTotal   *prometheus.CounterVec
	clientRequestDuration *prometheus.HistogramVec
)

func initClientMetricCollectors() {
	clientMetricOnce.Do(func() {
		ns := GetConfig().Namespace

		counter := prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: ns,
			Name:      "http_client_requests_total",
			Help:      "HTTP 出站请求总数",
		}, []string{"method", "host", "status"})

		histogram := prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: ns,
			Name:      "http_client_request_duration_ms",
			Help:      "HTTP 出站请求耗时分布（毫秒）",
			Buckets:   defaultClientDurationMsBuckets,
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

	clientRequestsTotal.WithLabelValues(method, host, status).Inc()
	clientRequestDuration.WithLabelValues(method, host, status).Observe(durationMs)

	return resp, err
}
