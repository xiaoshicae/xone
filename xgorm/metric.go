package xgorm

import (
	"database/sql"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/xiaoshicae/xone/v2/xmetric"
)

// 连接池指标名，按 Prometheus 约定以基准单位命名
const (
	metricOpenConns     = "db_connections_open"
	metricInUseConns    = "db_connections_in_use"
	metricIdleConns     = "db_connections_idle"
	metricMaxOpenConns  = "db_connections_max_open"
	metricWaitCount     = "db_connections_wait_total"
	metricWaitDuration  = "db_connections_wait_duration_seconds_total"
	metricMaxIdleClosed = "db_connections_closed_max_idle_total"
	metricMaxLifeClosed = "db_connections_closed_max_lifetime_total"
)

// poolCollector 在 scrape 时读取各连接池的实时状态
//
// 实现 prometheus.Collector 而不是定时把值推进 Gauge：
// 连接池状态是瞬时量，推模式下采集间隔与推送间隔错开就会读到过期值，
// 而且需要额外一个后台协程
type poolCollector struct {
	openDesc    *prometheus.Desc
	inUseDesc   *prometheus.Desc
	idleDesc    *prometheus.Desc
	maxOpenDesc *prometheus.Desc
	waitDesc    *prometheus.Desc
	waitDurDesc *prometheus.Desc
	maxIdleDesc *prometheus.Desc
	maxLifeDesc *prometheus.Desc
	statsFor    func() map[string]sql.DBStats
}

func newPoolCollector() *poolCollector {
	ns := xmetric.GetConfig().Namespace
	labels := xmetric.GetConstLabels()
	newDesc := func(name, help string) *prometheus.Desc {
		return prometheus.NewDesc(prometheus.BuildFQName(ns, "", name), help, []string{"name"}, labels)
	}
	return &poolCollector{
		openDesc:    newDesc(metricOpenConns, "当前已建立的连接数（使用中 + 空闲）"),
		inUseDesc:   newDesc(metricInUseConns, "当前正在使用的连接数"),
		idleDesc:    newDesc(metricIdleConns, "当前空闲的连接数"),
		maxOpenDesc: newDesc(metricMaxOpenConns, "连接数上限，0 表示不限制"),
		waitDesc:    newDesc(metricWaitCount, "累计等待连接的次数"),
		waitDurDesc: newDesc(metricWaitDuration, "累计等待连接的时长"),
		maxIdleDesc: newDesc(metricMaxIdleClosed, "因超过空闲上限而关闭的连接累计数"),
		maxLifeDesc: newDesc(metricMaxLifeClosed, "因超过存活时长而关闭的连接累计数"),
		statsFor:    collectPoolStats,
	}
}

func (c *poolCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.openDesc
	ch <- c.inUseDesc
	ch <- c.idleDesc
	ch <- c.maxOpenDesc
	ch <- c.waitDesc
	ch <- c.waitDurDesc
	ch <- c.maxIdleDesc
	ch <- c.maxLifeDesc
}

func (c *poolCollector) Collect(ch chan<- prometheus.Metric) {
	for name, s := range c.statsFor() {
		gauge := func(d *prometheus.Desc, v float64) {
			ch <- prometheus.MustNewConstMetric(d, prometheus.GaugeValue, v, name)
		}
		counter := func(d *prometheus.Desc, v float64) {
			ch <- prometheus.MustNewConstMetric(d, prometheus.CounterValue, v, name)
		}
		gauge(c.openDesc, float64(s.OpenConnections))
		gauge(c.inUseDesc, float64(s.InUse))
		gauge(c.idleDesc, float64(s.Idle))
		gauge(c.maxOpenDesc, float64(s.MaxOpenConnections))
		counter(c.waitDesc, float64(s.WaitCount))
		counter(c.waitDurDesc, s.WaitDuration.Seconds())
		counter(c.maxIdleDesc, float64(s.MaxIdleTimeClosed))
		counter(c.maxLifeDesc, float64(s.MaxLifetimeClosed))
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
