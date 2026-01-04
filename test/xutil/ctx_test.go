package xutil

import (
	"context"
	"testing"

	"github.com/xiaoshicae/xone/xutil"

	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"

	. "github.com/bytedance/mockey"
	. "github.com/smartystreets/goconvey/convey"
)

func TestCtx(t *testing.T) {
	PatchConvey("TestCtx", t, func() {
		tId := xutil.GetTraceIDFromCtx(context.Background())
		So(tId, ShouldBeEmpty)

		sId := xutil.GetSpanIDFromCtx(context.Background())
		So(sId, ShouldBeEmpty)

		ctx := trace.ContextWithSpan(context.Background(), &MySpan{})
		tId = xutil.GetTraceIDFromCtx(ctx)
		So(tId, ShouldEqual, "4bf92f3577b34da6a3ce929d0e0e4736")

		sId = xutil.GetSpanIDFromCtx(ctx)
		So(sId, ShouldEqual, "fc4f418ae50ccd5e")
	})
}

type MySpan struct {
	noop.Span
}

func (*MySpan) SpanContext() trace.SpanContext {
	tId, _ := trace.TraceIDFromHex("4bf92f3577b34da6a3ce929d0e0e4736")

	sId, _ := trace.SpanIDFromHex("fc4f418ae50ccd5e")

	sc := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    tId,
		SpanID:     sId,
		TraceFlags: 0,
		TraceState: trace.TraceState{},
		Remote:     false,
	})

	return sc
}
