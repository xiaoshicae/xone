package xtrace

import (
	"errors"
	"testing"

	"github.com/xiaoshicae/xone/v2/xconfig"
	"github.com/xiaoshicae/xone/v2/xutil"

	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/sdk/resource"

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
			So(c.Console, ShouldBeFalse)
		})

		PatchConvey("ExistingValues", func() {
			enableFalse := false
			c := configMergeDefault(&Config{Enable: &enableFalse, Console: true})
			So(*c.Enable, ShouldBeFalse)
			So(c.Console, ShouldBeTrue)
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
		PatchConvey("DefaultTrue", func() {
			MockValue(&traceEnabled).To(true)
			So(EnableTrace(), ShouldBeTrue)
		})

		PatchConvey("ExplicitFalse", func() {
			MockValue(&traceEnabled).To(false)
			So(EnableTrace(), ShouldBeFalse)
		})

		PatchConvey("ExplicitTrue", func() {
			MockValue(&traceEnabled).To(true)
			So(EnableTrace(), ShouldBeTrue)
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
			err := initXTraceByConfig(&Config{Console: false}, "test-svc", "v1.0.0")
			So(err, ShouldBeNil)
		})

		PatchConvey("WithConsole", func() {
			err := initXTraceByConfig(&Config{Console: true}, "test-svc", "v1.0.0")
			So(err, ShouldBeNil)
		})

		PatchConvey("ResourceNewFail", func() {
			Mock(resource.New).Return(nil, errors.New("resource failed")).Build()
			err := initXTraceByConfig(&Config{Console: false}, "test-svc", "v1.0.0")
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "resource.New failed")
		})

		PatchConvey("ExporterFail", func() {
			Mock(stdouttrace.New).Return(nil, errors.New("exporter failed")).Build()
			err := initXTraceByConfig(&Config{Console: true}, "test-svc", "v1.0.0")
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
			shutdownExecuted.Store(false)
			calls := 0
			xTraceShutdownFunc = func() error {
				calls++
				return nil
			}
			So(shutdownXTrace(), ShouldBeNil)
			So(shutdownXTrace(), ShouldBeNil)
			So(calls, ShouldEqual, 1)
		})

		PatchConvey("NilFunc", func() {
			// fn == nil 路径
			shutdownExecuted.Store(false)
			xTraceShutdownFunc = nil
			So(shutdownXTrace(), ShouldBeNil)
		})

		PatchConvey("AfterInit", func() {
			// 覆盖 shutdown 闭包内 tp.Shutdown 的执行
			shutdownExecuted.Store(false)
			xTraceShutdownFunc = nil
			err := initXTraceByConfig(&Config{Console: false}, "test-svc", "v1.0.0")
			So(err, ShouldBeNil)
			// initXTraceByConfig 设置了 xTraceShutdownFunc，执行 shutdown
			So(shutdownXTrace(), ShouldBeNil)
		})
	})
}

// ==================== util.go (traceEnabled 从模块变量读取) ====================

func TestEnableTrace_ModuleVariable(t *testing.T) {
	PatchConvey("TestEnableTrace_ModuleVariable", t, func() {
		PatchConvey("DefaultTrue", func() {
			// traceEnabled 默认值为 true
			MockValue(&traceEnabled).To(true)
			So(EnableTrace(), ShouldBeTrue)
		})

		PatchConvey("SetToFalse", func() {
			MockValue(&traceEnabled).To(false)
			So(EnableTrace(), ShouldBeFalse)
		})

		PatchConvey("SetToTrue", func() {
			MockValue(&traceEnabled).To(true)
			So(EnableTrace(), ShouldBeTrue)
		})
	})
}

// ==================== xtrace_init.go (initXTrace 设置 traceEnabled) ====================

func TestInitXTrace_SetsTraceEnabled(t *testing.T) {
	PatchConvey("TestInitXTrace_SetsTraceEnabled", t, func() {
		PatchConvey("DisabledSetsTraceEnabledFalse", func() {
			enableFalse := false
			Mock(getConfig).Return(&Config{Enable: &enableFalse}, nil).Build()

			err := initXTrace()
			So(err, ShouldBeNil)
			So(EnableTrace(), ShouldBeFalse)
		})

		PatchConvey("EnabledSetsTraceEnabledTrue", func() {
			enableTrue := true
			Mock(getConfig).Return(&Config{Enable: &enableTrue}, nil).Build()
			Mock(xconfig.GetServerName).Return("test-svc").Build()
			Mock(xconfig.GetServerVersion).Return("v1.0.0").Build()
			Mock(xutil.InfoIfEnableDebug).Return().Build()
			Mock(initXTraceByConfig).Return(nil).Build()

			err := initXTrace()
			So(err, ShouldBeNil)
			So(EnableTrace(), ShouldBeTrue)
		})
	})
}

// ==================== xtrace_init.go (shutdown 超时) ====================

func TestShutdownXTrace_WithTimeout(t *testing.T) {
	PatchConvey("TestShutdownXTrace_WithTimeout", t, func() {
		PatchConvey("ShutdownFuncUsesTimeout", func() {
			// initXTraceByConfig 创建的 shutdown 闭包使用 10s 超时
			shutdownExecuted.Store(false)
			xTraceShutdownFunc = nil
			err := initXTraceByConfig(&Config{Console: false}, "test-svc", "v1.0.0")
			So(err, ShouldBeNil)
			// xTraceShutdownFunc 不为 nil，说明已设置
			So(xTraceShutdownFunc, ShouldNotBeNil)
			// 执行 shutdown，应成功
			err = shutdownXTrace()
			So(err, ShouldBeNil)
			// 再次调用应跳过（幂等）
			err = shutdownXTrace()
			So(err, ShouldBeNil)
		})
	})
}
