package xmetric

import (
	"net/http"
	"slices"
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
	h := metricsHandler
	reg := defaultRegistry
	registryMu.RUnlock()
	if h != nil {
		return h
	}
	// 兜底：未初始化时返回基于当前 registry 的 handler
	//
	// 不缓存这个兜底实例：registry 本身可被替换，缓存会让它指向旧实例。
	// 这条路径只在初始化之前走到，代价可忽略。
	return promhttp.HandlerFor(reg, promhttp.HandlerOpts{})
}

// MustRegister 注册自定义指标到全局 Registry，重复注册会 panic
func MustRegister(cs ...prometheus.Collector) {
	Registry().MustRegister(cs...)
}

// SafeRegister 安全注册 collector，重复注册时复用已有实例而非 panic
func SafeRegister(c prometheus.Collector) prometheus.Collector {
	return safeRegister(c)
}

// GetConfig 获取 xmetric 配置的副本
//
// 返回副本而非内部实例：调用方拿到指针后改动字段或切片元素，
// 会直接污染全局配置。
func GetConfig() *Config {
	registryMu.RLock()
	c := metricConfig
	registryMu.RUnlock()
	if c == nil {
		return configMergeDefault(nil)
	}
	return c.clone()
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

// GetHttpDurationBuckets 获取 HTTP 请求耗时桶边界（秒），供 xgin middleware 等外部包使用
//
// 返回副本：调用方直接把它交给 prometheus.HistogramOpts，
// 若返回内部切片，任何越界写入都会污染全局配置。
func GetHttpDurationBuckets() []float64 {
	return getHttpDurationBuckets()
}

func getHttpDurationBuckets() []float64 {
	registryMu.RLock()
	defer registryMu.RUnlock()
	if metricConfig != nil && len(metricConfig.HttpDurationBuckets) > 0 {
		return slices.Clone(metricConfig.HttpDurationBuckets)
	}
	return slices.Clone(defaultHttpDurationBuckets)
}

func getHistogramObserveBuckets() []float64 {
	registryMu.RLock()
	defer registryMu.RUnlock()
	if metricConfig != nil && len(metricConfig.HistogramObserveBuckets) > 0 {
		return slices.Clone(metricConfig.HistogramObserveBuckets)
	}
	return slices.Clone(prometheus.DefBuckets)
}
