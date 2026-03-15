package xmetric

import (
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
	"github.com/xiaoshicae/xone/v2/xutil"
)

// metricLogHook logrus Hook，在 Error 及以上级别触发时自动上报 metric
// label: level + caller（聚合维度）
// exemplar: trace_id + span_id（跳转链路追踪）
type metricLogHook struct {
	errorCounter *prometheus.CounterVec
}

func newMetricLogHook(ns string) *metricLogHook {
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

	return &metricLogHook{errorCounter: counter}
}

func (h *metricLogHook) Levels() []logrus.Level {
	return []logrus.Level{logrus.ErrorLevel, logrus.FatalLevel, logrus.PanicLevel}
}

func (h *metricLogHook) Fire(entry *logrus.Entry) error {
	level := entry.Level.String()
	caller := buildCaller(entry.Data)
	counter := h.errorCounter.WithLabelValues(level, caller)

	exemplar := buildExemplar(entry)
	if exemplar != nil {
		if adder, ok := counter.(prometheus.ExemplarAdder); ok {
			safeExemplar(func() { adder.AddWithExemplar(1, exemplar) })
			return nil
		}
	}

	counter.Inc()
	return nil
}

// buildCaller 从 entry.Data 提取日志位置，格式: filename:line
func buildCaller(data logrus.Fields) string {
	filename, _ := data["filename"].(string)
	lineid, _ := data["lineid"].(string)
	if filename != "" && lineid != "" {
		return filename + ":" + lineid
	}
	if filename != "" {
		return filename
	}
	return "unknown"
}

// buildExemplar 从 entry 中提取可用的 exemplar 标签
// trace_id: 关联链路追踪，Grafana 可点击跳转到 Jaeger/Tempo
// span_id: 定位具体 span 节点
// 无数据时返回 nil，避免高频场景下的空 map 分配
func buildExemplar(entry *logrus.Entry) prometheus.Labels {
	traceID := getStringField(entry.Data, "traceid")
	if traceID == "" && entry.Context != nil {
		traceID = xutil.GetTraceIDFromCtx(entry.Context)
	}

	spanID := getStringField(entry.Data, "spanid")
	if spanID == "" && entry.Context != nil {
		spanID = xutil.GetSpanIDFromCtx(entry.Context)
	}

	// 无数据直接返回 nil，避免空 map 分配
	if traceID == "" && spanID == "" {
		return nil
	}

	labels := make(prometheus.Labels, 2)
	if traceID != "" {
		labels["trace_id"] = traceID
	}
	if spanID != "" {
		labels["span_id"] = spanID
	}
	return labels
}

// getStringField 从 logrus.Fields 安全提取字符串值
func getStringField(data logrus.Fields, key string) string {
	v, ok := data[key]
	if !ok || v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprint(v)
}
