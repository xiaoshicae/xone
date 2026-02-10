package xutil

import (
	"testing"

	"github.com/xiaoshicae/xone/v2/xutil"

	. "github.com/bytedance/mockey"
	. "github.com/smartystreets/goconvey/convey"
)

func TestReflect(t *testing.T) {
	PatchConvey("TestReflect", t, func() {
		PatchConvey("test IsSlice", func() {
			var x []string
			s1 := xutil.IsSlice(x)
			So(s1, ShouldBeTrue)

			x = make([]string, 0)
			s2 := xutil.IsSlice(x)
			So(s2, ShouldBeTrue)

			s3 := xutil.IsSlice(nil)
			So(s3, ShouldBeFalse)

			s4 := xutil.IsSlice("nil")
			So(s4, ShouldBeFalse)

			s5 := xutil.IsSlice(1)
			So(s5, ShouldBeFalse)

			s6 := xutil.IsSlice(true)
			So(s6, ShouldBeFalse)
		})
	})
}

func TestGetFuncName(t *testing.T) {
	PatchConvey("TestGetFuncName", t, func() {
		name := xutil.GetFuncName(MyFunc)
		So(name, ShouldEqual, "MyFunc")

		f := func() {}
		name2 := xutil.GetFuncName(f)
		So(name2, ShouldEqual, "TestGetFuncName.func1.1")
	})
}

func MyFunc() {

}
