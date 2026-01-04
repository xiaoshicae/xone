package xconfig

import (
	"os"
	"testing"

	"github.com/xiaoshicae/xone"
	"github.com/xiaoshicae/xone/xconfig"

	. "github.com/bytedance/mockey"
	. "github.com/smartystreets/goconvey/convey"
)

func TestXConfig(t *testing.T) {
	PatchConvey("TestXConfig", t, func() {
		if err := xone.R(); err != nil {
			panic(err)
		}

		So(os.Getenv("x"), ShouldEqual, "123")
		So(os.Getenv("y"), ShouldEqual, "456")
		So(xconfig.GetConfig("A.B.C"), ShouldEqual, "a.b.c")
		So(xconfig.GetConfig("A.B.D"), ShouldEqual, "123")
		So(xconfig.GetConfig("A.B.E"), ShouldEqual, "456")
		So(xconfig.GetConfig("A.B.F"), ShouldEqual, "789")
	})
}
