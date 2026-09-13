package xutil

import (
	"context"
	"sync/atomic"
)

// TraceContextExtractor 从 ctx 中提取链路标识，未处于链路中时返回两个空串
type TraceContextExtractor func(ctx context.Context) (traceID, spanID string)

// traceContextExtractor 当前生效的提取器，由 xtrace 在 init 阶段注入
//
// 基础层不直接依赖 OpenTelemetry：链路是 xtrace 的职责，若把 otel/trace
// 编进 xutil，所有 import 了 xone 的服务都要背上整棵 otel trace/attribute 树，
// 哪怕它们根本不用链路。改由 xtrace 注入后，没有 xtrace 就自然没有链路标识。
var traceContextExtractor atomic.Pointer[TraceContextExtractor]

// SetTraceContextExtractor 注入链路标识提取器，传 nil 表示取消注入
//
// 由 xtrace 在 init 阶段自动调用，业务通常无需关心；
// 直接使用 OpenTelemetry 而不经由 xtrace 时，可自行注入以便日志关联链路。
func SetTraceContextExtractor(fn TraceContextExtractor) {
	if fn == nil {
		traceContextExtractor.Store(nil)
		return
	}
	traceContextExtractor.Store(&fn)
}

// GetTraceAndSpanIDFromCtx 一次性获取 TraceID 与 SpanID
//
// 日志等热点路径两个值都要用，合并取用可省去一次提取。
func GetTraceAndSpanIDFromCtx(ctx context.Context) (traceID, spanID string) {
	fn := traceContextExtractor.Load()
	if fn == nil || ctx == nil {
		return "", ""
	}
	return (*fn)(ctx)
}

// GetTraceIDFromCtx 从ctx获取TraceID
func GetTraceIDFromCtx(ctx context.Context) string {
	traceID, _ := GetTraceAndSpanIDFromCtx(ctx)
	return traceID
}

// GetSpanIDFromCtx 从ctx获取SpanID
func GetSpanIDFromCtx(ctx context.Context) string {
	_, spanID := GetTraceAndSpanIDFromCtx(ctx)
	return spanID
}
