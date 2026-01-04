package xutil

import (
	"testing"

	"github.com/xiaoshicae/xone/xutil"

	. "github.com/bytedance/mockey"
	. "github.com/smartystreets/goconvey/convey"
)

func TestFileExist(t *testing.T) {
	PatchConvey("TestFileExist", t, func() {
		exist := xutil.FileExist("not_exist.txt")
		So(exist, ShouldBeFalse)

		exist = xutil.FileExist("./f.txt")
		So(exist, ShouldBeTrue)
	})
}

func TestDirExist(t *testing.T) {
	PatchConvey("TestDirExist", t, func() {
		exist := xutil.DirExist("not_exist")
		So(exist, ShouldBeFalse)

		exist = xutil.DirExist("./d/")
		So(exist, ShouldBeTrue)

		exist = xutil.FileExist("./d/f.txt")
		So(exist, ShouldBeTrue)
	})
}
