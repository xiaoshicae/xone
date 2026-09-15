package xredis

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/redis/go-redis/v9"
	"github.com/xiaoshicae/xone/v3/xmetric"
)

// poolMetric 一个连接池指标的完整定义
//
// 用表驱动而不是为每个指标开一个结构体字段：后者要在常量、字段、
// 构造函数、Collect 四处各写一遍，加一个指标就得同步改四个地方
type poolMetric struct {
	name  string
	help  string
	typ   prometheus.ValueType
	value func(*redis.PoolStats) float64
}

// poolMetrics 连接池指标表
var poolMetrics = []poolMetric{
	{"redis_pool_connections_total_current", "当前连接池中的连接数（使用中 + 空闲）", prometheus.GaugeValue,
		func(s *redis.PoolStats) float64 { return float64(s.TotalConns) }},
	{"redis_pool_connections_idle", "当前空闲的连接数", prometheus.GaugeValue,
		func(s *redis.PoolStats) float64 { return float64(s.IdleConns) }},
	{"redis_pool_connections_stale_total", "因超时被移除的连接累计数", prometheus.CounterValue,
		func(s *redis.PoolStats) float64 { return float64(s.StaleConns) }},
	{"redis_pool_hits_total", "累计命中空闲连接的次数", prometheus.CounterValue,
		func(s *redis.PoolStats) float64 { return float64(s.Hits) }},
	{"redis_pool_misses_total", "累计未命中空闲连接的次数", prometheus.CounterValue,
		func(s *redis.PoolStats) float64 { return float64(s.Misses) }},
	{"redis_pool_timeouts_total", "累计等待连接超时的次数", prometheus.CounterValue,
		func(s *redis.PoolStats) float64 { return float64(s.Timeouts) }},
}

// poolCollector 在 scrape 时读取各连接池的实时状态
// 与 xgorm 同样采用拉模式：连接池状态是瞬时量，推模式会读到过期值
type poolCollector struct {
	descs    []*prometheus.Desc // 与 poolMetrics 一一对应
	statsFor func() map[string]*redis.PoolStats
}

func newPoolCollector() *poolCollector {
	ns := xmetric.GetConfig().Namespace
	labels := xmetric.GetConstLabels()

	descs := make([]*prometheus.Desc, len(poolMetrics))
	for i, m := range poolMetrics {
		descs[i] = prometheus.NewDesc(prometheus.BuildFQName(ns, "", m.name), m.help, []string{"name"}, labels)
	}
	return &poolCollector{descs: descs, statsFor: collectPoolStats}
}

func (c *poolCollector) Describe(ch chan<- *prometheus.Desc) {
	for _, d := range c.descs {
		ch <- d
	}
}

func (c *poolCollector) Collect(ch chan<- prometheus.Metric) {
	for name, s := range c.statsFor() {
		for i, m := range poolMetrics {
			ch <- prometheus.MustNewConstMetric(c.descs[i], m.typ, m.value(s), name)
		}
	}
}

// collectPoolStats 读取各命名连接池的状态
//
// 跳过 defaultClientName：它是某个具名 client 的别名，
// 不跳过会让同一个池子的指标出现两份、总量翻倍
func collectPoolStats() map[string]*redis.PoolStats {
	clientMu.RLock()
	defer clientMu.RUnlock()

	out := make(map[string]*redis.PoolStats, len(clientMap))
	for name, client := range clientMap {
		if name == defaultClientName || client == nil {
			continue
		}
		out[name] = client.PoolStats()
	}
	// 单 client 场景下只有 default 一个键，此时用它兜底
	if len(out) == 0 {
		if client := clientMap[defaultClientName]; client != nil {
			out[defaultClientName] = client.PoolStats()
		}
	}
	return out
}

// registerPoolMetrics 注册连接池指标
func registerPoolMetrics() {
	xmetric.SafeRegister(newPoolCollector())
}
