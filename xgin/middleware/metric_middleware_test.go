package middleware

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	. "github.com/bytedance/mockey"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/xiaoshicae/xone/v2/xmetric"
)

// resetMetricMiddlewareState 重置 metric 中间件全局状态
func resetMetricMiddlewareState() {
	metricOnce = sync.Once{}
	requestsTotal = nil
	requestDuration = nil
}

// findFamily 从 Gather 结果中查找指定名称的 MetricFamily
func findFamily(metrics []*dto.MetricFamily, name string) *dto.MetricFamily {
	for _, m := range metrics {
		if *m.Name == name {
			return m
		}
	}
	return nil
}

// labelValue 从 Metric 的 Label 中查找指定 name 的 value
func labelValue(metric *dto.Metric, name string) string {
	for _, l := range metric.Label {
		if *l.Name == name {
			return *l.Value
		}
	}
	return ""
}

func TestGinXMetricMiddleware_NormalRequest(t *testing.T) {
	PatchConvey("TestGinXMetricMiddleware-正常GET请求", t, func() {
		resetMetricMiddlewareState()
		Mock(xmetric.GetConfig).Return(&xmetric.Config{}).Build()
		Mock(xmetric.SafeRegister).To(func(c prometheus.Collector) prometheus.Collector {
			return c
		}).Build()

		gin.SetMode(gin.TestMode)
		r := gin.New()
		r.Use(GinXMetricMiddleware())
		r.GET("/api/users", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"status": "ok"})
		})

		req := httptest.NewRequest("GET", "/api/users", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		So(w.Code, ShouldEqual, http.StatusOK)

		// 验证 counter
		So(requestsTotal, ShouldNotBeNil)
		So(requestDuration, ShouldNotBeNil)
	})
}

func TestGinXMetricMiddleware_RecordsMetrics(t *testing.T) {
	PatchConvey("TestGinXMetricMiddleware-记录请求数和耗时", t, func() {
		resetMetricMiddlewareState()

		// 使用真实 registry 验证指标数据
		testRegistry := prometheus.NewRegistry()
		Mock(xmetric.GetConfig).Return(&xmetric.Config{}).Build()
		Mock(xmetric.SafeRegister).To(func(c prometheus.Collector) prometheus.Collector {
			testRegistry.MustRegister(c)
			return c
		}).Build()

		gin.SetMode(gin.TestMode)
		r := gin.New()
		r.Use(GinXMetricMiddleware())
		r.GET("/api/users", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"status": "ok"})
		})
		r.POST("/api/orders", func(c *gin.Context) {
			c.JSON(http.StatusCreated, gin.H{"id": 1})
		})

		// 发送 2 次 GET，1 次 POST
		for i := 0; i < 2; i++ {
			req := httptest.NewRequest("GET", "/api/users", nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			So(w.Code, ShouldEqual, http.StatusOK)
		}

		reqPost := httptest.NewRequest("POST", "/api/orders", nil)
		wPost := httptest.NewRecorder()
		r.ServeHTTP(wPost, reqPost)
		So(wPost.Code, ShouldEqual, http.StatusCreated)

		// 验证 counter
		metrics, err := testRegistry.Gather()
		So(err, ShouldBeNil)

		counterFamily := findFamily(metrics, "http_requests_total")
		So(counterFamily, ShouldNotBeNil)
		So(len(counterFamily.Metric), ShouldEqual, 2) // GET 和 POST 两组

		for _, m := range counterFamily.Metric {
			method := labelValue(m, "method")
			if method == "GET" {
				So(*m.Counter.Value, ShouldEqual, 2)
				So(labelValue(m, "path"), ShouldEqual, "/api/users")
				So(labelValue(m, "status"), ShouldEqual, "200")
			}
			if method == "POST" {
				So(*m.Counter.Value, ShouldEqual, 1)
				So(labelValue(m, "path"), ShouldEqual, "/api/orders")
				So(labelValue(m, "status"), ShouldEqual, "201")
			}
		}

		// 验证 histogram（共 3 次请求）
		histFamily := findFamily(metrics, "http_request_duration_ms")
		So(histFamily, ShouldNotBeNil)
		var totalCount uint64
		for _, m := range histFamily.Metric {
			totalCount += *m.Histogram.SampleCount
		}
		So(totalCount, ShouldEqual, 3)
	})
}

