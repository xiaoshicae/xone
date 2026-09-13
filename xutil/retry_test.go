package xutil

import (
	"context"
	"errors"
	"testing"
	"time"

	. "github.com/bytedance/mockey"
	. "github.com/smartystreets/goconvey/convey"
)

func TestRetryBasic(t *testing.T) {
	PatchConvey("TestRetryBasic", t, func() {
		boom := errors.New("boom")

		PatchConvey("首次成功不再重试", func() {
			calls := 0
			So(Retry(func() error { calls++; return nil }, 3, time.Millisecond), ShouldBeNil)
			So(calls, ShouldEqual, 1)
		})

		PatchConvey("失败时重试到次数用尽并返回最后错误", func() {
			calls := 0
			err := Retry(func() error { calls++; return boom }, 3, time.Millisecond)
			So(calls, ShouldEqual, 3)
			So(errors.Is(err, boom), ShouldBeTrue)
		})

		PatchConvey("attempts <= 0 时只执行一次", func() {
			calls := 0
			So(Retry(func() error { calls++; return nil }, 0, 0), ShouldBeNil)
			So(calls, ShouldEqual, 1)

			calls = 0
			_ = Retry(func() error { calls++; return boom }, -5, 0)
			So(calls, ShouldEqual, 1)
		})

		PatchConvey("中途成功则停止", func() {
			calls := 0
			err := Retry(func() error {
				calls++
				if calls == 2 {
					return nil
				}
				return boom
			}, 5, time.Millisecond)
			So(err, ShouldBeNil)
			So(calls, ShouldEqual, 2)
		})
	})
}

func TestRetryWithContext(t *testing.T) {
	PatchConvey("TestRetryWithContext", t, func() {
		boom := errors.New("boom")

		PatchConvey("ctx 取消后不再重试，返回已发生的业务错误", func() {
			ctx, cancel := context.WithCancel(context.Background())
			calls := 0
			err := RetryWithContext(ctx, func(context.Context) error {
				calls++
				cancel() // 第一次失败后调用方放弃
				return boom
			}, 10, time.Hour)

			So(calls, ShouldEqual, 1)
			So(errors.Is(err, boom), ShouldBeTrue)
		})

		PatchConvey("一开始就取消则返回 ctx.Err()", func() {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			calls := 0
			err := RetryWithContext(ctx, func(context.Context) error { calls++; return nil }, 3, 0)

			So(calls, ShouldEqual, 0)
			So(errors.Is(err, context.Canceled), ShouldBeTrue)
		})

		PatchConvey("等待期间被取消时立即返回，不硬拖满 sleep", func() {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
			defer cancel()

			start := time.Now()
			err := RetryWithContext(ctx, func(context.Context) error { return boom }, 5, time.Hour)
			elapsed := time.Since(start)

			So(errors.Is(err, boom), ShouldBeTrue)
			So(elapsed, ShouldBeLessThan, time.Second)
		})

		PatchConvey("ctx 传递给业务函数", func() {
			type key struct{}
			ctx := context.WithValue(context.Background(), key{}, "v")
			var got any
			So(RetryWithContext(ctx, func(c context.Context) error {
				got = c.Value(key{})
				return nil
			}, 1, 0), ShouldBeNil)
			So(got, ShouldEqual, "v")
		})
	})
}

func TestNextBackoff(t *testing.T) {
	PatchConvey("TestNextBackoff", t, func() {
		PatchConvey("正常翻倍", func() {
			So(nextBackoff(time.Second, time.Minute), ShouldEqual, 2*time.Second)
			So(nextBackoff(2*time.Second, time.Minute), ShouldEqual, 4*time.Second)
		})

		PatchConvey("达到上限后不再增长", func() {
			So(nextBackoff(40*time.Second, time.Minute), ShouldEqual, time.Minute)
			So(nextBackoff(time.Minute, time.Minute), ShouldEqual, time.Minute)
			So(nextBackoff(2*time.Minute, time.Minute), ShouldEqual, time.Minute)
		})

		PatchConvey("无上限时不溢出成负数", func() {
			// 旧实现先翻倍再比上限，二十来次后 int64 溢出，退避彻底失效
			d := time.Hour
			for range 100 {
				d = nextBackoff(d, 0)
				So(d, ShouldBeGreaterThan, 0)
			}
		})

		PatchConvey("有上限时连续翻倍最终收敛到上限且始终为正", func() {
			d := time.Nanosecond
			for range 100 {
				d = nextBackoff(d, time.Minute)
				So(d, ShouldBeGreaterThan, 0)
			}
			So(d, ShouldEqual, time.Minute)
		})
	})
}

func TestRetryWithBackoff(t *testing.T) {
	PatchConvey("TestRetryWithBackoff", t, func() {
		boom := errors.New("boom")

		PatchConvey("重试到次数用尽", func() {
			calls := 0
			err := RetryWithBackoff(func() error { calls++; return boom }, 3, time.Millisecond, 10*time.Millisecond)
			So(calls, ShouldEqual, 3)
			So(errors.Is(err, boom), ShouldBeTrue)
		})

		PatchConvey("首次成功不重试", func() {
			calls := 0
			So(RetryWithBackoff(func() error { calls++; return nil }, 3, time.Millisecond, time.Second), ShouldBeNil)
			So(calls, ShouldEqual, 1)
		})

		PatchConvey("attempts <= 0 时只执行一次", func() {
			calls := 0
			So(RetryWithBackoff(func() error { calls++; return nil }, 0, 0, 0), ShouldBeNil)
			So(calls, ShouldEqual, 1)
		})

		PatchConvey("可取消版本在 ctx 结束时停止", func() {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
			defer cancel()

			start := time.Now()
			err := RetryWithBackoffContext(ctx, func(context.Context) error { return boom }, 10, time.Hour, 10*time.Hour)
			So(errors.Is(err, boom), ShouldBeTrue)
			So(time.Since(start), ShouldBeLessThan, time.Second)
		})
	})
}

// TestRetryContextCancelledBeforeAnyError ctx 先于任何业务错误结束时返回 ctx.Err()
func TestRetryContextCancelledBeforeAnyError(t *testing.T) {
	PatchConvey("TestRetryContextCancelledBeforeAnyError", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		calls := 0
		err := RetryWithContext(ctx, func(context.Context) error {
			calls++
			return nil
		}, 3, time.Millisecond)

		So(calls, ShouldEqual, 0)
		So(errors.Is(err, context.Canceled), ShouldBeTrue)
	})
}

// TestRetryContextCancelledBetweenAttempts 无等待间隔时，下一轮开头检出取消并保留业务错误
func TestRetryContextCancelledBetweenAttempts(t *testing.T) {
	PatchConvey("TestRetryContextCancelledBetweenAttempts", t, func() {
		boom := errors.New("boom")
		ctx, cancel := context.WithCancel(context.Background())

		calls := 0
		// sleep 为 0：不走等待分支，取消只能在下一轮循环开头被检出
		err := RetryWithContext(ctx, func(context.Context) error {
			calls++
			cancel()
			return boom
		}, 3, 0)

		So(calls, ShouldEqual, 1)
		// 业务错误比"被取消"更有信息量，应优先返回
		So(errors.Is(err, boom), ShouldBeTrue)
		So(errors.Is(err, context.Canceled), ShouldBeFalse)
	})
}
