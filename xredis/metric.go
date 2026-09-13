package xredis

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/redis/go-redis/v9"
	"github.com/xiaoshicae/xone/v2/xmetric"
)

// 连接池指标名，按 Prometheus 约定命名
const (
	metricRedisTotalConns = "redis_pool_connections_total_current"
	metricRedisIdleConns  = "redis_pool_connections_idle"
	metricRedisStaleConns = "redis_pool_connections_stale_total"
	metricRedisHits       = "redis_pool_hits_total"
	metricRedisMisses     = "redis_pool_misses_total"
	metricRedisTimeouts   = "redis_pool_timeouts_total"
)

// poolCollector 在 scrape 时读取各连接池的实时状态
// 与 xgorm 同样采用拉模式：连接池状态是瞬时量，推模式会读到过期值
type poolCollector struct {
	totalDesc   *prometheus.Desc
	idleDesc    *prometheus.Desc
	staleDesc   *prometheus.Desc
	hitsDesc    *prometheus.Desc
	missesDesc  *prometheus.Desc
	timeoutDesc *prometheus.Desc
	statsFor    func() map[string]*redis.PoolStats
}

func newPoolCollector() *poolCollector {
	ns := xmetric.GetConfig().Namespace
	labels := xmetric.GetConstLabels()
	newDesc := func(name, help string) *prometheus.Desc {
		return prometheus.NewDesc(prometheus.BuildFQName(ns, "", name), help, []string{"name"}, labels)
	}
	return &poolCollector{
		totalDesc:   newDesc(metricRedisTotalConns, "当前连接池中的连接数（使用中 + 空闲）"),
		idleDesc:    newDesc(metricRedisIdleConns, "当前空闲的连接数"),
		staleDesc:   newDesc(metricRedisStaleConns, "因超时被移除的连接累计数"),
		hitsDesc:    newDesc(metricRedisHits, "累计命中空闲连接的次数"),
		missesDesc:  newDesc(metricRedisMisses, "累计未命中空闲连接的次数"),
		timeoutDesc: newDesc(metricRedisTimeouts, "累计等待连接超时的次数"),
		statsFor:    collectPoolStats,
	}
}

func (c *poolCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.totalDesc
	ch <- c.idleDesc
	ch <- c.staleDesc
	ch <- c.hitsDesc
	ch <- c.missesDesc
	ch <- c.timeoutDesc
}

func (c *poolCollector) Collect(ch chan<- prometheus.Metric) {
	for name, s := range c.statsFor() {
		gauge := func(d *prometheus.Desc, v float64) {
			ch <- prometheus.MustNewConstMetric(d, prometheus.GaugeValue, v, name)
		}
		counter := func(d *prometheus.Desc, v float64) {
			ch <- prometheus.MustNewConstMetric(d, prometheus.CounterValue, v, name)
		}
		gauge(c.totalDesc, float64(s.TotalConns))
		gauge(c.idleDesc, float64(s.IdleConns))
		counter(c.staleDesc, float64(s.StaleConns))
		counter(c.hitsDesc, float64(s.Hits))
		counter(c.missesDesc, float64(s.Misses))
		counter(c.timeoutDesc, float64(s.Timeouts))
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
