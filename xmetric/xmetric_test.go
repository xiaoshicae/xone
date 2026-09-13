package xmetric

import (
	"context"
	"errors"
	"fmt"
	"github.com/xiaoshicae/xone/v2/xlog"
	"sync"
	"testing"
	"time"

	. "github.com/bytedance/mockey"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/xiaoshicae/xone/v2/xconfig"
	"github.com/xiaoshicae/xone/v2/xutil"
)

var errTest = errors.New("test error")

// resetState 重置全局状态，每个测试用例独立
func resetState() {
	registryMu.Lock()
	defaultRegistry = prometheus.NewRegistry()
	metricsHandler = nil
	metricConfig = nil
	registryMu.Unlock()
	collectors = sync.Map{}
}

func TestParseTags(t *testing.T) {
	PatchConvey("TestParseTags-正常解析", t, func() {
		names, values := parseTags([]Tag{T("method", "GET"), T("path", "/api")})
		So(names, ShouldResemble, []string{"method", "path"})
		So(values, ShouldResemble, []string{"GET", "/api"})
	})

	PatchConvey("TestParseTags-自动排序", t, func() {
		names, values := parseTags([]Tag{T("path", "/api"), T("method", "GET")})
		So(names, ShouldResemble, []string{"method", "path"})
		So(values, ShouldResemble, []string{"GET", "/api"})
	})

	PatchConvey("TestParseTags-空参数", t, func() {
		names, values := parseTags(nil)
		So(names, ShouldBeNil)
		So(values, ShouldBeNil)
	})

	PatchConvey("TestParseTags-单个标签", t, func() {
		names, values := parseTags([]Tag{T("method", "GET")})
		So(names, ShouldResemble, []string{"method"})
		So(values, ShouldResemble, []string{"GET"})
	})
}

func TestCounterInc(t *testing.T) {
	PatchConvey("TestCounterInc-正常递增", t, func() {
		resetState()

		CounterInc("request_total", T("method", "GET"))
		CounterInc("request_total", T("method", "GET"))
		CounterInc("request_total", T("method", "POST"))

		metrics, err := defaultRegistry.Gather()
		So(err, ShouldBeNil)
		So(len(metrics), ShouldEqual, 1)
		So(*metrics[0].Name, ShouldEqual, "request_total")
		So(len(metrics[0].Metric), ShouldEqual, 2)

		for _, m := range metrics[0].Metric {
			for _, l := range m.Label {
				if *l.Value == "GET" {
					So(*m.Counter.Value, ShouldEqual, 2)
				}
				if *l.Value == "POST" {
					So(*m.Counter.Value, ShouldEqual, 1)
				}
			}
		}
	})
}

func TestCounterAdd(t *testing.T) {
	PatchConvey("TestCounterAdd-正常累加", t, func() {
		resetState()

		CounterAdd("order_amount", 99.9, T("channel", "wechat"))
		CounterAdd("order_amount", 50.1, T("channel", "wechat"))

		metrics, err := defaultRegistry.Gather()
		So(err, ShouldBeNil)
		So(len(metrics), ShouldEqual, 1)
		So(*metrics[0].Metric[0].Counter.Value, ShouldEqual, 150.0)
	})
}

func TestGaugeSet(t *testing.T) {
	PatchConvey("TestGaugeSet-设置值", t, func() {
		resetState()

		GaugeSet("active_conns", 42, T("type", "ws"))
		GaugeSet("active_conns", 10, T("type", "ws"))

		metrics, err := defaultRegistry.Gather()
		So(err, ShouldBeNil)
		So(len(metrics), ShouldEqual, 1)
		So(*metrics[0].Metric[0].Gauge.Value, ShouldEqual, 10)
	})
}

