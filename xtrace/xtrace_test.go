package xtrace

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/xiaoshicae/xone/v2/xconfig"
	"github.com/xiaoshicae/xone/v2/xutil"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"

	. "github.com/bytedance/mockey"
	. "github.com/smartystreets/goconvey/convey"
)

// ==================== config.go ====================

func TestConfigMergeDefault(t *testing.T) {
	PatchConvey("TestConfigMergeDefault", t, func() {
		PatchConvey("Nil", func() {
			c := configMergeDefault(nil)
			So(c.Enable, ShouldNotBeNil)
			So(*c.Enable, ShouldBeTrue)
			So(c.EnableConsole, ShouldBeFalse)
			So(c.SampleRatio, ShouldEqual, 1.0)
		})

		PatchConvey("ExistingValues", func() {
			enableFalse := false
			c := configMergeDefault(&Config{Enable: &enableFalse, EnableConsole: true, SampleRatio: 0.25})
			So(*c.Enable, ShouldBeFalse)
			So(c.EnableConsole, ShouldBeTrue)
			So(c.SampleRatio, ShouldEqual, 0.25)
		})

		PatchConvey("ForwardHeadersDefault", func() {
			c := configMergeDefault(&Config{})
			So(c.ForwardHeaders, ShouldBeNil)
		})

		PatchConvey("ForwardHeadersPreserved", func() {
			headers := []string{"X-Request-Id", "X-Tenant-Id"}
			c := configMergeDefault(&Config{ForwardHeaders: headers})
			So(c.ForwardHeaders, ShouldResemble, headers)
		})
	})
}

// ==================== util.go ====================

func TestEnableTrace(t *testing.T) {
	PatchConvey("TestEnableTrace", t, func() {
		defer func() {
			traceEnabled.Store(true)
			forwardHeaderEnabled.Store(false)
		}()

		PatchConvey("初始化前默认开启", func() {
			traceEnabled.Store(true)
			So(EnableTrace(), ShouldBeTrue)
		})

		PatchConvey("初始化写入 false 后关闭", func() {
			traceEnabled.Store(false)
			So(EnableTrace(), ShouldBeFalse)
		})

		PatchConvey("EnableForwardHeader 跟随初始化写入", func() {
			forwardHeaderEnabled.Store(false)
			So(EnableForwardHeader(), ShouldBeFalse)
			forwardHeaderEnabled.Store(true)
			So(EnableForwardHeader(), ShouldBeTrue)
		})
	})
}

// ==================== xtrace_init.go ====================

func TestGetTracer(t *testing.T) {
	PatchConvey("TestGetTracer", t, func() {
		tracer := GetTracer("test-tracer")
		So(tracer, ShouldNotBeNil)
	})
}

func TestGetConfig(t *testing.T) {
	PatchConvey("TestGetConfig", t, func() {
		PatchConvey("UnmarshalFail", func() {
			Mock(xconfig.UnmarshalConfig).Return(errors.New("unmarshal failed")).Build()
			config, err := getConfig()
			So(err, ShouldNotBeNil)
			So(config, ShouldBeNil)
		})

		PatchConvey("Success", func() {
			Mock(xconfig.UnmarshalConfig).Return(nil).Build()
			config, err := getConfig()
			So(err, ShouldBeNil)
			So(config, ShouldNotBeNil)
			So(*config.Enable, ShouldBeTrue)
		})
	})
}

func TestInitXTrace(t *testing.T) {
	PatchConvey("TestInitXTrace", t, func() {
		PatchConvey("GetConfigFail", func() {
			Mock(getConfig).Return(nil, errors.New("config failed")).Build()
			err := initXTrace()
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "getConfig failed")
		})

		PatchConvey("Disabled", func() {
			enableFalse := false
			Mock(getConfig).Return(&Config{Enable: &enableFalse}, nil).Build()
			err := initXTrace()
			So(err, ShouldBeNil)
		})

		PatchConvey("Enabled", func() {
			enableTrue := true
			Mock(getConfig).Return(&Config{Enable: &enableTrue}, nil).Build()
			Mock(xconfig.GetServerName).Return("test-svc").Build()
			Mock(xconfig.GetServerVersion).Return("v1.0.0").Build()
			Mock(xutil.InfoIfEnableDebug).Return().Build()
			Mock(initXTraceByConfig).Return(nil).Build()

			err := initXTrace()
			So(err, ShouldBeNil)
		})

		PatchConvey("EnabledButInitFail", func() {
			enableTrue := true
			Mock(getConfig).Return(&Config{Enable: &enableTrue}, nil).Build()
			Mock(xconfig.GetServerName).Return("test-svc").Build()
			Mock(xconfig.GetServerVersion).Return("v1.0.0").Build()
			Mock(xutil.InfoIfEnableDebug).Return().Build()
			Mock(initXTraceByConfig).Return(errors.New("init failed")).Build()

			err := initXTrace()
			So(err, ShouldNotBeNil)
		})
	})
}

