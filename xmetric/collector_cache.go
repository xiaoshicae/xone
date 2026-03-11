package xmetric

import (
	"strings"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/xiaoshicae/xone/v2/xutil"
)

var (
	collectors sync.Map   // map[string]prometheus.Collector
	createMu   sync.Mutex // 保护 collector 创建和注册的原子性
)

// parseTags 提取标签名和值，按 name 排序确保不同调用顺序生成相同的 cache key
func parseTags(tags []Tag) (names []string, values []string) {
	n := len(tags)
	if n == 0 {
		return nil, nil
	}

	// 复制后排序，避免修改调用方的 slice
	sorted := make([]Tag, n)
	copy(sorted, tags)
	for i := 1; i < len(sorted); i++ {
		for j := i; j > 0 && sorted[j].Name < sorted[j-1].Name; j-- {
			sorted[j], sorted[j-1] = sorted[j-1], sorted[j]
		}
	}

	names = make([]string, n)
	values = make([]string, n)
	for i, t := range sorted {
		names[i] = t.Name
		values[i] = t.Value
	}
	return
}

func buildCacheKey(metricType, name string, labelNames []string) string {
	return metricType + ":" + name + ":" + strings.Join(labelNames, ",")
}

// safeRegister 安全注册 collector，重复注册时复用已有实例而非 panic
func safeRegister(c prometheus.Collector) prometheus.Collector {
	registryMu.RLock()
	reg := defaultRegistry
	registryMu.RUnlock()

	err := reg.Register(c)
	if err == nil {
		return c
	}
	// 已注册则复用已有的 collector
	if are, ok := err.(prometheus.AlreadyRegisteredError); ok {
		return are.ExistingCollector
	}
	// 其他注册错误（如同名不同 labels），记录日志但不 panic
	xutil.ErrorIfEnableDebug("xmetric safeRegister failed, err=[%v]", err)
	return c
}

func getOrCreateCounter(name string, labelNames []string) *prometheus.CounterVec {
	key := buildCacheKey("c", name, labelNames)
	if v, ok := collectors.Load(key); ok {
		return v.(*prometheus.CounterVec)
	}

	createMu.Lock()
	defer createMu.Unlock()
	if v, ok := collectors.Load(key); ok {
		return v.(*prometheus.CounterVec)
	}

	counter := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: getNamespace(),
		Name:      name,
		Help:      name,
	}, labelNames)

	registered := safeRegister(counter)
	if cv, ok := registered.(*prometheus.CounterVec); ok {
		counter = cv
	}
	collectors.Store(key, counter)
	return counter
}

func getOrCreateGauge(name string, labelNames []string) *prometheus.GaugeVec {
	key := buildCacheKey("g", name, labelNames)
	if v, ok := collectors.Load(key); ok {
		return v.(*prometheus.GaugeVec)
	}

	createMu.Lock()
	defer createMu.Unlock()
	if v, ok := collectors.Load(key); ok {
		return v.(*prometheus.GaugeVec)
	}

	gauge := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: getNamespace(),
		Name:      name,
		Help:      name,
	}, labelNames)

	registered := safeRegister(gauge)
	if gv, ok := registered.(*prometheus.GaugeVec); ok {
		gauge = gv
	}
	collectors.Store(key, gauge)
	return gauge
}

func getOrCreateHistogram(name string, labelNames []string) *prometheus.HistogramVec {
	key := buildCacheKey("h", name, labelNames)
	if v, ok := collectors.Load(key); ok {
		return v.(*prometheus.HistogramVec)
	}

	createMu.Lock()
	defer createMu.Unlock()
	if v, ok := collectors.Load(key); ok {
		return v.(*prometheus.HistogramVec)
	}

	histogram := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: getNamespace(),
		Name:      name,
		Help:      name,
		Buckets:   getHistogramBuckets(),
	}, labelNames)

	registered := safeRegister(histogram)
	if hv, ok := registered.(*prometheus.HistogramVec); ok {
		histogram = hv
	}
	collectors.Store(key, histogram)
	return histogram
}