func TestGaugeIncDec(t *testing.T) {
	PatchConvey("TestGaugeIncDec-递增递减", t, func() {
		resetState()

		GaugeInc("connections", T("type", "tcp"))
		GaugeInc("connections", T("type", "tcp"))
		GaugeDec("connections", T("type", "tcp"))

		metrics, err := defaultRegistry.Gather()
		So(err, ShouldBeNil)
		So(len(metrics), ShouldEqual, 1)
		So(*metrics[0].Metric[0].Gauge.Value, ShouldEqual, 1)
	})
}

func TestHistogramObserve(t *testing.T) {
	PatchConvey("TestHistogramObserve-观测值", t, func() {
		resetState()

		HistogramObserve("request_duration", 0.1, T("method", "GET"))
		HistogramObserve("request_duration", 0.5, T("method", "GET"))
		HistogramObserve("request_duration", 1.0, T("method", "GET"))

		metrics, err := defaultRegistry.Gather()
		So(err, ShouldBeNil)
		So(len(metrics), ShouldEqual, 1)
		So(*metrics[0].Metric[0].Histogram.SampleCount, ShouldEqual, 3)
		So(*metrics[0].Metric[0].Histogram.SampleSum, ShouldEqual, 1.6)
	})
}

func TestCounterInc_TagOrderIndependent(t *testing.T) {
	PatchConvey("TestCounterInc-不同Tag顺序复用同一指标", t, func() {
		resetState()

		CounterInc("request_total", T("method", "GET"), T("path", "/api"))
		CounterInc("request_total", T("path", "/api"), T("method", "GET"))

		metrics, err := defaultRegistry.Gather()
		So(err, ShouldBeNil)
		So(len(metrics), ShouldEqual, 1)
		So(*metrics[0].Metric[0].Counter.Value, ShouldEqual, 2)
	})
}

func TestCounterInc_NoTags(t *testing.T) {
	PatchConvey("TestCounterInc-无标签", t, func() {
		resetState()

		CounterInc("simple_counter")
		CounterInc("simple_counter")

		metrics, err := defaultRegistry.Gather()
		So(err, ShouldBeNil)
		So(len(metrics), ShouldEqual, 1)
		So(*metrics[0].Metric[0].Counter.Value, ShouldEqual, 2)
	})
}

func TestCounterInc_WithNamespace(t *testing.T) {
	PatchConvey("TestCounterInc-带命名空间", t, func() {
		resetState()
		registryMu.Lock()
		metricConfig = &Config{Namespace: "myapp"}
		registryMu.Unlock()

		CounterInc("request_total")

		metrics, err := defaultRegistry.Gather()
		So(err, ShouldBeNil)
		So(len(metrics), ShouldEqual, 1)
		So(*metrics[0].Name, ShouldEqual, "myapp_request_total")
	})
}

// gatherLogErrors 取出 log_errors_total 指标族
func gatherLogErrors(t *testing.T) *dto.MetricFamily {
	t.Helper()
	metrics, err := defaultRegistry.Gather()
	So(err, ShouldBeNil)
	for _, m := range metrics {
		if *m.Name == "log_errors_total" {
			return m
		}
	}
	return nil
}

// labelValue 取指定 label 的值
func labelValue(m *dto.Metric, name string) string {
	for _, l := range m.Label {
		if *l.Name == name {
			return *l.Value
		}
	}
	return ""
}