func TestInitXTraceByConfig(t *testing.T) {
	PatchConvey("TestInitXTraceByConfig", t, func() {
		PatchConvey("Success", func() {
			err := initXTraceByConfig(&Config{}, "test-svc", "v1.0.0")
			So(err, ShouldBeNil)
		})

		PatchConvey("WithConsole", func() {
			err := initXTraceByConfig(&Config{EnableConsole: true}, "test-svc", "v1.0.0")
			So(err, ShouldBeNil)
		})

		PatchConvey("ResourceNewFail", func() {
			Mock(resource.New).Return(nil, errors.New("resource failed")).Build()
			err := initXTraceByConfig(&Config{}, "test-svc", "v1.0.0")
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "resource.New failed")
		})

		PatchConvey("ExporterFail", func() {
			Mock(stdouttrace.New).Return(nil, errors.New("exporter failed")).Build()
			err := initXTraceByConfig(&Config{EnableConsole: true}, "test-svc", "v1.0.0")
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "init exporter failed")
		})

		PatchConvey("WithForwardHeaders", func() {
			headers := []string{"X-Request-Id", "X-Tenant-Id"}
			err := initXTraceByConfig(&Config{ForwardHeaders: headers}, "test-svc", "v1.0.0")
			So(err, ShouldBeNil)
		})

		PatchConvey("WithEmptyForwardHeaders", func() {
			err := initXTraceByConfig(&Config{ForwardHeaders: []string{}}, "test-svc", "v1.0.0")
			So(err, ShouldBeNil)
		})
	})
}

func TestShutdownXTrace(t *testing.T) {
	PatchConvey("TestShutdownXTrace", t, func() {
		PatchConvey("Idempotent", func() {
			So(initXTraceByConfig(&Config{}, "test-svc", "v1.0.0"), ShouldBeNil)
			So(shutdownXTrace(), ShouldBeNil)
			// 第二次没有实例可关，直接返回 nil
			So(shutdownXTrace(), ShouldBeNil)
			So(tracerProvider, ShouldBeNil)
		})

		PatchConvey("NoProvider", func() {
			swapTracerProvider(nil)
			So(shutdownXTrace(), ShouldBeNil)
		})
	})
}

// ==================== 审查回归 ====================

// TestReinitShutsDownPrevious 重复初始化必须关闭上一个 TracerProvider，
// 否则旧实例连同其 SpanProcessor 一起被遗弃
func TestReinitShutsDownPrevious(t *testing.T) {
	PatchConvey("TestReinitShutsDownPrevious", t, func() {
		defer func() { _ = shutdownXTrace() }()

		So(initXTraceByConfig(&Config{}, "svc", "v1"), ShouldBeNil)
		first := tracerProvider
		So(first, ShouldNotBeNil)

		So(initXTraceByConfig(&Config{}, "svc", "v1"), ShouldBeNil)
		So(tracerProvider, ShouldNotEqual, first)

		// 旧实例已关闭：再次 Shutdown 仍返回 nil，但对其创建 Span 不再进入 Processor
		So(first.Shutdown(context.Background()), ShouldBeNil)
	})
}

// TestDisabledKeepsForwardHeaders Enable=false 不应让已配置的 Header 透传静默失效
func TestDisabledKeepsForwardHeaders(t *testing.T) {
	PatchConvey("TestDisabledKeepsForwardHeaders", t, func() {
		defer func() {
			_ = shutdownXTrace()
			traceEnabled.Store(true)
			forwardHeaderEnabled.Store(false)
		}()

		PatchConvey("配置了透传则保留 Propagator", func() {
			c := configMergeDefault(&Config{Enable: xutil.ToPtr(false), ForwardHeaders: []string{"X-Request-Id"}})
			So(initXTraceDisabled(c), ShouldBeNil)

			in := http.Header{}
			in.Set("X-Request-Id", "abc")
			ctx := otel.GetTextMapPropagator().Extract(context.Background(), propagation.HeaderCarrier(in))
			So(ForwardHeaderFromContext(ctx, "X-Request-Id"), ShouldEqual, "abc")
		})

		PatchConvey("未配置透传则清空 Propagator", func() {
			c := configMergeDefault(&Config{Enable: xutil.ToPtr(false)})
			So(initXTraceDisabled(c), ShouldBeNil)
			So(otel.GetTextMapPropagator().Fields(), ShouldBeEmpty)
		})
	})
}

// TestSamplerOf 采样率映射
func TestSamplerOf(t *testing.T) {
	PatchConvey("TestSamplerOf", t, func() {
		So(samplerOf(1).Description(), ShouldEqual, "AlwaysOnSampler")
		So(samplerOf(2).Description(), ShouldEqual, "AlwaysOnSampler")
		So(samplerOf(0.5).Description(), ShouldContainSubstring, "TraceIDRatioBased{0.5}")
	})
}

// TestSwapTracerProviderShutdownError 旧实例关闭失败只告警，不影响新实例生效
func TestSwapTracerProviderShutdownError(t *testing.T) {
	PatchConvey("TestSwapTracerProviderShutdownError", t, func() {
		defer func() { _ = shutdownXTrace() }()

		So(initXTraceByConfig(&Config{}, "svc", "v1"), ShouldBeNil)
		Mock((*trace.TracerProvider).Shutdown).Return(errors.New("shutdown failed")).Build()

		next := trace.NewTracerProvider()
		So(func() { swapTracerProvider(next) }, ShouldNotPanic)
		So(tracerProvider, ShouldEqual, next)
	})
}
