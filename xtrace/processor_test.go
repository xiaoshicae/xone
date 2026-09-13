package xtrace

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/xiaoshicae/xone/v2/xutil"

	. "github.com/bytedance/mockey"
	. "github.com/smartystreets/goconvey/convey"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// recorder 记录型处理器，模拟使用者自带的 exporter
type recorder struct {
	mu           sync.Mutex
	spans        []string
	shutdownCnt  int
	flushCnt     int
	shutdownErr  error
	shutdownHold time.Duration
}

func (r *recorder) OnStart(context.Context, sdktrace.ReadWriteSpan) {}

func (r *recorder) OnEnd(s sdktrace.ReadOnlySpan) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.spans = append(r.spans, s.Name())
}

func (r *recorder) Shutdown(ctx context.Context) error {
	if r.shutdownHold > 0 {
		select {
		case <-time.After(r.shutdownHold):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.shutdownCnt++
	return r.shutdownErr
}

func (r *recorder) ForceFlush(context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.flushCnt++
	return nil
}

func (r *recorder) snapshot() ([]string, int, int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.spans...), r.shutdownCnt, r.flushCnt
}

// resetTraceState 清空模块状态，避免用例之间互相影响
func resetTraceState() {
	traceMu.Lock()
	tracerProvider = nil
	spanProcessors = nil
	shutdownTimeout = defaultShutdownTimeout
	traceMu.Unlock()
}

func TestAddSpanProcessor(t *testing.T) {
	PatchConvey("TestAddSpanProcessor", t, func() {
		resetTraceState()
		defer func() {
			_ = shutdownXTrace()
			resetTraceState()
		}()

		PatchConvey("nil 处理器直接 panic", func() {
			So(func() { AddSpanProcessor(nil) }, ShouldPanicWith, "XOne xtrace span processor can not be nil")
		})

		PatchConvey("初始化前注册，初始化时装上并开始收 Span", func() {
			rec := &recorder{}
			AddSpanProcessor(rec)

			So(initXTraceByConfig(configMergeDefault(&Config{}), "svc", "v1"), ShouldBeNil)
			_, span := otel.Tracer("t").Start(context.Background(), "before-init")
			span.End()

			spans, _, _ := rec.snapshot()
			So(spans, ShouldResemble, []string{"before-init"})
		})

		PatchConvey("初始化后注册立即生效", func() {
			So(initXTraceByConfig(configMergeDefault(&Config{}), "svc", "v1"), ShouldBeNil)

			rec := &recorder{}
			AddSpanProcessor(rec)

			_, span := otel.Tracer("t").Start(context.Background(), "after-init")
			span.End()

			spans, _, _ := rec.snapshot()
			So(spans, ShouldResemble, []string{"after-init"})
		})

		PatchConvey("重复初始化后处理器仍然有效，且不会被旧实例关闭", func() {
			rec := &recorder{}
			AddSpanProcessor(rec)
			So(initXTraceByConfig(configMergeDefault(&Config{}), "svc", "v1"), ShouldBeNil)
			So(initXTraceByConfig(configMergeDefault(&Config{}), "svc", "v1"), ShouldBeNil)

			_, span := otel.Tracer("t").Start(context.Background(), "after-reinit")
			span.End()

			spans, shutdowns, _ := rec.snapshot()
			So(spans, ShouldResemble, []string{"after-reinit"})
			// 关键：旧 TracerProvider 的关闭不能把使用者的处理器一起关掉
			So(shutdowns, ShouldEqual, 0)
		})

		PatchConvey("多个处理器都能收到 Span", func() {
			a, b := &recorder{}, &recorder{}
			AddSpanProcessor(a)
			AddSpanProcessor(b)
			So(initXTraceByConfig(configMergeDefault(&Config{}), "svc", "v1"), ShouldBeNil)

			_, span := otel.Tracer("t").Start(context.Background(), "multi")
			span.End()

			sa, _, _ := a.snapshot()
			sb, _, _ := b.snapshot()
			So(sa, ShouldResemble, []string{"multi"})
			So(sb, ShouldResemble, []string{"multi"})
		})

		PatchConvey("链路关闭时注册会告警，处理器收不到 Span", func() {
			rec := &recorder{}
			AddSpanProcessor(rec)

			c := configMergeDefault(&Config{Enable: xutil.ToPtr(false)})
			So(initXTrace2(c), ShouldBeNil)

			_, span := otel.Tracer("t").Start(context.Background(), "disabled")
			span.End()

			spans, _, _ := rec.snapshot()
			So(spans, ShouldBeEmpty)
		})
	})
}

// initXTrace2 复刻 initXTrace 中依赖配置的分支，避免测试去读真实配置文件
func initXTrace2(c *Config) error {
	traceEnabled.Store(*c.Enable)
	forwardHeaderEnabled.Store(c.forwardEnabled())

	traceMu.Lock()
	shutdownTimeout = xutil.ToDuration(c.ShutdownTimeout)
	pending := len(spanProcessors)
	traceMu.Unlock()

	if !*c.Enable && pending > 0 {
		xutil.WarnIfEnableDebug("XOne initXTrace disabled but %d span processor(s) registered", pending)
	}
	if !*c.Enable {
		return initXTraceDisabled(c)
	}
	return initXTraceByConfig(c, "svc", "v1")
}

func TestShutdownXTraceWithProcessors(t *testing.T) {
	PatchConvey("TestShutdownXTraceWithProcessors", t, func() {
		resetTraceState()
		defer resetTraceState()

		PatchConvey("关闭时逐个关闭使用者注册的处理器", func() {
			a, b := &recorder{}, &recorder{}
			AddSpanProcessor(a)
			AddSpanProcessor(b)
			So(initXTraceByConfig(configMergeDefault(&Config{}), "svc", "v1"), ShouldBeNil)

			So(shutdownXTrace(), ShouldBeNil)
			_, sa, _ := a.snapshot()
			_, sb, _ := b.snapshot()
			So(sa, ShouldEqual, 1)
			So(sb, ShouldEqual, 1)

			// 再次关闭无事可做
			So(shutdownXTrace(), ShouldBeNil)
			_, sa2, _ := a.snapshot()
			So(sa2, ShouldEqual, 1)
		})

		PatchConvey("未初始化也能关闭已注册的处理器", func() {
			rec := &recorder{}
			AddSpanProcessor(rec)
			So(shutdownXTrace(), ShouldBeNil)
			_, cnt, _ := rec.snapshot()
			So(cnt, ShouldEqual, 1)
		})

		PatchConvey("处理器关闭失败时汇总错误", func() {
			rec := &recorder{shutdownErr: errors.New("exporter down")}
			AddSpanProcessor(rec)
			So(initXTraceByConfig(configMergeDefault(&Config{}), "svc", "v1"), ShouldBeNil)

			err := shutdownXTrace()
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "exporter down")
		})

		PatchConvey("TracerProvider 关闭失败时汇总错误", func() {
			So(initXTraceByConfig(configMergeDefault(&Config{}), "svc", "v1"), ShouldBeNil)
			Mock((*sdktrace.TracerProvider).Shutdown).Return(errors.New("tp down")).Build()
			err := shutdownXTrace()
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "tp down")
		})

		PatchConvey("导出端卡住时按 ShutdownTimeout 返回，而不是无限等待", func() {
			rec := &recorder{shutdownHold: 10 * time.Second}
			AddSpanProcessor(rec)
			So(initXTraceByConfig(configMergeDefault(&Config{ShutdownTimeout: "100ms"}), "svc", "v1"), ShouldBeNil)

			traceMu.Lock()
			shutdownTimeout = 100 * time.Millisecond
			traceMu.Unlock()

			start := time.Now()
			err := shutdownXTrace()
			elapsed := time.Since(start)

			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "context deadline exceeded")
			So(elapsed, ShouldBeLessThan, 3*time.Second)
		})
	})
}

