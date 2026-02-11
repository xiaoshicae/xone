package xutil

import (
	"context"

	"go.opentelemetry.io/otel/trace"
)

// GetTraceIDFromCtx 从ctx获取TraceID
func GetTraceIDFromCtx(ctx context.Context) string {
	return spanFieldFromCtx(ctx, func(sc trace.SpanContext) string {
		return sc.TraceID().String()
	})
}

// GetSpanIDFromCtx 从ctx获取SpanID
func GetSpanIDFromCtx(ctx context.Context) string {
	return spanFieldFromCtx(ctx, func(sc trace.SpanContext) string {
		return sc.SpanID().String()
	})
}

// spanFieldFromCtx 从 ctx 的 span 中提取指定字段
func spanFieldFromCtx(ctx context.Context, extract func(trace.SpanContext) string) string {
	span := trace.SpanFromContext(ctx)
	if span != nil && span.SpanContext().IsValid() {
		return extract(span.SpanContext())
	}
	return ""
}
