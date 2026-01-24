package xtrace

import (
	"context"
	"errors"
	"log"
	"testing"
	"time"

	"github.com/xiaoshicae/xone/xconfig"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"

	. "github.com/bytedance/mockey"
	. "github.com/smartystreets/goconvey/convey"
)

func TestXTraceConfig(t *testing.T) {
	PatchConvey("TestXTraceConfig-configMergeDefault-Param-Nil", t, func() {
		c := configMergeDefault(nil)
		So(c.Enable, ShouldNotBeNil)
		So(*c.Enable, ShouldBeTrue)
		So(c.Console, ShouldBeFalse)
	})
	PatchConvey("TestXTraceConfig-configMergeDefault-Param-Exist", t, func() {
		enableFalse := false
		c := configMergeDefault(&Config{
			Enable:  &enableFalse,
			Console: true,
		})
		So(c.Enable, ShouldNotBeNil)
		So(*c.Enable, ShouldBeFalse)
		So(c.Console, ShouldBeTrue)
	})
}

func TestEnableTrace(t *testing.T) {
	PatchConvey("TestEnableTrace-default enabled", t, func() {
		Mock(xconfig.GetString).Return("").Build()
		So(EnableTrace(), ShouldBeTrue)
	})

	PatchConvey("TestEnableTrace-explicit false", t, func() {
		Mock(xconfig.GetString).Return("false").Build()
		So(EnableTrace(), ShouldBeFalse)
	})

	PatchConvey("TestEnableTrace-explicit true", t, func() {
		Mock(xconfig.GetString).Return("true").Build()
		So(EnableTrace(), ShouldBeTrue)
	})
}

func TestShutdownXTraceIdempotent(t *testing.T) {
	PatchConvey("TestShutdownXTraceIdempotent", t, func() {
		calls := 0
		xTraceShutdownFunc = func() error {
			calls++
			return nil
		}

		So(shutdownXTrace(), ShouldBeNil)
		So(shutdownXTrace(), ShouldBeNil)
		So(calls, ShouldEqual, 1)
	})
}

func TestSetShutdownTimeout(t *testing.T) {
	PatchConvey("TestSetShutdownTimeout-ValidTimeout", t, func() {
		original := defaultShutdownTimeout
		SetShutdownTimeout(10 * time.Second)
		So(defaultShutdownTimeout, ShouldEqual, 10*time.Second)
		defaultShutdownTimeout = original
	})

	PatchConvey("TestSetShutdownTimeout-ZeroTimeout", t, func() {
		original := defaultShutdownTimeout
		SetShutdownTimeout(0)
		So(defaultShutdownTimeout, ShouldEqual, original)
	})

	PatchConvey("TestSetShutdownTimeout-NegativeTimeout", t, func() {
		original := defaultShutdownTimeout
		SetShutdownTimeout(-1 * time.Second)
		So(defaultShutdownTimeout, ShouldEqual, original)
	})
}

func TestGetTracer(t *testing.T) {
	PatchConvey("TestGetTracer", t, func() {
		tracer := GetTracer("test-tracer")
		So(tracer, ShouldNotBeNil)
	})
}

func TestGetConfigXTrace(t *testing.T) {
	PatchConvey("TestGetConfig-UnmarshalFail", t, func() {
		Mock(xconfig.UnmarshalConfig).Return(errors.New("unmarshal failed")).Build()

		config, err := getConfig()
		So(err, ShouldNotBeNil)
		So(config, ShouldBeNil)
	})

	PatchConvey("TestGetConfig-Success", t, func() {
		Mock(xconfig.UnmarshalConfig).Return(nil).Build()

		config, err := getConfig()
		So(err, ShouldBeNil)
		So(config, ShouldNotBeNil)
		So(*config.Enable, ShouldBeTrue) // 默认值
	})
}

func TestInitXTrace(t *testing.T) {
	PatchConvey("TestInitXTrace-GetConfigFail", t, func() {
		Mock(getConfig).Return(nil, errors.New("config failed")).Build()

		err := initXTrace()
		So(err, ShouldNotBeNil)
		So(err.Error(), ShouldContainSubstring, "getConfig failed")
	})

	PatchConvey("TestInitXTrace-Disabled", t, func() {
		enableFalse := false
		Mock(getConfig).Return(&Config{Enable: &enableFalse}, nil).Build()

		err := initXTrace()
		So(err, ShouldBeNil)
	})
}

func TestInitXTraceByConfig(t *testing.T) {
	PatchConvey("TestInitXTraceByConfig-Success", t, func() {
		config := &Config{Console: false}
		err := initXTraceByConfig(config, "test-service", "v1.0.0")
		So(err, ShouldBeNil)
	})

	PatchConvey("TestInitXTraceByConfig-WithConsole", t, func() {
		config := &Config{Console: true}
		err := initXTraceByConfig(config, "test-service", "v1.0.0")
		So(err, ShouldBeNil)
	})
}

// TestConsolePrintTraceDemo 本地测试，打印上报内容到屏幕
func TestConsolePrintTraceDemo(t *testing.T) {
	t.Skip("本地测试，打印上报内容到屏幕")

	ctx := context.Background()

	// 初始化TracerProvider
	exporter, err := stdouttrace.New(stdouttrace.WithPrettyPrint())
	if err != nil {
		log.Fatalf("failed to initialize stdouttrace export pipeline: %v", err)
	}

	// 自定义资源属性
	r, err := resource.New(ctx,
		resource.WithFromEnv(),
		resource.WithProcess(),
		resource.WithTelemetrySDK(),
		resource.WithHost(),
		resource.WithAttributes(
			semconv.ServiceNameKey.String("MyServiceName"),
			semconv.ServiceVersionKey.String("MyServiceVersion"),
			attribute.String("custom.attribute", "customValue"),
		),
	)
	if err != nil {
		log.Fatalf("failed to create resource: %v", err)
	}

	tp := trace.NewTracerProvider(
		trace.WithBatcher(exporter),
		trace.WithSampler(trace.AlwaysSample()),
		trace.WithResource(r),
	)
	otel.SetTracerProvider(tp)

	// 创建Tracer
	tracer := otel.Tracer("my-package-name")

	// 创建并启动一个Span
	ctx, span := tracer.Start(ctx, "mySpan")

	// 添加自定义Span属性
	span.SetAttributes(attribute.String("customSpanAttribute", "customValue"))

	span.End()
	// 添加自定义事件
	//span.AddEvent("customEvent", trace.WithAttributes(attribute.String("eventAttribute", "eventValue")))

	// 业务逻辑...

	// 确保所有上报逻辑完成后再关闭TracerProvider
	if err := tp.Shutdown(ctx); err != nil {
		log.Fatalf("error shutting down tracer provider: %v", err)
	}
}
