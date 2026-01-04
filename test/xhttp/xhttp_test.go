package xhttp

import (
	"context"
	"fmt"
	"testing"

	"github.com/xiaoshicae/xone"
	"github.com/xiaoshicae/xone/xhttp"
	"github.com/xiaoshicae/xone/xutil"

	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

func TestXHttp(t *testing.T) {
	//t.Skip("真实环境测试，如果client能连通，可以注释掉该Skip进行测试")

	if err := xone.R(); err != nil {
		panic(err)
	}

	ctx := trace.ContextWithSpan(context.Background(), &MySpan{})
	tId := xutil.GetTraceIDFromCtx(ctx)
	t.Logf("TraceID:%v", tId)

	r := xhttp.RWithCtx(ctx)
	resp, err := r.
		EnableTrace().
		Get("https://httpbin.org/get")

	fmt.Println("Request Info:")
	fmt.Println("RequestHeader:", r.Header)

	fmt.Println("Response Info:")
	fmt.Println("  ErrorIfEnableDebug      :", err)
	fmt.Println("  Status Code:", resp.StatusCode())
	fmt.Println("  Status     :", resp.Status())
	fmt.Println("  Proto      :", resp.Proto())
	fmt.Println("  Time       :", resp.Time())
	fmt.Println("  Received At:", resp.ReceivedAt())
	fmt.Println("  Body       :\n", resp)
	fmt.Println("  ResponseHeader       :\n", xutil.ToJsonString(resp.Request.Header))
	fmt.Println()
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
