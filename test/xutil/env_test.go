package xutil

import (
	"os"
	"testing"

	"github.com/xiaoshicae/xone/v2/xutil"

	. "github.com/bytedance/mockey"
	. "github.com/smartystreets/goconvey/convey"
)

func TestEnv(t *testing.T) {
	PatchConvey("TestEnv", t, func() {
		PatchConvey("TestEnv-NotDebug", func() {
			Mock(os.Getenv).Return("").Build()
			e := xutil.EnableDebug()
			So(e, ShouldBeFalse)
		})

		PatchConvey("TestEnv-Debug1", func() {
			Mock(os.Getenv).Return("True").Build()
			e := xutil.EnableDebug()
			So(e, ShouldBeTrue)
		})

		PatchConvey("TestEnv-Debug2", func() {
			Mock(os.Getenv).Return("true").Build()
			e := xutil.EnableDebug()
			So(e, ShouldBeTrue)
		})

		PatchConvey("TestEnv-Debug3", func() {
			Mock(os.Getenv).Return("1").Build()
			e := xutil.EnableDebug()
			So(e, ShouldBeTrue)
		})
	})
}
