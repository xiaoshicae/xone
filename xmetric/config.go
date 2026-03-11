package xmetric

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/xiaoshicae/xone/v2/xutil"
)

const XMetricConfigKey = "XMetric"

// defaultHttpDurationBuckets HTTP 请求耗时默认桶边界（毫秒）
var defaultHttpDurationBuckets = []float64{1, 5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000, 10000}

// Config xmetric 配置
type Config struct {
	// Namespace 指标命名空间前缀
	// optional default ""
	Namespace string `mapstructure:"Namespace"`

	// ConstLabels 全局常量标签，自动附加到所有指标上
	// 典型用途：区分环境（env=prod）、集群（cluster=cn-east）等
	// optional default nil
	ConstLabels map[string]string `mapstructure:"ConstLabels"`

	// HttpDurationBuckets HTTP 入站/出站请求耗时 Histogram 的桶边界（毫秒）
	// 影响 http_request_duration_ms（xgin middleware）和 http_client_request_duration_ms（xhttp transport）
	// optional default [1, 5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000, 10000]
	HttpDurationBuckets []float64 `mapstructure:"HttpDurationBuckets"`

	// HistogramObserveBuckets 通过 HistogramObserve() API 创建的业务 Histogram 默认桶边界（秒）
	// 影响 xmetric.HistogramObserve() 调用创建的所有 Histogram 指标
	// optional default prometheus.DefBuckets
	HistogramObserveBuckets []float64 `mapstructure:"HistogramObserveBuckets"`

	// EnableGoMetrics 是否启用 Go runtime 指标（goroutine 数、GC 等）
	// optional default true
	EnableGoMetrics *bool `mapstructure:"EnableGoMetrics"`

	// EnableProcessMetrics 是否启用进程指标（CPU、内存、文件描述符等）
	// optional default true
	EnableProcessMetrics *bool `mapstructure:"EnableProcessMetrics"`

	// EnableLogErrorMetric 是否启用 xlog.Error 自动上报 metric（log_errors_total）
	// optional default true
	EnableLogErrorMetric *bool `mapstructure:"EnableLogErrorMetric"`
}

func configMergeDefault(c *Config) *Config {
	if c == nil {
		c = &Config{}
	}
	if len(c.HttpDurationBuckets) == 0 {
		c.HttpDurationBuckets = append([]float64(nil), defaultHttpDurationBuckets...)
	}
	if len(c.HistogramObserveBuckets) == 0 {
		c.HistogramObserveBuckets = append([]float64(nil), prometheus.DefBuckets...)
	}
	if c.EnableGoMetrics == nil {
		c.EnableGoMetrics = xutil.ToPtr(true)
	}
	if c.EnableProcessMetrics == nil {
		c.EnableProcessMetrics = xutil.ToPtr(true)
	}
	if c.EnableLogErrorMetric == nil {
		c.EnableLogErrorMetric = xutil.ToPtr(true)
	}
	return c
}
