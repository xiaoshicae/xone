package xredis

import (
	"context"
	"slices"
	"strings"
	"sync"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

// recordingTP 只记下每个 Span 开始时带的属性。
//
// 不用 otel 的 sdk + tracetest：测试依赖会被 go mod tidy 记成直接依赖，
// 跟着进每个使用者的模块图（理由同 fakeRedis）。redisotel 的属性都是在
// Start 时一次给齐的，记这一处就够了。
type recordingTP struct {
	noop.TracerProvider
	mu    sync.Mutex
	attrs []attribute.KeyValue
	names []string
}

func (p *recordingTP) Tracer(string, ...trace.TracerOption) trace.Tracer {
	return recordingTracer{p: p}
}

func (p *recordingTP) spans() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.names...)
}

func (p *recordingTP) all() []attribute.KeyValue {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]attribute.KeyValue(nil), p.attrs...)
}

type recordingTracer struct {
	noop.Tracer
	p *recordingTP
}

func (t recordingTracer) Start(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	cfg := trace.NewSpanStartConfig(opts...)
	attrs := cfg.Attributes()
	t.p.mu.Lock()
	t.p.attrs = append(t.p.attrs, attrs...)
	t.p.names = append(t.p.names, name)
	t.p.mu.Unlock()
	return t.Tracer.Start(ctx, name, opts...)
}

func TestTrace_CommandArgsNotInSpan(t *testing.T) {
	// redisotel v9.22.0 默认 dbStmtEnabled=true，把整条命令连同参数写进
	// db.statement：SET 的值、AUTH 的密码，统统进了链路后端。
	// 与 xgorm 的「不记 Statement.Vars」是同一条原则
	tp := &recordingTP{}
	old := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	t.Cleanup(func() { otel.SetTracerProvider(old) })

	f := newFakeRedis(t)
	client, closer, err := New(context.Background(), liveCfg(f))
	if err != nil {
		t.Fatal(err)
	}
	defer closer.Close()

	const secret = "s3cr3t-value"
	if err := client.Set(context.Background(), "session:1", secret, 0).Err(); err != nil {
		t.Fatal(err)
	}

	var instrumented bool
	for _, a := range tp.all() {
		if strings.Contains(a.Value.Emit(), secret) {
			t.Errorf("命令参数进了 Span：%s=%s", a.Key, a.Value.Emit())
		}
		if a.Key == "db.system" {
			instrumented = true
		}
	}
	if !instrumented {
		t.Fatal("前提：链路钩子该挂上，否则这条测试什么都没验")
	}
}

// useTP 换上记录用的 TracerProvider，测试结束还原
func useTP(t *testing.T) *recordingTP {
	tp := &recordingTP{}
	old := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	t.Cleanup(func() { otel.SetTracerProvider(old) })
	return tp
}

func TestTrace_StartupConnectCheckOpensNoSpan(t *testing.T) {
	// 钩子挂在建连验证之前的话，每一次 Ping 尝试都是一个没有父 Span 的 ping，
	// 连不上时一轮重试就是一串报错的根 Span——那是一次启动，不是业务请求
	tp := useTP(t)
	f := newFakeRedis(t)
	f.setFailPing(true)
	if _, _, err := New(context.Background(), liveCfg(f)); err == nil {
		t.Fatal("前提：Ping 失败时 New 该失败")
	}
	if got := tp.spans(); len(got) != 0 {
		t.Errorf("连不上时的建连验证不该留下 Span，got=%v", got)
	}

	f.setFailPing(false)
	client, closer, err := New(context.Background(), liveCfg(f))
	if err != nil {
		t.Fatal(err)
	}
	defer closer.Close()
	if got := tp.spans(); slices.Contains(got, "ping") {
		t.Errorf("连得上时的那次建连验证也不该留下 ping Span，got=%v", got)
	}
	if err := client.Ping(context.Background()).Err(); err != nil {
		t.Fatal(err)
	}
	if got := tp.spans(); !slices.Contains(got, "ping") {
		t.Errorf("建好之后业务发的命令该有 Span（钩子得挂上），got=%v", got)
	}
}
