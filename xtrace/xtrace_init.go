package xtrace

import (
	"context"
	"sync"

	"github.com/xiaoshicae/xone/v2/xconfig"
	"github.com/xiaoshicae/xone/v2/xerror"
	"github.com/xiaoshicae/xone/v2/xhook"
	"github.com/xiaoshicae/xone/v2/xutil"

	"go.opentelemetry.io/contrib/propagators/b3"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
	oteltrace "go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

// tracerProvider 当前生效的 TracerProvider，重复初始化时替换并关闭旧实例
//
// 只用一个互斥锁：锁内取出并置空即可保证关闭最多执行一次，
// 额外的"已关闭"标志与这把锁无法原子地一起更新，反而会让并发的重新初始化
// 被紧随其后的 shutdown 关掉刚建好的实例。
var (
	tracerProvider   *trace.TracerProvider
	tracerProviderMu sync.Mutex
)

func init() {
	xhook.BeforeStart(initXTrace)
	xhook.BeforeStop(shutdownXTrace)
}

// GetTracer 获取 Tracer，方便用户创建自定义 Span
func GetTracer(name string, opts ...oteltrace.TracerOption) oteltrace.Tracer {
	return otel.Tracer(name, opts...)
}

func initXTrace() error {
	c, err := getConfig()
	if err != nil {
		return xerror.Newf("xtrace", "init", "getConfig failed, err=[%v]", err)
	}

	traceEnabled.Store(*c.Enable)
	forwardHeaderEnabled.Store(c.forwardEnabled())

	if !*c.Enable {
		return initXTraceDisabled(c)
	}

	serviceName := xconfig.GetServerName()
	serviceVersion := xconfig.GetServerVersion()

	xutil.InfoIfEnableDebug("XOne initXTrace got param: ServiceName:%s, ServiceVersion:%s", serviceName, serviceVersion)

	return initXTraceByConfig(c, serviceName, serviceVersion)
}

// initXTraceDisabled 关闭链路，但保留 Header 透传能力
//
// Header 透传（如 X-Request-Id）与是否采样 Span 是两件事：
// 关掉链路不应让已配置的透传规则静默失效。
func initXTraceDisabled(c *Config) error {
	swapTracerProvider(nil)
	otel.SetTracerProvider(noop.NewTracerProvider())

	if c.forwardEnabled() {
		otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(newHeaderPropagatorFromConfig(c)))
		xutil.InfoIfEnableDebug("XOne initXTrace disabled, HeaderPropagator kept for header forwarding")
	} else {
		otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator())
		xutil.InfoIfEnableDebug("XOne initXTrace ignored, because of config XTrace.Enable=false")
	}
	return nil
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
		return xerror.Newf("xtrace", "init", "resource.New failed, err=[%v]", err)
	}

	tpOpts := []trace.TracerProviderOption{
		trace.WithSampler(samplerOf(c.SampleRatio)),
		trace.WithResource(r),
	}

	if c.EnableConsole {
		exporter, err := stdouttrace.New(stdouttrace.WithPrettyPrint())
		if err != nil {
			return xerror.Newf("xtrace", "init", "init exporter failed, err=[%v]", err)
		}
		// 使用 SimpleSpanProcessor 确保每个 Span 都被导出，避免丢失
		tpOpts = append(tpOpts, trace.WithSpanProcessor(trace.NewSimpleSpanProcessor(exporter)))
	}

	tp := trace.NewTracerProvider(tpOpts...)
	otel.SetTracerProvider(tp)

	// 设置 propagator，支持 W3C Trace Context、Baggage 和 B3 格式
	propagators := []propagation.TextMapPropagator{
		propagation.TraceContext{},
		propagation.Baggage{},
		b3.New(),
	}
	if c.forwardEnabled() {
		propagators = append(propagators, newHeaderPropagatorFromConfig(c))
		xutil.InfoIfEnableDebug("XOne initXTrace registered HeaderPropagator, globalHeaders=%v, rules=%v", c.ForwardHeaders, c.ForwardHeaderRules)
	}
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagators...))

	// 先发布新实例再关闭旧实例，避免切换窗口内的 Span 丢失
	swapTracerProvider(tp)
	return nil
}

// samplerOf 按采样率构造 Sampler，比例 >= 1 时全采样
func samplerOf(ratio float64) trace.Sampler {
	if ratio >= 1 {
		return trace.AlwaysSample()
	}
	return trace.ParentBased(trace.TraceIDRatioBased(ratio))
}

func newHeaderPropagatorFromConfig(c *Config) *HeaderPropagator {
	return NewHeaderPropagator(c.ForwardHeaders, c.ForwardHeaderRules)
}

// swapTracerProvider 替换当前 TracerProvider 并关闭旧实例，避免重复初始化遗留未关闭的实例
func swapTracerProvider(tp *trace.TracerProvider) {
	tracerProviderMu.Lock()
	old := tracerProvider
	tracerProvider = tp
	tracerProviderMu.Unlock()

	if old != nil {
		if err := old.Shutdown(context.Background()); err != nil {
			xutil.WarnIfEnableDebug("XOne swapTracerProvider shutdown previous provider failed, err=[%v]", err)
		}
	}
}

func getConfig() (*Config, error) {
	c := &Config{}
	if err := xconfig.UnmarshalConfig(XTraceConfigKey, c); err != nil {
		return nil, err
	}
	c = configMergeDefault(c)
	return c, nil
}

// shutdownXTrace 关闭 TracerProvider，超时由 xhook 的 Hook 超时统一控制
func shutdownXTrace() error {
	tracerProviderMu.Lock()
	tp := tracerProvider
	tracerProvider = nil
	tracerProviderMu.Unlock()

	if tp == nil {
		return nil
	}
	return tp.Shutdown(context.Background())
}