func TestGinXMetricMiddleware_StatusCodes(t *testing.T) {
	PatchConvey("TestGinXMetricMiddleware-不同状态码分开统计", t, func() {
		resetMetricMiddlewareState()

		testRegistry := prometheus.NewRegistry()
		Mock(xmetric.GetConfig).Return(&xmetric.Config{}).Build()
		Mock(xmetric.SafeRegister).To(func(c prometheus.Collector) prometheus.Collector {
			testRegistry.MustRegister(c)
			return c
		}).Build()

		gin.SetMode(gin.TestMode)
		r := gin.New()
		r.Use(GinXMetricMiddleware())
		r.GET("/api/ok", func(c *gin.Context) {
			c.JSON(http.StatusOK, nil)
		})
		r.GET("/api/error", func(c *gin.Context) {
			c.JSON(http.StatusInternalServerError, nil)
		})
		r.GET("/api/not-found", func(c *gin.Context) {
			c.JSON(http.StatusNotFound, nil)
		})

		// 200
		req := httptest.NewRequest("GET", "/api/ok", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		// 500
		req = httptest.NewRequest("GET", "/api/error", nil)
		w = httptest.NewRecorder()
		r.ServeHTTP(w, req)

		// 404
		req = httptest.NewRequest("GET", "/api/not-found", nil)
		w = httptest.NewRecorder()
		r.ServeHTTP(w, req)

		metrics, err := testRegistry.Gather()
		So(err, ShouldBeNil)

		counterFamily := findFamily(metrics, "http_requests_total")
		So(counterFamily, ShouldNotBeNil)
		So(len(counterFamily.Metric), ShouldEqual, 3) // 200, 404, 500

		statuses := make(map[string]float64)
		for _, m := range counterFamily.Metric {
			statuses[labelValue(m, "status")] = *m.Counter.Value
		}
		So(statuses["200"], ShouldEqual, 1)
		So(statuses["500"], ShouldEqual, 1)
		So(statuses["404"], ShouldEqual, 1)
	})
}

func TestGinXMetricMiddleware_UnknownPath(t *testing.T) {
	PatchConvey("TestGinXMetricMiddleware-未匹配路由path为unknown", t, func() {
		resetMetricMiddlewareState()

		testRegistry := prometheus.NewRegistry()
		Mock(xmetric.GetConfig).Return(&xmetric.Config{}).Build()
		Mock(xmetric.SafeRegister).To(func(c prometheus.Collector) prometheus.Collector {
			testRegistry.MustRegister(c)
			return c
		}).Build()

		gin.SetMode(gin.TestMode)
		r := gin.New()
		r.Use(GinXMetricMiddleware())
		// 不注册任何路由

		req := httptest.NewRequest("GET", "/not-exist", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		So(w.Code, ShouldEqual, http.StatusNotFound)

		metrics, err := testRegistry.Gather()
		So(err, ShouldBeNil)

		counterFamily := findFamily(metrics, "http_requests_total")
		So(counterFamily, ShouldNotBeNil)
		So(labelValue(counterFamily.Metric[0], "path"), ShouldEqual, "unknown")
	})
}

func TestGinXMetricMiddleware_WithNamespace(t *testing.T) {
	PatchConvey("TestGinXMetricMiddleware-带Namespace前缀", t, func() {
		resetMetricMiddlewareState()

		testRegistry := prometheus.NewRegistry()
		Mock(xmetric.GetConfig).Return(&xmetric.Config{Namespace: "myapp"}).Build()
		Mock(xmetric.SafeRegister).To(func(c prometheus.Collector) prometheus.Collector {
			testRegistry.MustRegister(c)
			return c
		}).Build()

		gin.SetMode(gin.TestMode)
		r := gin.New()
		r.Use(GinXMetricMiddleware())
		r.GET("/api/health", func(c *gin.Context) {
			c.String(http.StatusOK, "ok")
		})

		req := httptest.NewRequest("GET", "/api/health", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		metrics, err := testRegistry.Gather()
		So(err, ShouldBeNil)

		counterFamily := findFamily(metrics, "myapp_http_requests_total")
		So(counterFamily, ShouldNotBeNil)

		histFamily := findFamily(metrics, "myapp_http_request_duration_ms")
		So(histFamily, ShouldNotBeNil)
	})
}

func TestGinXMetricMiddleware_PathTemplate(t *testing.T) {
	PatchConvey("TestGinXMetricMiddleware-路径参数使用模板而非实际值", t, func() {
		resetMetricMiddlewareState()

		testRegistry := prometheus.NewRegistry()
		Mock(xmetric.GetConfig).Return(&xmetric.Config{}).Build()
		Mock(xmetric.SafeRegister).To(func(c prometheus.Collector) prometheus.Collector {
			testRegistry.MustRegister(c)
			return c
		}).Build()

		gin.SetMode(gin.TestMode)
		r := gin.New()
		r.Use(GinXMetricMiddleware())
		r.GET("/api/users/:id", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"id": c.Param("id")})
		})

		// 不同 id 参数的请求
		for _, id := range []string{"1", "2", "3"} {
			req := httptest.NewRequest("GET", "/api/users/"+id, nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			So(w.Code, ShouldEqual, http.StatusOK)
		}

		metrics, err := testRegistry.Gather()
		So(err, ShouldBeNil)

		counterFamily := findFamily(metrics, "http_requests_total")
		So(counterFamily, ShouldNotBeNil)
		// 应该只有 1 组（路由模板相同），不会因为不同 id 导致维度爆炸
		So(len(counterFamily.Metric), ShouldEqual, 1)
		So(*counterFamily.Metric[0].Counter.Value, ShouldEqual, 3)
		So(labelValue(counterFamily.Metric[0], "path"), ShouldEqual, "/api/users/:id")
	})
}

func TestGinXMetricMiddleware_DurationBuckets(t *testing.T) {
	PatchConvey("TestGinXMetricMiddleware-耗时桶边界正确", t, func() {
		expected := []float64{1, 5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000, 10000}
		So(xmetric.GetHttpDurationBuckets(), ShouldResemble, expected)
	})
}

func TestMetricsHandler(t *testing.T) {
	PatchConvey("TestMetricsHandler-返回有效handler", t, func() {
		Mock(xmetric.Handler).Return(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("metrics_output"))
		})).Build()

		handler := MetricsHandler()
		So(handler, ShouldNotBeNil)

		gin.SetMode(gin.TestMode)
		r := gin.New()
		r.GET("/metrics", handler)

		req := httptest.NewRequest("GET", "/metrics", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		So(w.Code, ShouldEqual, http.StatusOK)
		So(w.Body.String(), ShouldContainSubstring, "metrics_output")
	})
}

func TestInitMetricCollectors_Idempotent(t *testing.T) {
	PatchConvey("TestInitMetricCollectors-幂等性", t, func() {
		resetMetricMiddlewareState()
		Mock(xmetric.GetConfig).Return(&xmetric.Config{}).Build()
		Mock(xmetric.SafeRegister).To(func(c prometheus.Collector) prometheus.Collector {
			return c
		}).Build()

		initMetricCollectors()
		first := requestsTotal

		initMetricCollectors()
		second := requestsTotal

		So(first, ShouldNotBeNil)
		So(first, ShouldEqual, second) // sync.Once 保证同一实例
	})
}
