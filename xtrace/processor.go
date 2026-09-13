package xtrace

import (
	"context"

	"github.com/xiaoshicae/xone/v2/xutil"

	"go.opentelemetry.io/otel/sdk/trace"
)

// AddSpanProcessor 注册自定义 SpanProcessor，用于把 Span 上报到远端
//
// 框架不内置任何上报 exporter：OTLP / Jaeger 等 exporter 会带进上百个构建依赖
// （实测 OTLP 为 +143 个包），不应由所有使用者承担。需要上报的服务自行引入
// exporter 并在此注册，不需要的服务零代价。
//
//	exp, err := otlptracegrpc.New(ctx, otlptracegrpc.WithEndpoint(endpoint), otlptracegrpc.WithInsecure())
//	if err != nil { return err }
//	xtrace.AddSpanProcessor(trace.NewBatchSpanProcessor(exp))
//
// 可在初始化前后任意时刻调用：初始化前注册的会在初始化时装上，
// 初始化后注册的立即生效，重复初始化时自动装回新的 TracerProvider。
//
// 注册的处理器由 xtrace 在 BeforeStop 阶段统一关闭，关闭等待上限由
// XTrace.ShutdownTimeout 控制。
func AddSpanProcessor(sp trace.SpanProcessor) {
	if sp == nil {
		panic("XOne xtrace span processor can not be nil")
	}

	// 注册与初始化共用一把锁：否则并发的重新初始化可能把处理器装到
	// 正在被关闭的旧 TracerProvider 上
	traceMu.Lock()
	defer traceMu.Unlock()

	spanProcessors = append(spanProcessors, sp)
	if tracerProvider == nil {
		xutil.InfoIfEnableDebug("XOne xtrace span processor registered before init, will be attached on init")
		return
	}
	tracerProvider.RegisterSpanProcessor(keepAlive(sp))
}

// keepAlive 包装用户注册的处理器，屏蔽 TracerProvider 对它的 Shutdown
//
// TracerProvider.Shutdown 会关闭它持有的全部处理器，而用户注册的处理器要跨越
// 重复初始化继续存活，不能被旧 TracerProvider 带走。它们的关闭由 shutdownXTrace
// 统一负责，这样关闭超时也能由框架控制。
func keepAlive(sp trace.SpanProcessor) trace.SpanProcessor {
	return nonClosingProcessor{SpanProcessor: sp}
}

type nonClosingProcessor struct {
	trace.SpanProcessor
}

// Shutdown 不向下传递，由 shutdownXTrace 直接关闭原始处理器
func (p nonClosingProcessor) Shutdown(context.Context) error { return nil }
