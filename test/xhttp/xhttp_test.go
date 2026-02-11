package xhttp

import (
	"context"
	"fmt"
	"testing"

	"github.com/xiaoshicae/xone/v2/xhttp"
	"github.com/xiaoshicae/xone/v2/xserver"
	"github.com/xiaoshicae/xone/v2/xutil"

	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

func TestXHttp(t *testing.T) {
	t.Skip("集成测试，需手动运行")

	if err := xserver.R(); err != nil {
		panic(err)
	}

	// 测试 RawClient 获取原生 http.Client
	rawClient := xhttp.RawClient()
	if rawClient == nil {
		t.Fatal("RawClient should not be nil after xserver.R()")
	}
	t.Logf("RawClient Timeout: %v", rawClient.Timeout)

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

// TestRawClient 测试 RawClient 用于流式请求场景
func TestRawClient(t *testing.T) {
	t.Skip("集成测试，需手动运行")

	if err := xserver.R(); err != nil {
		panic(err)
	}

	rawClient := xhttp.RawClient()
	if rawClient == nil {
		t.Fatal("RawClient should not be nil")
	}

	// 验证 RawClient 配置了超时
	if rawClient.Timeout == 0 {
		t.Error("RawClient should have timeout configured")
	}
	t.Logf("RawClient Timeout: %v", rawClient.Timeout)

	// 验证 Transport 配置
	if rawClient.Transport != nil {
		t.Log("RawClient Transport is configured")
	}

	// 测试使用 RawClient 发起请求（网络不稳定时跳过）
	resp, err := rawClient.Get("https://httpbin.org/get")
	if err != nil {
		t.Logf("RawClient request failed (network issue, skipped): %v", err)
		return
	}
	defer resp.Body.Close()

	t.Logf("RawClient Response Status: %s", resp.Status)
	t.Logf("RawClient Response Proto: %s", resp.Proto)
}
