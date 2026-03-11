package xmetric

import "github.com/prometheus/client_golang/prometheus"

const XMetricConfigKey = "XMetric"

// Config xmetric 配置
type Config struct {
	// Namespace 指标命名空间前缀
	// optional default ""
	Namespace string `mapstructure:"Namespace"`

	// Path metrics 端点路径
	// optional default "/metrics"
	Path string `mapstructure:"Path"`

	// HistogramBuckets 直方图默认桶边界
	// optional default prometheus.DefBuckets
	HistogramBuckets []float64 `mapstructure:"HistogramBuckets"`
}

func configMergeDefault(c *Config) *Config {
	if c == nil {
		c = &Config{}
	}
	if c.Path == "" {
		c.Path = "/metrics"
	}
	if len(c.HistogramBuckets) == 0 {
		c.HistogramBuckets = prometheus.DefBuckets
	}
	return c
}
