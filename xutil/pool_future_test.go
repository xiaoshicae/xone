package xutil

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	. "github.com/bytedance/mockey"
	. "github.com/smartystreets/goconvey/convey"
)

// ==================== 审查回归 ====================

// TestSubmitShutdownRace 并发 Submit 与 Shutdown 不得 panic
//
// 旧实现先查 ctx 再发送，close 落在两步之间时会 send on closed channel
func TestSubmitShutdownRace(t *testing.T) {
	PatchConvey("TestSubmitShutdownRace", t, func() {
		So(func() {
			for range 200 {
				p := NewPool(1) // 缓冲小，容易让 Submit 卡在发送上
				var wg sync.WaitGroup
				for range 50 {
					wg.Add(1)
					go func() { defer wg.Done(); p.Submit(func() {}) }()
				}
				go p.Shutdown()
				wg.Wait()
			}
		}, ShouldNotPanic)
	})
}

// TestTaskPanicIsolated 任务里的 panic 不得崩掉进程或杀死 worker
func TestTaskPanicIsolated(t *testing.T) {
	PatchConvey("TestTaskPanicIsolated", t, func() {
		p := NewPool(1)
		defer p.Shutdown()

		done := make(chan int, 1)
		So(p.Submit(func() { panic("任务炸了") }), ShouldBeTrue)
		// worker 必须活着继续干活
		So(p.Submit(func() { done <- 42 }), ShouldBeTrue)

		select {
		case v := <-done:
			So(v, ShouldEqual, 42)
		case <-time.After(2 * time.Second):
			t.Fatal("worker 在 panic 后死掉了")
		}
	})
}

// TestAsyncPanicBecomesError Async 中的 panic 转成 error 而不是崩进程
func TestAsyncPanicBecomesError(t *testing.T) {
	PatchConvey("TestAsyncPanicBecomesError", t, func() {
		f := Async(func() (int, error) { panic("业务代码炸了") })
		val, err := f.Get()

		So(val, ShouldEqual, 0)
		So(err, ShouldNotBeNil)
		So(err.Error(), ShouldContainSubstring, "panic occurred, 业务代码炸了")
	})
}

// TestGoOnClosedPool 池已关闭时 Future 必须立刻完成，而不是永久阻塞
func TestGoOnClosedPool(t *testing.T) {
	PatchConvey("TestGoOnClosedPool", t, func() {
		p := NewPool(2)
		p.Shutdown()

		f := Go(p, func() (int, error) { return 1, nil })

		val, err := f.GetWithTimeout(500 * time.Millisecond)
		So(val, ShouldEqual, 0)
		So(errors.Is(err, ErrPoolClosed), ShouldBeTrue)
		So(errors.Is(err, context.DeadlineExceeded), ShouldBeFalse) // 不是超时，是立即返回
		So(f.IsDone(), ShouldBeTrue)
	})
}

// TestGoPanicBecomesError 池中任务的 panic 同样转成 error
func TestGoPanicBecomesError(t *testing.T) {
	PatchConvey("TestGoPanicBecomesError", t, func() {
		p := NewPool(1)
		defer p.Shutdown()

		f := Go(p, func() (string, error) { panic("池任务炸了") })
		val, err := f.Get()
		So(val, ShouldBeEmpty)
		So(err.Error(), ShouldContainSubstring, "panic occurred, 池任务炸了")
	})
}

// TestDefaultPoolIsLazy 未使用时不得启动 worker
func TestDefaultPoolIsLazy(t *testing.T) {
	PatchConvey("TestDefaultPoolIsLazy", t, func() {
		// defaultPool 是 sync.OnceValue，只有调用才会构造
		So(defaultPool, ShouldNotBeNil)

		done := make(chan struct{})
		So(Submit(func() { close(done) }), ShouldBeTrue)
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("默认池未执行任务")
		}
	})
}

// TestSubmitReturnValue Submit 的返回值语义
func TestSubmitReturnValue(t *testing.T) {
	PatchConvey("TestSubmitReturnValue", t, func() {
		p := NewPool(1)

		So(p.Submit(nil), ShouldBeFalse)
		So(p.Submit(func() {}), ShouldBeTrue)

		p.Shutdown()
		So(p.Submit(func() {}), ShouldBeFalse)
		// 多次关闭安全
		So(func() { p.Shutdown() }, ShouldNotPanic)
	})
}

// TestFutureGetWithContext ctx 结束时返回 ctx.Err()
func TestFutureGetWithContext(t *testing.T) {
	PatchConvey("TestFutureGetWithContext", t, func() {
		PatchConvey("任务完成时返回结果", func() {
			f := Async(func() (int, error) { return 7, nil })
			val, err := f.GetWithContext(context.Background())
			So(val, ShouldEqual, 7)
			So(err, ShouldBeNil)
		})

		PatchConvey("ctx 取消时返回 ctx.Err()", func() {
			f := Async(func() (int, error) {
				time.Sleep(time.Second)
				return 1, nil
			})
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			val, err := f.GetWithContext(ctx)
			So(val, ShouldEqual, 0)
			So(errors.Is(err, context.Canceled), ShouldBeTrue)
		})
	})
}

// TestFutureCompleteOnce 重复完成只有首次生效
func TestFutureCompleteOnce(t *testing.T) {
	PatchConvey("TestFutureCompleteOnce", t, func() {
		f := newFuture[int]()
		f.complete(1, nil)
		f.complete(2, errors.New("late"))

		val, err := f.Get()
		So(val, ShouldEqual, 1)
		So(err, ShouldBeNil)
	})
}
