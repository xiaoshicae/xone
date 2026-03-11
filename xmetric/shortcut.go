package xmetric

// Tag 指标标签键值对
type Tag struct {
	Name  string
	Value string
}

// T 创建标签
func T(name, value string) Tag {
	return Tag{Name: name, Value: value}
}

// CounterInc 递增计数器 +1
func CounterInc(name string, tags ...Tag) {
	names, values := parseTags(tags)
	getOrCreateCounter(name, names).WithLabelValues(values...).Inc()
}

// CounterAdd 递增计数器 +v（v 必须 >= 0）
func CounterAdd(name string, v float64, tags ...Tag) {
	names, values := parseTags(tags)
	getOrCreateCounter(name, names).WithLabelValues(values...).Add(v)
}

// GaugeSet 设置仪表盘值
func GaugeSet(name string, v float64, tags ...Tag) {
	names, values := parseTags(tags)
	getOrCreateGauge(name, names).WithLabelValues(values...).Set(v)
}

// GaugeInc 仪表盘 +1
func GaugeInc(name string, tags ...Tag) {
	names, values := parseTags(tags)
	getOrCreateGauge(name, names).WithLabelValues(values...).Inc()
}

// GaugeDec 仪表盘 -1
func GaugeDec(name string, tags ...Tag) {
	names, values := parseTags(tags)
	getOrCreateGauge(name, names).WithLabelValues(values...).Dec()
}

// HistogramObserve 直方图观测
func HistogramObserve(name string, v float64, tags ...Tag) {
	names, values := parseTags(tags)
	getOrCreateHistogram(name, names).WithLabelValues(values...).Observe(v)
}
