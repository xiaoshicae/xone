package middleware

import (
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/xiaoshicae/xone/v2/xmetric"
)

var (
	metricOnce      sync.Once
	requestsTotal   *prometheus.CounterVec
	requestDuration *prometheus.HistogramVec
)

func initMetricCollectors() {
	metricOnce.Do(func() {
		cfg := xmetric.GetConfig()
		ns := cfg.Namespace
		cl := xmetric.GetConstLabels()

		counter := prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace:   ns,
			Name:        "http_requests_total",
			Help:        "HTTP 请求总数",
			ConstLabels: cl,
		}, []string{"method", "path", "status"})

		histogram := prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace:   ns,
			Name:        "http_request_duration_seconds",
			Help:        "HTTP 请求耗时分布（秒）",
			Buckets:     xmetric.GetHttpDurationBuckets(),
			ConstLabels: cl,
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
// 采集指标：http_requests_total（请求数量+状态码）、http_request_duration_seconds（请求耗时+状态码）
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
		requestsTotal.WithLabelValues(method, path, status).Inc()
		// 用 Seconds() 而非 Milliseconds()：后者是整数截断，0.4ms 的请求会被记成 0
		requestDuration.WithLabelValues(method, path, status).Observe(time.Since(start).Seconds())
	}
}
