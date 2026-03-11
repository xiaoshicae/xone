package xmetric

import (
	"net/http"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	defaultRegistry = prometheus.NewRegistry()
	metricsHandler  http.Handler
	metricConfig    *Config
	registryMu      sync.RWMutex
)

// Registry 获取全局 Prometheus Registry
func Registry() *prometheus.Registry {
	registryMu.RLock()
	defer registryMu.RUnlock()
	return defaultRegistry
}

// Handler 获取 /metrics HTTP handler
func Handler() http.Handler {
	registryMu.RLock()
	defer registryMu.RUnlock()
	if metricsHandler != nil {
		return metricsHandler
	}
	// 兜底：未初始化时返回基于当前 registry 的 handler
	return promhttp.HandlerFor(defaultRegistry, promhttp.HandlerOpts{})
}

// MustRegister 注册自定义指标到全局 Registry
func MustRegister(cs ...prometheus.Collector) {
	registryMu.RLock()
	reg := defaultRegistry
	registryMu.RUnlock()
	reg.MustRegister(cs...)
}

// SafeRegister 安全注册 collector，重复注册时复用已有实例而非 panic
func SafeRegister(c prometheus.Collector) prometheus.Collector {
	return safeRegister(c)
}

// GetConfig 获取 xmetric 配置
func GetConfig() *Config {
	registryMu.RLock()
	defer registryMu.RUnlock()
	if metricConfig != nil {
		return metricConfig
	}
	return configMergeDefault(nil)
}

func getNamespace() string {
	registryMu.RLock()
	defer registryMu.RUnlock()
	if metricConfig != nil {
		return metricConfig.Namespace
	}
	return ""
}

// GetConstLabels 获取全局常量标签（供外部包使用）
func GetConstLabels() prometheus.Labels {
	return getConstLabels()
}

func getConstLabels() prometheus.Labels {
	registryMu.RLock()
	defer registryMu.RUnlock()
	if metricConfig == nil || len(metricConfig.ConstLabels) == 0 {
		return nil
	}
	// 返回浅拷贝，防止外部修改影响内部状态
	result := make(prometheus.Labels, len(metricConfig.ConstLabels))
	for k, v := range metricConfig.ConstLabels {
		result[k] = v
	}
	return result
}

// GetHttpDurationBuckets 获取 HTTP 请求耗时桶边界（毫秒），供 xgin middleware 等外部包使用
func GetHttpDurationBuckets() []float64 {
	return getHttpDurationBuckets()
}

func getHttpDurationBuckets() []float64 {
	registryMu.RLock()
	defer registryMu.RUnlock()
	if metricConfig != nil && len(metricConfig.HttpDurationBuckets) > 0 {
		return metricConfig.HttpDurationBuckets
	}
	return defaultHttpDurationBuckets
}

func getHistogramObserveBuckets() []float64 {
	registryMu.RLock()
	defer registryMu.RUnlock()
	if metricConfig != nil && len(metricConfig.HistogramObserveBuckets) > 0 {
		return metricConfig.HistogramObserveBuckets
	}
	return prometheus.DefBuckets
}
