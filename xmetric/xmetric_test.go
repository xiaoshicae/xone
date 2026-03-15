package xmetric

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	. "github.com/bytedance/mockey"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/sirupsen/logrus"
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

func TestMetricLogHook_Fire(t *testing.T) {
	PatchConvey("TestMetricLogHook-Error级别上报带caller", t, func() {
		resetState()

		hook := newMetricLogHook("")
		entry := logrus.NewEntry(logrus.StandardLogger())
		entry.Level = logrus.ErrorLevel
		entry.Context = context.Background()
		entry.Data["filename"] = "order_handler.go"
		entry.Data["lineid"] = "42"

		fireErr := hook.Fire(entry)
		So(fireErr, ShouldBeNil)

		metrics, gatherErr := defaultRegistry.Gather()
		So(gatherErr, ShouldBeNil)

		var found *dto.MetricFamily
		for _, m := range metrics {
			if *m.Name == "log_errors_total" {
				found = m
				break
			}
		}
		So(found, ShouldNotBeNil)
		So(*found.Metric[0].Counter.Value, ShouldEqual, 1)

		// 验证 caller label
		var callerLabel string
		for _, l := range found.Metric[0].Label {
			if *l.Name == "caller" {
				callerLabel = *l.Value
			}
		}
		So(callerLabel, ShouldEqual, "order_handler.go:42")
	})

	PatchConvey("TestMetricLogHook-无caller时fallback为unknown", t, func() {
		resetState()

		hook := newMetricLogHook("")
		entry := logrus.NewEntry(logrus.StandardLogger())
		entry.Level = logrus.ErrorLevel
		entry.Context = context.Background()

		fireErr := hook.Fire(entry)
		So(fireErr, ShouldBeNil)

		metrics, gatherErr := defaultRegistry.Gather()
		So(gatherErr, ShouldBeNil)

		var found *dto.MetricFamily
		for _, m := range metrics {
			if *m.Name == "log_errors_total" {
				found = m
				break
			}
		}
		So(found, ShouldNotBeNil)

		var callerLabel string
		for _, l := range found.Metric[0].Label {
			if *l.Name == "caller" {
				callerLabel = *l.Value
			}
		}
		So(callerLabel, ShouldEqual, "unknown")
	})

	PatchConvey("TestMetricLogHook-带TraceID和SpanID的Exemplar", t, func() {
		resetState()

		hook := newMetricLogHook("")
		entry := logrus.NewEntry(logrus.StandardLogger())
		entry.Level = logrus.ErrorLevel
		entry.Context = context.Background()
		entry.Data["filename"] = "pay_service.go"
		entry.Data["lineid"] = "88"
		entry.Data["traceid"] = "abc123"
		entry.Data["spanid"] = "def456"

		fireErr := hook.Fire(entry)
		So(fireErr, ShouldBeNil)

		metrics, gatherErr := defaultRegistry.Gather()
		So(gatherErr, ShouldBeNil)

		var found *dto.MetricFamily
		for _, m := range metrics {
			if *m.Name == "log_errors_total" {
				found = m
				break
			}
		}
		So(found, ShouldNotBeNil)
		So(*found.Metric[0].Counter.Value, ShouldEqual, 1)
	})

	PatchConvey("TestMetricLogHook-从ctx兜底获取traceID", t, func() {
		resetState()
		Mock(xutil.GetTraceIDFromCtx).Return("ctx_trace_id").Build()
		Mock(xutil.GetSpanIDFromCtx).Return("ctx_span_id").Build()

		hook := newMetricLogHook("")
		entry := logrus.NewEntry(logrus.StandardLogger())
		entry.Level = logrus.ErrorLevel
		entry.Context = context.Background()

		fireErr := hook.Fire(entry)
		So(fireErr, ShouldBeNil)

		metrics, gatherErr := defaultRegistry.Gather()
		So(gatherErr, ShouldBeNil)

		var found *dto.MetricFamily
		for _, m := range metrics {
			if *m.Name == "log_errors_total" {
				found = m
				break
			}
		}
		So(found, ShouldNotBeNil)
		So(*found.Metric[0].Counter.Value, ShouldEqual, 1)
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

func TestBuildCaller_FilenameOnly(t *testing.T) {
	PatchConvey("TestBuildCaller-仅filename无lineid", t, func() {
		data := logrus.Fields{"filename": "handler.go"}
		result := buildCaller(data)
		So(result, ShouldEqual, "handler.go")
	})
}

func TestGetStringField_NonString(t *testing.T) {
	PatchConvey("TestGetStringField-非字符串值转string", t, func() {
		data := logrus.Fields{"count": 42}
		result := getStringField(data, "count")
		So(result, ShouldEqual, "42")
	})

	PatchConvey("TestGetStringField-nil值返回空", t, func() {
		data := logrus.Fields{"key": nil}
		result := getStringField(data, "key")
		So(result, ShouldEqual, "")
	})

	PatchConvey("TestGetStringField-不存在的key返回空", t, func() {
		data := logrus.Fields{}
		result := getStringField(data, "missing")
		So(result, ShouldEqual, "")
	})
}

// ==================== 补充覆盖率：xmetric_init.go ====================

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

// ==================== 补充覆盖率：initMetric 多次调用不 panic ====================

func TestInitMetric_MultipleCallsNoPanic(t *testing.T) {
	PatchConvey("TestInitMetric-多次调用safeRegister不panic", t, func() {
		resetState()
		Mock(xconfig.UnmarshalConfig).Return(nil).Build()
		Mock(xconfig.ContainKey).Return(false).Build()

		// 第一次初始化
		err := initMetric()
		So(err, ShouldBeNil)

		// 重置 once 以允许再次初始化（模拟重启场景）
		resetState()

		// 第二次初始化不应 panic（safeRegister 替代 MustRegister）
		So(func() {
			err = initMetric()
		}, ShouldNotPanic)
		So(err, ShouldBeNil)
	})
}

// ==================== 补充覆盖率：closeMetric 重建 registry ====================

func TestCloseMetric_ResetsRegistryToNewInstance(t *testing.T) {
	PatchConvey("TestCloseMetric-registry为新实例", t, func() {
		resetState()
		registryMu.Lock()
		metricConfig = &Config{}
		registryMu.Unlock()

		oldRegistry := defaultRegistry

		err := closeMetric()
		So(err, ShouldBeNil)

		// 验证 defaultRegistry 是新实例
		registryMu.RLock()
		newRegistry := defaultRegistry
		registryMu.RUnlock()
		So(newRegistry, ShouldNotEqual, oldRegistry)
	})
}

func TestCloseMetric_ThenReinitialize(t *testing.T) {
	PatchConvey("TestCloseMetric-关闭后可重新初始化", t, func() {
		resetState()
		Mock(xconfig.UnmarshalConfig).Return(nil).Build()
		Mock(xconfig.ContainKey).Return(false).Build()

		// 初始化
		err := initMetric()
		So(err, ShouldBeNil)
		So(Handler(), ShouldNotBeNil)

		// 关闭
		err = closeMetric()
		So(err, ShouldBeNil)
		So(metricConfig, ShouldBeNil)

		// 重新初始化应成功
		err = initMetric()
		So(err, ShouldBeNil)
		So(Handler(), ShouldNotBeNil)
	})
}

// ==================== 补充覆盖率：getter 返回 slice 副本 ====================

func TestGetHttpDurationBuckets_ReturnsCopy(t *testing.T) {
	PatchConvey("TestGetHttpDurationBuckets-修改返回值不影响内部状态", t, func() {
		resetState()
		customBuckets := []float64{10, 50, 100}
		registryMu.Lock()
		metricConfig = &Config{HttpDurationBuckets: customBuckets}
		registryMu.Unlock()

		result := getHttpDurationBuckets()
		original := make([]float64, len(result))
		copy(original, result)

		// 修改返回的 slice
		result[0] = 99999

		// 再次获取，验证内部状态未被修改
		result2 := getHttpDurationBuckets()
		So(result2, ShouldResemble, original)
	})
}

func TestGetHistogramObserveBuckets_ReturnsCopy(t *testing.T) {
	PatchConvey("TestGetHistogramObserveBuckets-修改返回值不影响内部状态", t, func() {
		resetState()
		customBuckets := []float64{10, 50, 100}
		registryMu.Lock()
		metricConfig = &Config{HistogramObserveBuckets: customBuckets}
		registryMu.Unlock()

		result := getHistogramObserveBuckets()
		original := make([]float64, len(result))
		copy(original, result)

		// 修改返回的 slice
		result[0] = 99999

		// 再次获取，验证内部状态未被修改
		result2 := getHistogramObserveBuckets()
		So(result2, ShouldResemble, original)
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