func TestMetricLogObserver_Observe(t *testing.T) {
	PatchConvey("TestMetricLogObserver-Error级别上报带caller", t, func() {
		resetState()

		obs := newMetricLogObserver("")
		obs.observe(context.Background(), xlog.Record{
			Level: xlog.ErrorLevel,
			File:  "order_handler.go",
			Line:  42,
		})

		found := gatherLogErrors(t)
		So(found, ShouldNotBeNil)
		So(*found.Metric[0].Counter.Value, ShouldEqual, 1)
		So(labelValue(found.Metric[0], "caller"), ShouldEqual, "order_handler.go:42")
		So(labelValue(found.Metric[0], "level"), ShouldEqual, "error")
	})

	PatchConvey("TestMetricLogObserver-无caller时fallback为unknown", t, func() {
		resetState()

		obs := newMetricLogObserver("")
		obs.observe(context.Background(), xlog.Record{Level: xlog.ErrorLevel})

		found := gatherLogErrors(t)
		So(found, ShouldNotBeNil)
		So(labelValue(found.Metric[0], "caller"), ShouldEqual, "unknown")
	})

	PatchConvey("TestMetricLogObserver-带TraceID和SpanID的Exemplar", t, func() {
		resetState()

		obs := newMetricLogObserver("")
		obs.observe(context.Background(), xlog.Record{
			Level:   xlog.ErrorLevel,
			File:    "pay_service.go",
			Line:    88,
			TraceID: "abc123",
			SpanID:  "def456",
		})

		found := gatherLogErrors(t)
		So(found, ShouldNotBeNil)
		So(*found.Metric[0].Counter.Value, ShouldEqual, 1)
	})

	PatchConvey("TestMetricLogObserver-Fatal与Panic同样上报", t, func() {
		// Level 数值越小级别越高，Fatal/Panic 需一并纳入
		resetState()

		obs := newMetricLogObserver("")
		obs.observe(context.Background(), xlog.Record{Level: xlog.FatalLevel, File: "a.go", Line: 1})
		obs.observe(context.Background(), xlog.Record{Level: xlog.PanicLevel, File: "a.go", Line: 1})

		found := gatherLogErrors(t)
		So(found, ShouldNotBeNil)
		So(len(found.Metric), ShouldEqual, 2)
	})

	PatchConvey("TestMetricLogObserver-低于Error的级别不上报", t, func() {
		resetState()

		obs := newMetricLogObserver("")
		obs.observe(context.Background(), xlog.Record{Level: xlog.WarnLevel, File: "a.go", Line: 1})
		obs.observe(context.Background(), xlog.Record{Level: xlog.InfoLevel, File: "a.go", Line: 1})
		obs.observe(context.Background(), xlog.Record{Level: xlog.DebugLevel, File: "a.go", Line: 1})

		So(gatherLogErrors(t), ShouldBeNil)
	})
}
func TestInitMetric(t *testing.T) {
	PatchConvey("TestInitMetric-正常初始化", t, func() {
		resetState()
		Mock(xconfig.UnmarshalConfig).Return(nil).Build()
		Mock(xconfig.ContainKey).Return(false).Build()

		err := initMetric()
		So(err, ShouldBeNil)
		So(Handler(), ShouldNotBeNil)
	})

	PatchConvey("TestInitMetric-自定义Namespace", t, func() {
		resetState()
		Mock(xconfig.UnmarshalConfig).To(func(key string, c any) error {
			cfg := c.(*Config)
			cfg.Namespace = "myapp"
			return nil
		}).Build()
		Mock(xconfig.ContainKey).Return(false).Build()

		err := initMetric()
		So(err, ShouldBeNil)
		So(getNamespace(), ShouldEqual, "myapp")
	})

	PatchConvey("TestInitMetric-配置读取失败", t, func() {
		resetState()
		Mock(xconfig.UnmarshalConfig).Return(errTest).Build()

		err := initMetric()
		So(err, ShouldNotBeNil)
	})
}

func TestCloseMetric(t *testing.T) {
	PatchConvey("TestCloseMetric-正常关闭", t, func() {
		resetState()
		registryMu.Lock()
		metricConfig = &Config{}
		registryMu.Unlock()

		err := closeMetric()
		So(err, ShouldBeNil)
		So(metricConfig, ShouldBeNil)
	})
}

func TestHandler(t *testing.T) {
	PatchConvey("TestHandler-未初始化时返回兜底handler", t, func() {
		resetState()

		h := Handler()
		So(h, ShouldNotBeNil)
	})
}

func TestGetConfig(t *testing.T) {
	PatchConvey("TestGetConfig-未初始化返回默认", t, func() {
		resetState()

		c := GetConfig()
		So(c, ShouldNotBeNil)
		So(c.Namespace, ShouldEqual, "")
	})
}

// ==================== 补充覆盖率：client.go ====================

