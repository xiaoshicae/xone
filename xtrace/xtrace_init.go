package xtrace

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/xiaoshicae/xone/xconfig"
	"github.com/xiaoshicae/xone/xhook"
	"github.com/xiaoshicae/xone/xutil"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
	oteltrace "go.opentelemetry.io/otel/trace"
)

var defaultShutdownTimeout = 5 * time.Second

var (
	xTraceShutdownFunc func() error
	shutdownExecuted   atomic.Bool // 确保 shutdown 只执行一次
	shutdownMu         sync.Mutex
)

func init() {
	xhook.BeforeStart(initXTrace)
	xhook.BeforeStop(shutdownXTrace)
}

// SetShutdownTimeout 设置 shutdown 超时时间
func SetShutdownTimeout(timeout time.Duration) {
	if timeout > 0 {
		defaultShutdownTimeout = timeout
	}
}

// GetTracer 获取 Tracer，方便用户创建自定义 Span
func GetTracer(name string, opts ...oteltrace.TracerOption) oteltrace.Tracer {
	return otel.Tracer(name, opts...)
}

func initXTrace() error {
	c, err := getConfig()
	if err != nil {
		return fmt.Errorf("XOne initXTrace getConfig failed, err=[%v]", err)
	}

	if c.Enable != nil && !*c.Enable {
		otel.SetTracerProvider(oteltrace.NewNoopTracerProvider())
		xutil.InfoIfEnableDebug("XOne initXTrace ignored, because of config XTrace.Enable=false")
		return nil
	}

	serviceName := xconfig.GetServerName()
	serviceVersion := xconfig.GetServerVersion()

	xutil.InfoIfEnableDebug("XOne initXTrace got param: ServiceName:%s, ServiceVersion:%s", serviceName, serviceVersion)

	return initXTraceByConfig(c, serviceName, serviceVersion)
}

func initXTraceByConfig(c *Config, serviceName, serviceVersion string) error {
	// 只使用 semconv 标准属性，避免重复
	r, err := resource.New(
		context.Background(),
		resource.WithFromEnv(),
		resource.WithProcess(),
		resource.WithTelemetrySDK(),
		resource.WithHost(),
		resource.WithAttributes(
			semconv.ServiceNameKey.String(serviceName),
			semconv.ServiceVersionKey.String(serviceVersion),
		),
	)
	if err != nil {
		return fmt.Errorf("XOne initXTraceByConfig invoke resource.New failed, err=[%v]", err)
	}

	tpOpts := []trace.TracerProviderOption{
		trace.WithSampler(trace.AlwaysSample()),
		trace.WithResource(r),
	}

	if c.Console {
		exporter, err := stdouttrace.New(stdouttrace.WithPrettyPrint())
		if err != nil {
			return fmt.Errorf("XOne initXTraceByConfig init exporter failed, err=[%v]", err)
		}
		// 使用 SimpleSpanProcessor 确保每个 Span 都被导出，避免丢失
		tpOpts = append(tpOpts, trace.WithSpanProcessor(trace.NewSimpleSpanProcessor(exporter)))
	}

	tp := trace.NewTracerProvider(tpOpts...)
	otel.SetTracerProvider(tp)

	// 设置 W3C Trace Context propagator，支持从请求 header 提取/注入 trace context
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	// 使用互斥锁保护 shutdown 函数的设置
	shutdownMu.Lock()
	xTraceShutdownFunc = func() error {
		ctx, cancel := context.WithTimeout(context.Background(), defaultShutdownTimeout)
		defer cancel()
		return tp.Shutdown(ctx)
	}
	// 重置 shutdown 标志，允许新的 shutdown
	shutdownExecuted.Store(false)
	shutdownMu.Unlock()

	return nil
}

func getConfig() (*Config, error) {
	c := &Config{}
	if err := xconfig.UnmarshalConfig(XTraceConfigKey, c); err != nil {
		return nil, err
	}
	c = configMergeDefault(c)
	return c, nil
}

func shutdownXTrace() error {
	// 使用 CAS 确保只执行一次
	if !shutdownExecuted.CompareAndSwap(false, true) {
		return nil
	}

	shutdownMu.Lock()
	fn := xTraceShutdownFunc
	xTraceShutdownFunc = nil
	shutdownMu.Unlock()

	if fn != nil {
		return fn()
	}
	return nil
}
