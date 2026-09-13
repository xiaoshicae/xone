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

// TestAsyncWithPoolOnClosedPool 池已关闭时 Future 必须立刻完成，而不是永久阻塞
func TestAsyncWithPoolOnClosedPool(t *testing.T) {
	PatchConvey("TestAsyncWithPoolOnClosedPool", t, func() {
		p := NewPool(2)
		p.Shutdown()

		f := AsyncWithPool(p, func() (int, error) { return 1, nil })

		val, err := f.GetWithTimeout(500 * time.Millisecond)
		So(val, ShouldEqual, 0)
		So(errors.Is(err, ErrPoolClosed), ShouldBeTrue)
		So(errors.Is(err, context.DeadlineExceeded), ShouldBeFalse) // 不是超时，是立即返回
		So(f.IsDone(), ShouldBeTrue)
	})
}

// TestAsyncWithPoolPanicBecomesError 池中任务的 panic 同样转成 error
func TestAsyncWithPoolPanicBecomesError(t *testing.T) {
	PatchConvey("TestAsyncWithPoolPanicBecomesError", t, func() {
		p := NewPool(1)
		defer p.Shutdown()

		f := AsyncWithPool(p, func() (string, error) { panic("池任务炸了") })
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
		So(TrySubmit(func() { close(done) }), ShouldBeTrue)
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

func TestTrySubmit(t *testing.T) {
	PatchConvey("TestTrySubmit", t, func() {
		PatchConvey("nil 任务不接收", func() {
			So(NewPool(1).TrySubmit(nil), ShouldBeFalse)
		})

		PatchConvey("队列满时立即返回而不阻塞", func() {
			// 阻塞背压只对自建池成立；不想等的调用方用 TrySubmit
			p := NewPool(1)
			block := make(chan struct{})
			defer close(block)

			So(p.Submit(func() { <-block }), ShouldBeTrue) // 占住唯一的 worker
			time.Sleep(30 * time.Millisecond)
			for range taskQueuePerWorker { // 填满队列
				So(p.Submit(func() {}), ShouldBeTrue)
			}

			start := time.Now()
			So(p.TrySubmit(func() {}), ShouldBeFalse)
			So(time.Since(start), ShouldBeLessThan, 100*time.Millisecond)
		})

		PatchConvey("池关闭后不接收", func() {
			p := NewPool(1)
			p.Shutdown()
			So(p.TrySubmit(func() {}), ShouldBeFalse)
		})

		PatchConvey("正常提交会被执行", func() {
			p := NewPool(1)
			defer p.Shutdown()
			done := make(chan struct{})
			So(p.TrySubmit(func() { close(done) }), ShouldBeTrue)
			select {
			case <-done:
			case <-time.After(2 * time.Second):
				t.Fatal("任务未执行")
			}
		})
	})
}

func TestGlobalTrySubmit(t *testing.T) {
	PatchConvey("TestGlobalTrySubmit", t, func() {
		PatchConvey("nil 任务不接收", func() {
			So(TrySubmit(nil), ShouldBeFalse)
		})

		PatchConvey("正常提交会被执行", func() {
			done := make(chan struct{})
			So(TrySubmit(func() { close(done) }), ShouldBeTrue)
			select {
			case <-done:
			case <-time.After(2 * time.Second):
				t.Fatal("任务未执行")
			}
		})

		PatchConvey("提交失败时记录日志，便于排查静默丢弃", func() {
			logged := 0
			Mock(ErrorIfEnableDebug).To(func(_ string, _ ...any) { logged++ }).Build()
			Mock((*Pool).TrySubmit).Return(false).Build()

			So(TrySubmit(func() {}), ShouldBeFalse)
			So(logged, ShouldEqual, 1)
		})
	})
}

func TestSubmitBlockedWakesOnShutdown(t *testing.T) {
	PatchConvey("TestSubmitBlockedWakesOnShutdown", t, func() {
		// 阻塞中的 Submit 必须能被 Shutdown 唤醒：
		// 否则它会一直持着读锁，Shutdown 永远取不到写锁，
		// 而 Go 的 RWMutex 写者优先，后续所有 Submit 也会跟着挂起
		p := NewPool(1)
		block := make(chan struct{})
		defer close(block)

		So(p.Submit(func() { <-block }), ShouldBeTrue)
		time.Sleep(30 * time.Millisecond)
		for range taskQueuePerWorker {
			So(p.Submit(func() {}), ShouldBeTrue)
		}

		blocked := make(chan bool, 1)
		go func() { blocked <- p.Submit(func() {}) }()
		time.Sleep(30 * time.Millisecond)

		closed := make(chan struct{})
		go func() {
			p.stopOnce.Do(func() {
				close(p.done)
				p.mu.Lock()
				p.closed = true
				close(p.tasks)
				p.mu.Unlock()
			})
			close(closed)
		}()

		select {
		case <-closed:
		case <-time.After(2 * time.Second):
			t.Fatal("Shutdown 取不到写锁，被阻塞中的 Submit 拖住")
		}

		select {
		case ok := <-blocked:
			So(ok, ShouldBeFalse) // 关闭时未入队，如实返回 false
		case <-time.After(2 * time.Second):
			t.Fatal("阻塞中的 Submit 没有被唤醒")
		}
	})
}

func TestTryAsyncWithPool(t *testing.T) {
	PatchConvey("TestTryAsyncWithPool", t, func() {
		PatchConvey("正常提交拿到结果", func() {
			p := NewPool(1)
			defer p.Shutdown()

			v, err := TryAsyncWithPool(p, func() (int, error) { return 42, nil }).Get()
			So(err, ShouldBeNil)
			So(v, ShouldEqual, 42)
		})

		PatchConvey("队列满时不阻塞，以 ErrPoolFull 完成", func() {
			// 与 AsyncWithPool 的区别就在这里：宁可降级也不要等
			p := NewPool(1)
			block := make(chan struct{})
			defer close(block)

			So(p.Submit(func() { <-block }), ShouldBeTrue)
			time.Sleep(30 * time.Millisecond)
			for range taskQueuePerWorker {
				So(p.Submit(func() {}), ShouldBeTrue)
			}

			start := time.Now()
			_, err := TryAsyncWithPool(p, func() (int, error) { return 1, nil }).Get()
			So(time.Since(start), ShouldBeLessThan, 100*time.Millisecond)
			So(errors.Is(err, ErrPoolFull), ShouldBeTrue)
		})

		PatchConvey("池已关闭时报 ErrPoolClosed 而非 ErrPoolFull", func() {
			// 关闭是永久状态、不必重试；队列满是瞬时的，两者处置方式不同
			p := NewPool(1)
			p.Shutdown()

			_, err := TryAsyncWithPool(p, func() (int, error) { return 1, nil }).Get()
			So(errors.Is(err, ErrPoolClosed), ShouldBeTrue)
			So(errors.Is(err, ErrPoolFull), ShouldBeFalse)
		})

		PatchConvey("任务 panic 同样转成 error", func() {
			p := NewPool(1)
			defer p.Shutdown()

			_, err := TryAsyncWithPool(p, func() (string, error) { panic("炸了") }).Get()
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "炸了")
		})
	})
}

func TestPoolIsClosed(t *testing.T) {
	PatchConvey("TestPoolIsClosed", t, func() {
		p := NewPool(1)
		So(p.isClosed(), ShouldBeFalse)
		p.Shutdown()
		So(p.isClosed(), ShouldBeTrue)
	})
}