func TestRegistry(t *testing.T) {
	PatchConvey("TestRegistry-返回全局registry", t, func() {
		resetState()
		reg := Registry()
		So(reg, ShouldNotBeNil)
		So(reg, ShouldEqual, defaultRegistry)
	})
}

func TestMustRegister(t *testing.T) {
	PatchConvey("TestMustRegister-正常注册", t, func() {
		resetState()

		counter := prometheus.NewCounter(prometheus.CounterOpts{
			Name: "test_must_register",
			Help: "test",
		})
		MustRegister(counter)

		metrics, err := defaultRegistry.Gather()
		So(err, ShouldBeNil)
		So(len(metrics), ShouldBeGreaterThan, 0)
	})
}

func TestGetHttpDurationBuckets(t *testing.T) {
	PatchConvey("TestGetHttpDurationBuckets-自定义桶", t, func() {
		resetState()
		customBuckets := []float64{5, 50, 500, 5000}
		registryMu.Lock()
		metricConfig = &Config{HttpDurationBuckets: customBuckets}
		registryMu.Unlock()

		result := getHttpDurationBuckets()
		So(result, ShouldResemble, customBuckets)
	})

	PatchConvey("TestGetHttpDurationBuckets-默认桶", t, func() {
		resetState()
		result := getHttpDurationBuckets()
		So(result, ShouldResemble, defaultHttpDurationBuckets)
	})

	PatchConvey("TestGetHttpDurationBuckets-导出函数", t, func() {
		resetState()
		result := GetHttpDurationBuckets()
		So(result, ShouldResemble, defaultHttpDurationBuckets)
	})
}

func TestGetHistogramObserveBuckets(t *testing.T) {
	PatchConvey("TestGetHistogramObserveBuckets-自定义桶", t, func() {
		resetState()
		customBuckets := []float64{10, 50, 100}
		registryMu.Lock()
		metricConfig = &Config{HistogramObserveBuckets: customBuckets}
		registryMu.Unlock()

		result := getHistogramObserveBuckets()
		So(result, ShouldResemble, customBuckets)
	})

	PatchConvey("TestGetHistogramObserveBuckets-默认桶", t, func() {
		resetState()
		result := getHistogramObserveBuckets()
		So(result, ShouldResemble, prometheus.DefBuckets)
	})
}

// ==================== 补充覆盖率：collector_cache.go ====================

func TestSafeRegister_ConflictError(t *testing.T) {
	PatchConvey("TestSafeRegister-同名不同labels记录日志不panic", t, func() {
		resetState()

		// 先注册一个带 label "a" 的 counter
		counter1 := prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "test_conflict",
			Help: "test",
		}, []string{"a"})
		safeRegister(counter1)

		// 再注册同名但 label 为 "b" 的 counter，触发非 AlreadyRegisteredError 的错误分支
		counter2 := prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "test_conflict",
			Help: "test",
		}, []string{"b"})
		result := safeRegister(counter2)
		// 回退返回 counter2 本身
		So(result, ShouldEqual, counter2)
	})
}

func TestGetOrCreateCounter_DoubleCheckLocking(t *testing.T) {
	PatchConvey("TestGetOrCreateCounter-双检查锁命中缓存", t, func() {
		resetState()

		name := "precached_counter"
		labels := []string{"tag"}
		key := buildCacheKey("c", name, labels)

		// 预填充缓存，模拟另一个 goroutine 已创建
		counter := prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: name,
			Help: name,
		}, labels)
		collectors.Store(key, counter)

		// getOrCreateCounter 应直接从缓存返回
		result := getOrCreateCounter(name, labels)
		So(result, ShouldEqual, counter)
	})

	PatchConvey("TestGetOrCreateCounter-锁内双检查命中", t, func() {
		resetState()

		name := "lock_check_counter"
		labels := []string{"tag"}
		key := buildCacheKey("c", name, labels)

		counter := prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: name,
			Help: name,
		}, labels)

		// 持有 createMu 锁，模拟另一个 goroutine 正在创建
		createMu.Lock()

		done := make(chan *prometheus.CounterVec, 1)
		go func() {
			// 此 goroutine 会在第一次 Load miss 后阻塞在 createMu.Lock()
			done <- getOrCreateCounter(name, labels)
		}()

		// 等 goroutine 阻塞在锁上，然后往缓存写入
		time.Sleep(50 * time.Millisecond)

		collectors.Store(key, counter)
		createMu.Unlock()

		result := <-done
		So(result, ShouldEqual, counter)
	})
}

