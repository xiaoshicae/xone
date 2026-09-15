package xtrace

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/xiaoshicae/xone/v3/xconfig"
	"github.com/xiaoshicae/xone/v3/xerror"
	"github.com/xiaoshicae/xone/v3/xhook"
	"github.com/xiaoshicae/xone/v3/xutil"

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

// 链路的可变状态，统一由 traceMu 保护
//
// 只用一把锁：锁内取出并置空即可保证关闭最多执行一次，额外的"已关闭"标志
// 与这把锁无法原子地一起更新，反而会让并发的重新初始化被紧随其后的 shutdown
// 关掉刚建好的实例。处理器注册也共用这把锁，避免装到正在关闭的旧实例上。
var (
	// tracerProvider 当前生效的 TracerProvider，重复初始化时替换并关闭旧实例
	tracerProvider *trace.TracerProvider

	// spanProcessors 由使用者通过 AddSpanProcessor 注册的处理器
	spanProcessors []trace.SpanProcessor

	// shutdownTimeout 关闭时等待 Span 导出完成的上限
	shutdownTimeout = defaultShutdownTimeout

	traceMu sync.Mutex
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

	traceMu.Lock()
	shutdownTimeout = xutil.ToDuration(c.ShutdownTimeout)
	pending := len(spanProcessors)
	traceMu.Unlock()

	if !*c.Enable && pending > 0 {
		xutil.WarnIfEnableDebug("XOne initXTrace disabled but %d span processor(s) registered, they will receive no span", pending)
	}

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
	publishTracerProvider(nil)
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
	publishTracerProvider(tp)
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

// publishTracerProvider 把已注册的处理器装到新 TracerProvider 上并发布它，
// 随后关闭旧实例，避免重复初始化遗留未关闭的实例
//
// 装处理器与发布必须在同一把锁内完成：否则并发的 AddSpanProcessor
// 可能把处理器装到即将被关闭的旧实例上。
func publishTracerProvider(tp *trace.TracerProvider) {
	traceMu.Lock()
	if tp != nil {
		for _, sp := range spanProcessors {
			tp.RegisterSpanProcessor(keepAlive(sp))
		}
	}
	old := tracerProvider
	tracerProvider = tp
	traceMu.Unlock()

	if old != nil {
		// 用户注册的处理器被 keepAlive 包着，不会被这次关闭带走
		if err := old.Shutdown(context.Background()); err != nil {
			xutil.WarnIfEnableDebug("XOne publishTracerProvider shutdown previous provider failed, err=[%v]", err)
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

// shutdownXTrace 关闭 TracerProvider 与使用者注册的处理器
//
// 必须限定等待上限：导出端不可达时 TracerProvider.Shutdown 会一直阻塞到
// context 超时，而 xhook 的 Hook 超时只是放弃等待、不会取消函数，
// 没有 deadline 就意味着那个 goroutine 永久泄漏。
func shutdownXTrace() error {
	traceMu.Lock()
	tp := tracerProvider
	tracerProvider = nil
	procs := spanProcessors
	spanProcessors = nil
	timeout := shutdownTimeout
	traceMu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	errMsgList := make([]string, 0, len(procs)+1)
	if tp != nil {
		if err := tp.Shutdown(ctx); err != nil {
			errMsgList = append(errMsgList, fmt.Sprintf("tracer provider: %v", err))
		}
	}
	// 用户注册的处理器由框架关闭：它们被 keepAlive 包装后不受 provider 关闭影响
	for _, sp := range procs {
		if err := sp.Shutdown(ctx); err != nil {
			errMsgList = append(errMsgList, fmt.Sprintf("span processor %T: %v", sp, err))
		}
	}

	if len(errMsgList) > 0 {
		return xerror.Newf("xtrace", "shutdown", "%s", strings.Join(errMsgList, "; "))
	}
	return nil
}