func TestKeepAlive(t *testing.T) {
	PatchConvey("TestKeepAlive", t, func() {
		rec := &recorder{}
		wrapped := keepAlive(rec)

		PatchConvey("Shutdown 不向下传递", func() {
			So(wrapped.Shutdown(context.Background()), ShouldBeNil)
			_, cnt, _ := rec.snapshot()
			So(cnt, ShouldEqual, 0)
		})

		PatchConvey("其余方法原样透传", func() {
			So(wrapped.ForceFlush(context.Background()), ShouldBeNil)
			_, _, flushes := rec.snapshot()
			So(flushes, ShouldEqual, 1)
		})
	})
}

// TestInitXTraceWarnsOnDisabledWithProcessors 链路关闭但注册了处理器时应告警
func TestInitXTraceWarnsOnDisabledWithProcessors(t *testing.T) {
	PatchConvey("TestInitXTraceWarnsOnDisabledWithProcessors", t, func() {
		resetTraceState()
		defer func() {
			resetTraceState()
			traceEnabled.Store(true)
			forwardHeaderEnabled.Store(false)
		}()

		AddSpanProcessor(&recorder{})
		Mock(getConfig).Return(configMergeDefault(&Config{Enable: xutil.ToPtr(false)}), nil).Build()

		var warned string
		Mock(xutil.WarnIfEnableDebug).To(func(msg string, args ...any) { warned = msg }).Build()

		So(initXTrace(), ShouldBeNil)
		So(warned, ShouldContainSubstring, "span processor(s) registered")
	})
}

// TestConfigMergeDefaultShutdownTimeout 非法 ShutdownTimeout 回落默认值并告警
func TestConfigMergeDefaultShutdownTimeout(t *testing.T) {
	PatchConvey("TestConfigMergeDefaultShutdownTimeout", t, func() {
		PatchConvey("未配置时用默认值，不告警", func() {
			var warned string
			Mock(xutil.WarnIfEnableDebug).To(func(msg string, args ...any) { warned = msg }).Build()
			c := configMergeDefault(&Config{})
			So(c.ShutdownTimeout, ShouldEqual, defaultShutdownTimeoutStr)
			So(warned, ShouldBeEmpty)
		})

		PatchConvey("非法值回落默认值并告警", func() {
			var warned string
			Mock(xutil.WarnIfEnableDebug).To(func(msg string, args ...any) { warned = msg }).Build()
			c := configMergeDefault(&Config{ShutdownTimeout: "not-a-duration"})
			So(c.ShutdownTimeout, ShouldEqual, defaultShutdownTimeoutStr)
			So(warned, ShouldContainSubstring, "ShutdownTimeout is invalid")
		})

		PatchConvey("合法值保留", func() {
			c := configMergeDefault(&Config{ShutdownTimeout: "30s"})
			So(c.ShutdownTimeout, ShouldEqual, "30s")
		})
	})
}
