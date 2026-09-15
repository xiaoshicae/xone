package xtrace

import (
	"context"

	"github.com/xiaoshicae/xone/v3/xutil"

	oteltrace "go.opentelemetry.io/otel/trace"
)

func init() {
	// 基础层不依赖 OpenTelemetry，链路标识的提取由本模块注入
	xutil.SetTraceContextExtractor(traceContextFromCtx)
}

// traceContextFromCtx 从 ctx 的 Span 中提取链路标识
func traceContextFromCtx(ctx context.Context) (traceID, spanID string) {
	sc := oteltrace.SpanFromContext(ctx).SpanContext()
	if !sc.IsValid() {
		return "", ""
	}
	return sc.TraceID().String(), sc.SpanID().String()
}
