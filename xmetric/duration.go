package xmetric

import (
	"strings"
	"sync"
	"time"
)

// durationSuffix 耗时指标的名称后缀
//
// Prometheus 约定指标名自带单位，耗时统一用秒并以 _seconds 结尾；
// Grafana 面板与告警规则都依赖这一约定来推断单位。
const durationSuffix = "_seconds"

// ObserveDuration 记录一次耗时观测
//
// 相比直接用 HistogramObserve，它替调用方定掉了两件最容易出错的事：
// 用哪种指标类型（Histogram），以及传什么单位。入参是 time.Duration，
// 内部换算成秒——这与 HistogramObserve 的默认桶（prometheus.DefBuckets，
// 单位为秒）一致；若按毫秒传数值，所有样本都会落进 +Inf 桶，分位数直接失效。
//
// 指标名会自动补上 _seconds 后缀（已有则不重复添加）。
func ObserveDuration(name string, d time.Duration, tags ...Tag) {
	HistogramObserve(durationMetricName(name), d.Seconds(), tags...)
}

// Timer 开始计时，返回的函数在调用时记录从此刻起的耗时
//
//	defer xmetric.Timer("handle_order")()
//
// 标签在计时开始时即固定。若要按执行结果打标签（如 status=success/failed），
// 改用 ObserveDuration：
//
//	start := time.Now()
//	defer func() { xmetric.ObserveDuration("handle_order", time.Since(start), xmetric.T("status", status)) }()
func Timer(name string, tags ...Tag) func() {
	start := time.Now()
	return func() {
		ObserveDuration(name, time.Since(start), tags...)
	}
}

// durationMetricName 按 Prometheus 约定补上 _seconds 后缀
func durationMetricName(name string) string {
	if strings.HasSuffix(name, durationSuffix) {
		return name
	}
	return name + durationSuffix
}

// TrackInFlight 记录"当前正在进行中的数量"，返回的函数在调用时减回去
//
//	defer xmetric.TrackInFlight("active_requests", xmetric.T("api", "/order"))()
//
// GaugeInc / GaugeDec 必须成对出现，而早返回和 panic 路径上极容易漏掉 Dec——
// 漏一次计数就永久偏高，且不会有任何报错。交给 defer 才是天然正确的。
//
// 返回的函数重复调用只有首次生效，避免计数被减穿。
func TrackInFlight(name string, tags ...Tag) func() {
	GaugeInc(name, tags...)

	var once sync.Once
	return func() {
		once.Do(func() { GaugeDec(name, tags...) })
	}
}
