package xmetric

import (
	"cmp"
	"errors"
	"slices"
	"strings"
	"sync"

	"github.com/xiaoshicae/xone/v2/xutil"

	"github.com/prometheus/client_golang/prometheus"
)

// collector 缓存的类型前缀，确保同名不同类型的指标各自占一个 key
const (
	kindCounter   = "c"
	kindGauge     = "g"
	kindHistogram = "h"
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
	sorted := slices.Clone(tags)
	slices.SortStableFunc(sorted, func(a, b Tag) int { return cmp.Compare(a.Name, b.Name) })

	names = make([]string, n)
	values = make([]string, n)
	for i, t := range sorted {
		names[i] = t.Name
		values[i] = t.Value
	}
	return
}

// buildCacheKey 拼出 collector 缓存键，一次分配完成
func buildCacheKey(kind, name string, labelNames []string) string {
	size := len(kind) + 1 + len(name) + 1
	for i, l := range labelNames {
		if i > 0 {
			size++
		}
		size += len(l)
	}

	var b strings.Builder
	b.Grow(size)
	b.WriteString(kind)
	b.WriteByte(':')
	b.WriteString(name)
	b.WriteByte(':')
	for i, l := range labelNames {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(l)
	}
	return b.String()
}

// safeRegister 安全注册 collector，重复注册时复用已有实例而非 panic
func safeRegister(c prometheus.Collector) prometheus.Collector {
	err := Registry().Register(c)
	if err == nil {
		return c
	}
	// 已注册则复用已有的 collector
	var are prometheus.AlreadyRegisteredError
	if errors.As(err, &are) {
		return are.ExistingCollector
	}
	// 其他注册错误（如同名不同 labels），记录日志但不 panic
	xutil.ErrorIfEnableDebug("XOne xmetric safeRegister failed, err=[%v]", err)
	return c
}

// getOrCreateCollector 按 key 复用 collector，未命中时由 build 创建并注册
//
// 注册后类型与预期不符，说明同一个指标名已被注册成别的类型（例如先 Counter 后 Gauge）。
// 此时本实例不在 registry 中，记录的值永远不会被导出 —— 必须告警，
// 否则这是一次完全静默的数据丢失。
func getOrCreateCollector[T prometheus.Collector](key, name string, build func() T) T {
	if v, ok := collectors.Load(key); ok {
		if typed, ok := v.(T); ok {
			return typed
		}
	}

	createMu.Lock()
	defer createMu.Unlock()
	if v, ok := collectors.Load(key); ok {
		if typed, ok := v.(T); ok {
			return typed
		}
	}

	c := build()
	registered := safeRegister(c)
	if typed, ok := registered.(T); ok {
		c = typed
	} else {
		xutil.ErrorIfEnableDebug("XOne xmetric metric name conflict, name=[%s] is already registered as %T, "+
			"values recorded through this call will NOT be exported", name, registered)
	}

	collectors.Store(key, c)
	return c
}

func getOrCreateCounter(name string, labelNames []string) *prometheus.CounterVec {
	return getOrCreateCollector(buildCacheKey(kindCounter, name, labelNames), name,
		func() *prometheus.CounterVec {
			return prometheus.NewCounterVec(prometheus.CounterOpts{
				Namespace:   getNamespace(),
				Name:        name,
				Help:        name,
				ConstLabels: getConstLabels(),
			}, labelNames)
		})
}

func getOrCreateGauge(name string, labelNames []string) *prometheus.GaugeVec {
	return getOrCreateCollector(buildCacheKey(kindGauge, name, labelNames), name,
		func() *prometheus.GaugeVec {
			return prometheus.NewGaugeVec(prometheus.GaugeOpts{
				Namespace:   getNamespace(),
				Name:        name,
				Help:        name,
				ConstLabels: getConstLabels(),
			}, labelNames)
		})
}

func getOrCreateHistogram(name string, labelNames []string) *prometheus.HistogramVec {
	return getOrCreateCollector(buildCacheKey(kindHistogram, name, labelNames), name,
		func() *prometheus.HistogramVec {
			return prometheus.NewHistogramVec(prometheus.HistogramOpts{
				Namespace:   getNamespace(),
				Name:        name,
				Help:        name,
				Buckets:     getHistogramObserveBuckets(),
				ConstLabels: getConstLabels(),
			}, labelNames)
		})
}

// resetCollectors 清空 collector 缓存，供关闭时调用
//
// 不清的话，重新初始化后新的 Namespace / ConstLabels 对已缓存的指标不生效。
func resetCollectors() {
	collectors.Clear()
}
