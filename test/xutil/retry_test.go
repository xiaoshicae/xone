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