func TestGetOrCreateGauge_DoubleCheckLocking(t *testing.T) {
	PatchConvey("TestGetOrCreateGauge-锁内双检查命中", t, func() {
		resetState()

		name := "lock_check_gauge"
		labels := []string{"tag"}
		key := buildCacheKey("g", name, labels)

		gauge := prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: name,
			Help: name,
		}, labels)

		createMu.Lock()

		done := make(chan *prometheus.GaugeVec, 1)
		go func() {
			done <- getOrCreateGauge(name, labels)
		}()

		// 等 goroutine 阻塞在锁上，然后往缓存写入
		time.Sleep(50 * time.Millisecond)

		collectors.Store(key, gauge)
		createMu.Unlock()

		result := <-done
		So(result, ShouldEqual, gauge)
	})
}

func TestGetOrCreateHistogram_DoubleCheckLocking(t *testing.T) {
	PatchConvey("TestGetOrCreateHistogram-锁内双检查命中", t, func() {
		resetState()

		name := "lock_check_histogram"
		labels := []string{"tag"}
		key := buildCacheKey("h", name, labels)

		histogram := prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    name,
			Help:    name,
			Buckets: prometheus.DefBuckets,
		}, labels)

		createMu.Lock()

		done := make(chan *prometheus.HistogramVec, 1)
		go func() {
			done <- getOrCreateHistogram(name, labels)
		}()

		// 等 goroutine 阻塞在锁上，然后往缓存写入
		time.Sleep(50 * time.Millisecond)

		collectors.Store(key, histogram)
		createMu.Unlock()

		result := <-done
		So(result, ShouldEqual, histogram)
	})
}

// ==================== 补充覆盖率：log_hook.go ====================

func TestBuildCaller(t *testing.T) {
	PatchConvey("TestBuildCaller-仅filename无行号", t, func() {
		So(buildCaller(xlog.Record{File: "handler.go"}), ShouldEqual, "handler.go")
	})

	PatchConvey("TestBuildCaller-完整位置", t, func() {
		So(buildCaller(xlog.Record{File: "handler.go", Line: 7}), ShouldEqual, "handler.go:7")
	})

	PatchConvey("TestBuildCaller-无位置信息", t, func() {
		So(buildCaller(xlog.Record{}), ShouldEqual, "unknown")
	})
}
func TestBuildExemplar(t *testing.T) {
	PatchConvey("TestBuildExemplar-无trace信息返回nil", t, func() {
		So(buildExemplar(xlog.Record{}), ShouldBeNil)
	})

	PatchConvey("TestBuildExemplar-仅TraceID", t, func() {
		labels := buildExemplar(xlog.Record{TraceID: "t1"})
		So(labels, ShouldResemble, prometheus.Labels{"trace_id": "t1"})
	})

	PatchConvey("TestBuildExemplar-TraceID与SpanID", t, func() {
		labels := buildExemplar(xlog.Record{TraceID: "t1", SpanID: "s1"})
		So(labels, ShouldResemble, prometheus.Labels{"trace_id": "t1", "span_id": "s1"})
	})
}
func TestConfigMergeDefault_BoolDefaults(t *testing.T) {
	PatchConvey("TestConfigMergeDefault-未配置bool字段默认true", t, func() {
		c := configMergeDefault(&Config{})
		So(*c.EnableGoMetrics, ShouldBeTrue)
		So(*c.EnableProcessMetrics, ShouldBeTrue)
		So(*c.EnableLogErrorMetric, ShouldBeTrue)
	})

	PatchConvey("TestConfigMergeDefault-显式配置false不被覆盖", t, func() {
		f := false
		c := configMergeDefault(&Config{
			EnableGoMetrics:      &f,
			EnableProcessMetrics: &f,
			EnableLogErrorMetric: &f,
		})
		So(*c.EnableGoMetrics, ShouldBeFalse)
		So(*c.EnableProcessMetrics, ShouldBeFalse)
		So(*c.EnableLogErrorMetric, ShouldBeFalse)
	})
}

