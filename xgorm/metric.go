package xgorm

import (
	"database/sql"

	"github.com/prometheus/client_golang/prometheus"
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
	value func(sql.DBStats) float64
}

// poolMetrics 连接池指标表，指标名按 Prometheus 约定以基准单位命名
var poolMetrics = []poolMetric{
	{"db_connections_open", "当前已建立的连接数（使用中 + 空闲）", prometheus.GaugeValue,
		func(s sql.DBStats) float64 { return float64(s.OpenConnections) }},
	{"db_connections_in_use", "当前正在使用的连接数", prometheus.GaugeValue,
		func(s sql.DBStats) float64 { return float64(s.InUse) }},
	{"db_connections_idle", "当前空闲的连接数", prometheus.GaugeValue,
		func(s sql.DBStats) float64 { return float64(s.Idle) }},
	{"db_connections_max_open", "连接数上限，0 表示不限制", prometheus.GaugeValue,
		func(s sql.DBStats) float64 { return float64(s.MaxOpenConnections) }},
	{"db_connections_wait_total", "累计等待连接的次数", prometheus.CounterValue,
		func(s sql.DBStats) float64 { return float64(s.WaitCount) }},
	{"db_connections_wait_duration_seconds_total", "累计等待连接的时长", prometheus.CounterValue,
		func(s sql.DBStats) float64 { return s.WaitDuration.Seconds() }},
	{"db_connections_closed_max_idle_total", "因超过空闲上限而关闭的连接累计数", prometheus.CounterValue,
		func(s sql.DBStats) float64 { return float64(s.MaxIdleTimeClosed) }},
	{"db_connections_closed_max_lifetime_total", "因超过存活时长而关闭的连接累计数", prometheus.CounterValue,
		func(s sql.DBStats) float64 { return float64(s.MaxLifetimeClosed) }},
}

// poolCollector 在 scrape 时读取各连接池的实时状态
//
// 实现 prometheus.Collector 而不是定时把值推进 Gauge：
// 连接池状态是瞬时量，推模式下采集间隔与推送间隔错开就会读到过期值，
// 而且需要额外一个后台协程
type poolCollector struct {
	descs    []*prometheus.Desc // 与 poolMetrics 一一对应
	statsFor func() map[string]sql.DBStats
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
func collectPoolStats() map[string]sql.DBStats {
	clientMu.RLock()
	defer clientMu.RUnlock()

	out := make(map[string]sql.DBStats, len(clientMap))
	for name, client := range clientMap {
		if name == defaultClientName {
			continue
		}
		db, err := client.DB()
		if err != nil || db == nil {
			continue
		}
		out[name] = db.Stats()
	}
	// 单 client 场景下只有 default 一个键，此时用它兜底
	if len(out) == 0 {
		if client := clientMap[defaultClientName]; client != nil {
			if db, err := client.DB(); err == nil && db != nil {
				out[defaultClientName] = db.Stats()
			}
		}
	}
	return out
}

// registerPoolMetrics 注册连接池指标
func registerPoolMetrics() {
	xmetric.SafeRegister(newPoolCollector())
}
