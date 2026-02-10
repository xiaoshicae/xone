package xutil

import (
	"testing"
	"time"

	"github.com/xiaoshicae/xone/v2/xutil"

	. "github.com/bytedance/mockey"
	. "github.com/smartystreets/goconvey/convey"
)

func TestToDuration(t *testing.T) {
	PatchConvey("TestToDuration", t, func() {
		d := xutil.ToDuration("1ms")
		So(d, ShouldEqual, time.Millisecond)

		d = xutil.ToDuration("1s")
		So(d, ShouldEqual, time.Second)

		d = xutil.ToDuration("1m")
		So(d, ShouldEqual, time.Minute)

		d = xutil.ToDuration("1h")
		So(d, ShouldEqual, time.Hour)

		d = xutil.ToDuration("1d")
		So(d, ShouldEqual, time.Hour*24)

		d = xutil.ToDuration("1d")
		So(d, ShouldEqual, time.Hour*24)

		d = xutil.ToDuration(xutil.ToPrt("1d"))
		So(d, ShouldEqual, time.Hour*24)

		d = xutil.ToDuration("2d2h2m2s")
		So(d, ShouldEqual, time.Hour*24*2+time.Hour*2+time.Minute*2+time.Second*2)

		d = xutil.ToDuration(xutil.ToPrt("2d2h2m2s"))
		So(d, ShouldEqual, time.Hour*24*2+time.Hour*2+time.Minute*2+time.Second*2)
	})
}

func TestConvert(t *testing.T) {
	PatchConvey("TestConvert", t, func() {
		str := xutil.GetOrDefault("", "1")
		So(str, ShouldEqual, "1")

		str = xutil.GetOrDefault("2", "1")
		So(str, ShouldEqual, "2")

		i := xutil.GetOrDefault(0, 1)
		So(i, ShouldEqual, 1)

		i = xutil.GetOrDefault(2, 1)
		So(i, ShouldEqual, 2)

		strList := xutil.GetOrDefault([]string{}, []string{"1"})
		So(strList, ShouldResemble, []string{})

		strList = xutil.GetOrDefault([]string{"2"}, []string{"1"})
		So(strList, ShouldResemble, []string{"2"})

		ds := xutil.GetOrDefault(DemoStruct{}, DemoStruct{A: "aaa"})
		So(ds, ShouldResemble, DemoStruct{A: "aaa"})

		dsPrt := xutil.GetOrDefault(nil, &DemoStruct{A: "aaa"})
		So(dsPrt, ShouldResemble, &DemoStruct{A: "aaa"})
	})
}

type DemoStruct struct {
	A string
}