// ==================== 补充覆盖率：closeMetric ====================

func TestCloseMetric_ResetsAllState(t *testing.T) {
	PatchConvey("TestCloseMetric-重置所有全局状态", t, func() {
		resetState()
		registryMu.Lock()
		metricConfig = &Config{
			Namespace:               "myapp",
			ConstLabels:             map[string]string{"env": "test"},
			HistogramObserveBuckets: []float64{1, 2, 3},
		}
		registryMu.Unlock()

		err := closeMetric()
		So(err, ShouldBeNil)
		So(metricConfig, ShouldBeNil)
		So(metricsHandler, ShouldBeNil)
		// 验证 getter 返回默认值
		So(getNamespace(), ShouldEqual, "")
		So(getConstLabels(), ShouldBeNil)
		So(getHistogramObserveBuckets(), ShouldResemble, prometheus.DefBuckets)
	})
}

func TestConstLabels(t *testing.T) {
	PatchConvey("TestConstLabels-Counter自动附加全局标签", t, func() {
		resetState()
		registryMu.Lock()
		metricConfig = &Config{ConstLabels: map[string]string{"env": "prod"}}
		registryMu.Unlock()

		CounterInc("order_total", T("channel", "wechat"))

		metrics, err := defaultRegistry.Gather()
		So(err, ShouldBeNil)

		var found *dto.MetricFamily
		for _, m := range metrics {
			if *m.Name == "order_total" {
				found = m
				break
			}
		}
		So(found, ShouldNotBeNil)

		// 验证 env 常量标签存在
		metric := found.Metric[0]
		envFound := false
		for _, l := range metric.Label {
			if *l.Name == "env" && *l.Value == "prod" {
				envFound = true
			}
		}
		So(envFound, ShouldBeTrue)
	})

	PatchConvey("TestConstLabels-Gauge自动附加全局标签", t, func() {
		resetState()
		registryMu.Lock()
		metricConfig = &Config{ConstLabels: map[string]string{"env": "test"}}
		registryMu.Unlock()

		GaugeSet("connections", 42, T("app", "chat"))

		metrics, err := defaultRegistry.Gather()
		So(err, ShouldBeNil)

		var found *dto.MetricFamily
		for _, m := range metrics {
			if *m.Name == "connections" {
				found = m
				break
			}
		}
		So(found, ShouldNotBeNil)

		envFound := false
		for _, l := range found.Metric[0].Label {
			if *l.Name == "env" && *l.Value == "test" {
				envFound = true
			}
		}
		So(envFound, ShouldBeTrue)
	})

	PatchConvey("TestConstLabels-Histogram自动附加全局标签", t, func() {
		resetState()
		registryMu.Lock()
		metricConfig = &Config{ConstLabels: map[string]string{"cluster": "cn-east"}}
		registryMu.Unlock()

		HistogramObserve("latency_ms", 10, T("api", "users"))

		metrics, err := defaultRegistry.Gather()
		So(err, ShouldBeNil)

		var found *dto.MetricFamily
		for _, m := range metrics {
			if *m.Name == "latency_ms" {
				found = m
				break
			}
		}
		So(found, ShouldNotBeNil)

		clusterFound := false
		for _, l := range found.Metric[0].Label {
			if *l.Name == "cluster" && *l.Value == "cn-east" {
				clusterFound = true
			}
		}
		So(clusterFound, ShouldBeTrue)
	})

	PatchConvey("TestConstLabels-nil时无额外标签", t, func() {
		resetState()

		CounterInc("simple_total", T("key", "val"))

		metrics, err := defaultRegistry.Gather()
		So(err, ShouldBeNil)

		var found *dto.MetricFamily
		for _, m := range metrics {
			if *m.Name == "simple_total" {
				found = m
				break
			}
		}
		So(found, ShouldNotBeNil)
		// 只有 key 一个标签
		So(len(found.Metric[0].Label), ShouldEqual, 1)
	})
}

