package xmetric

import (
	"context"
	"strconv"

	"github.com/xiaoshicae/xone/v2/xlog"

	"github.com/prometheus/client_golang/prometheus"
)

// metricLogObserver 日志观察者，在 Error 及以上级别触发时自动上报 metric
// label: level + caller（聚合维度）
// exemplar: trace_id + span_id（跳转链路追踪）
type metricLogObserver struct {
	errorCounter *prometheus.CounterVec
}

func newMetricLogObserver(ns string) *metricLogObserver {
	counter := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace:   ns,
		Subsystem:   "log",
		Name:        "errors_total",
		Help:        "日志 Error 及以上级别总数",
		ConstLabels: getConstLabels(),
	}, []string{"level", "caller"})

	registered := safeRegister(counter)
	if cv, ok := registered.(*prometheus.CounterVec); ok {
		counter = cv
	}

	return &metricLogObserver{errorCounter: counter}
}

// observe 实现 xlog.Observer，仅处理 Error 及以上级别
// Level 数值越小级别越高，故此处以 ErrorLevel 为上界
func (h *metricLogObserver) observe(_ context.Context, r xlog.Record) {
	if r.Level > xlog.ErrorLevel {
		return
	}

	counter := h.errorCounter.WithLabelValues(r.Level.String(), buildCaller(r))
	addWithExemplar(counter, buildExemplar(r))
}

// buildCaller 提取日志位置，格式: filename:line
func buildCaller(r xlog.Record) string {
	if r.File == "" {
		return "unknown"
	}
	if r.Line <= 0 {
		return r.File
	}
	return r.File + ":" + strconv.Itoa(r.Line)
}

// buildExemplar 提取可用的 exemplar 标签
// trace_id: 关联链路追踪，Grafana 可点击跳转到 Jaeger/Tempo
// span_id: 定位具体 span 节点
// 无数据时返回 nil，避免高频场景下的空 map 分配
func buildExemplar(r xlog.Record) prometheus.Labels {
	if r.TraceID == "" && r.SpanID == "" {
		return nil
	}

	labels := make(prometheus.Labels, 2)
	if r.TraceID != "" {
		labels["trace_id"] = r.TraceID
	}
	if r.SpanID != "" {
		labels["span_id"] = r.SpanID
	}
	return labels
}
