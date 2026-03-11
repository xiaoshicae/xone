package middleware

import (
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/xiaoshicae/xone/v2/xmetric"
)

// 默认耗时桶边界（毫秒）
var defaultDurationMsBuckets = []float64{1, 5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000, 10000}

var (
	metricOnce      sync.Once
	requestsTotal   *prometheus.CounterVec
	requestDuration *prometheus.HistogramVec
)

func initMetricCollectors() {
	metricOnce.Do(func() {
		ns := xmetric.GetConfig().Namespace

		counter := prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: ns,
			Name:      "http_requests_total",
			Help:      "HTTP 请求总数",
		}, []string{"method", "path", "status"})

		histogram := prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: ns,
			Name:      "http_request_duration_ms",
			Help:      "HTTP 请求耗时分布（毫秒）",
			Buckets:   defaultDurationMsBuckets,
		}, []string{"method", "path", "status"})

		if rc, ok := xmetric.SafeRegister(counter).(*prometheus.CounterVec); ok {
			counter = rc
		}
		if rh, ok := xmetric.SafeRegister(histogram).(*prometheus.HistogramVec); ok {
			histogram = rh
		}
		requestsTotal = counter
		requestDuration = histogram
	})
}

// GinXMetricMiddleware 返回 Gin HTTP 请求指标中间件
// 采集指标：http_requests_total（请求数量+状态码）、http_request_duration_ms（请求耗时+状态码）
func GinXMetricMiddleware() gin.HandlerFunc {
	initMetricCollectors()

	return func(c *gin.Context) {
		start := time.Now()

		c.Next()

		status := strconv.Itoa(c.Writer.Status())
		path := c.FullPath()
		if path == "" {
			path = "unknown"
		}
		method := c.Request.Method
		durationMs := float64(time.Since(start).Milliseconds())

		requestsTotal.WithLabelValues(method, path, status).Inc()
		requestDuration.WithLabelValues(method, path, status).Observe(durationMs)
	}
}
