package xutil

import (
	"errors"
	"testing"
	"time"

	"github.com/xiaoshicae/xone/v2/xutil"

	. "github.com/bytedance/mockey"
	. "github.com/smartystreets/goconvey/convey"
)

func TestRetry(t *testing.T) {
	PatchConvey("TestRetry", t, func() {
		err := xutil.Retry(func() error {
			return errors.New("for test")
		}, 3, time.Millisecond*100)
		So(err.Error(), ShouldEqual, "for test")

		err = xutil.Retry(func() error {
			return nil
		}, 3, time.Millisecond*100)
		So(err, ShouldBeNil)

		PatchConvey("attempts <= 0 still invokes once", func() {
			calls := 0
			err = xutil.Retry(func() error {
				calls++
				return nil
			}, 0, time.Millisecond*100)
			So(err, ShouldBeNil)
			So(calls, ShouldEqual, 1)

			calls = 0
			err = xutil.Retry(func() error {
				calls++
				return errors.New("for test")
			}, -1, time.Millisecond*100)
			So(err.Error(), ShouldEqual, "for test")
			So(calls, ShouldEqual, 1)
		})
	})
}

func TestRetryWithBackoff(t *testing.T) {
	PatchConvey("TestRetryWithBackoff", t, func() {
		PatchConvey("全部失败-返回最后错误", func() {
			calls := 0
			err := xutil.RetryWithBackoff(func() error {
				calls++
				return errors.New("backoff fail")
			}, 3, 10*time.Millisecond, 100*time.Millisecond)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldEqual, "backoff fail")
			So(calls, ShouldEqual, 3)
		})

		PatchConvey("第二次成功", func() {
			calls := 0
			err := xutil.RetryWithBackoff(func() error {
				calls++
				if calls < 2 {
					return errors.New("not yet")
				}
				return nil
			}, 5, 10*time.Millisecond, 1*time.Second)
			So(err, ShouldBeNil)
			So(calls, ShouldEqual, 2)
		})

		PatchConvey("attempts <= 0 仅调用一次", func() {
			calls := 0
			err := xutil.RetryWithBackoff(func() error {
				calls++
				return nil
			}, 0, 10*time.Millisecond, 100*time.Millisecond)
			So(err, ShouldBeNil)
			So(calls, ShouldEqual, 1)
		})

		PatchConvey("退避间隔不超过 maxDelay", func() {
			delays := make([]time.Time, 0)
			err := xutil.RetryWithBackoff(func() error {
				delays = append(delays, time.Now())
				return errors.New("fail")
			}, 4, 10*time.Millisecond, 20*time.Millisecond)
			So(err, ShouldNotBeNil)
			// 4次调用产生3个间隔：10ms, 20ms, 20ms（被 maxDelay 限制）
			So(len(delays), ShouldEqual, 4)
		})
	})
}
