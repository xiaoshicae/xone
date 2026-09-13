package xmetric

import (
	"strings"
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
