package xutil

import (
	"context"

	"go.opentelemetry.io/otel/trace"
)

// GetTraceIDFromCtx 从ctx获取TraceID
func GetTraceIDFromCtx(ctx context.Context) string {
	// 先从trace获取
	span := trace.SpanFromContext(ctx)
	if span != nil && span.SpanContext().IsValid() {
		return span.SpanContext().TraceID().String()
	}

	return ""
}

// GetSpanIDFromCtx 从ctx获取SpanID
func GetSpanIDFromCtx(ctx context.Context) string {
	span := trace.SpanFromContext(ctx)

	if span != nil && span.SpanContext().IsValid() {
		return span.SpanContext().SpanID().String()
	}

	return ""
}