func TestGetConstLabels(t *testing.T) {
	PatchConvey("TestGetConstLabels-返回全局常量标签", t, func() {
		resetState()
		registryMu.Lock()
		metricConfig = &Config{ConstLabels: map[string]string{"env": "prod"}}
		registryMu.Unlock()

		result := GetConstLabels()
		So(result, ShouldResemble, prometheus.Labels{"env": "prod"})
	})

	PatchConvey("TestGetConstLabels-未设置返回nil", t, func() {
		resetState()

		result := GetConstLabels()
		So(result, ShouldBeNil)
	})
}

// ==================== 审查回归 ====================

// TestMetricNameConflictWarns 同名不同类型时必须告警，不能静默丢数据
func TestMetricNameConflictWarns(t *testing.T) {
	PatchConvey("TestMetricNameConflictWarns", t, func() {
		resetState()

		var warned string
		Mock(xutil.ErrorIfEnableDebug).To(func(msg string, args ...any) {
			warned = fmt.Sprintf(msg, args...)
		}).Build()

		CounterInc("conflict_metric")
		GaugeSet("conflict_metric", 5)

		So(warned, ShouldContainSubstring, "metric name conflict")
		So(warned, ShouldContainSubstring, "conflict_metric")
		So(warned, ShouldContainSubstring, "will NOT be exported")
	})
}

// TestInitMetricIsRepeatable 重复初始化不能 panic
func TestInitMetricIsRepeatable(t *testing.T) {
	PatchConvey("TestInitMetricIsRepeatable", t, func() {
		resetState()
		Mock(getConfig).Return(configMergeDefault(nil), nil).Build()

		So(func() {
			So(initMetric(), ShouldBeNil)
			So(initMetric(), ShouldBeNil)
			So(initMetric(), ShouldBeNil)
		}, ShouldNotPanic)
	})
}

// TestCloseMetricClearsCollectorCache 关闭后重新初始化，新配置必须生效
func TestCloseMetricClearsCollectorCache(t *testing.T) {
	PatchConvey("TestCloseMetricClearsCollectorCache", t, func() {
		resetState()

		registryMu.Lock()
		metricConfig = configMergeDefault(&Config{Namespace: "ns1"})
		registryMu.Unlock()
		CounterInc("cached_metric")
		So(gatheredNames(), ShouldContain, "ns1_cached_metric")

		So(closeMetric(), ShouldBeNil)

		registryMu.Lock()
		metricConfig = configMergeDefault(&Config{Namespace: "ns2"})
		registryMu.Unlock()
		CounterInc("cached_metric")
		So(gatheredNames(), ShouldContain, "ns2_cached_metric")
	})
}

// TestConfigAccessorsReturnCopies 对外暴露的配置不能是内部实例
func TestConfigAccessorsReturnCopies(t *testing.T) {
	PatchConvey("TestConfigAccessorsReturnCopies", t, func() {
		resetState()
		registryMu.Lock()
		metricConfig = configMergeDefault(&Config{
			Namespace:   "orig",
			ConstLabels: map[string]string{"env": "prod"},
		})
		registryMu.Unlock()

		PatchConvey("GetConfig 返回深拷贝", func() {
			c := GetConfig()
			c.Namespace = "hacked"
			c.HttpDurationBuckets[0] = -999
			c.ConstLabels["env"] = "hacked"
			*c.EnableGoMetrics = false

			So(GetConfig().Namespace, ShouldEqual, "orig")
			So(getHttpDurationBuckets()[0], ShouldEqual, 1)
			So(GetConstLabels()["env"], ShouldEqual, "prod")
			So(*GetConfig().EnableGoMetrics, ShouldBeTrue)
		})

		PatchConvey("桶边界返回副本", func() {
			b := GetHttpDurationBuckets()
			b[0] = -999
			So(GetHttpDurationBuckets()[0], ShouldEqual, 1)

			hb := getHistogramObserveBuckets()
			hb[0] = -999
			So(getHistogramObserveBuckets()[0], ShouldNotEqual, -999)
		})

		PatchConvey("未初始化时也返回可安全改动的副本", func() {
			resetState()
			c := GetConfig()
			c.Namespace = "hacked"
			So(GetConfig().Namespace, ShouldBeEmpty)
			So(GetConfig().clone(), ShouldNotBeNil)
			So((*Config)(nil).clone(), ShouldBeNil)
		})
	})
}

// TestSafeExemplarWarns exemplar 被拒时要留痕，不能一声不吭
func TestSafeExemplarWarns(t *testing.T) {
	PatchConvey("TestSafeExemplarWarns", t, func() {
		var warned string
		Mock(xutil.WarnIfEnableDebug).To(func(msg string, args ...any) {
			warned = fmt.Sprintf(msg, args...)
		}).Build()

		So(func() { safeExemplar(func() { panic("exemplar too long") }) }, ShouldNotPanic)
		So(warned, ShouldContainSubstring, "exemplar rejected")
		So(warned, ShouldContainSubstring, "exemplar too long")
	})
}

// TestBuildCacheKey 缓存键按类型与标签名区分
func TestBuildCacheKey(t *testing.T) {
	PatchConvey("TestBuildCacheKey", t, func() {
		So(buildCacheKey(kindCounter, "n", nil), ShouldEqual, "c:n:")
		So(buildCacheKey(kindGauge, "n", []string{"a"}), ShouldEqual, "g:n:a")
		So(buildCacheKey(kindHistogram, "n", []string{"a", "b"}), ShouldEqual, "h:n:a,b")
		// 同名不同类型是两个 key
		So(buildCacheKey(kindCounter, "n", nil), ShouldNotEqual, buildCacheKey(kindGauge, "n", nil))
	})
}

// plainCounter 不实现 prometheus.ExemplarAdder 的计数器
type plainCounter struct {
	prometheus.Metric
	prometheus.Collector
	incCalls int
}

func (c *plainCounter) Inc()                   { c.incCalls++ }
func (c *plainCounter) Add(float64)            {}
func (c *plainCounter) Desc() *prometheus.Desc { return nil }

// plainObserver 不实现 prometheus.ExemplarObserver 的观测器
type plainObserver struct{ observed []float64 }

func (o *plainObserver) Observe(v float64) { o.observed = append(o.observed, v) }

// TestExemplarFallback 底层实现不支持 exemplar 时回落到普通记录
func TestExemplarFallback(t *testing.T) {
	PatchConvey("TestExemplarFallback", t, func() {
		exemplar := prometheus.Labels{"trace_id": "abc"}

		PatchConvey("计数器不支持 exemplar 时仍然计数", func() {
			c := &plainCounter{}
			addWithExemplar(c, exemplar)
			So(c.incCalls, ShouldEqual, 1)
		})

		PatchConvey("观测器不支持 exemplar 时仍然记录", func() {
			o := &plainObserver{}
			observeWithExemplar(o, 12.5, exemplar)
			So(o.observed, ShouldResemble, []float64{12.5})
		})

		PatchConvey("exemplar 为 nil 时走普通路径", func() {
			c := &plainCounter{}
			addWithExemplar(c, nil)
			So(c.incCalls, ShouldEqual, 1)

			o := &plainObserver{}
			observeWithExemplar(o, 1, nil)
			So(o.observed, ShouldResemble, []float64{1})
		})
	})
}
