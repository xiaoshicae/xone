package xutil

import (
	"strings"
	"testing"

	"github.com/xiaoshicae/xone/xutil"

	. "github.com/bytedance/mockey"
	. "github.com/smartystreets/goconvey/convey"
)

func TestGetConfigFromArgs(t *testing.T) {
	PatchConvey("TestGetConfigFromArgs", t, func() {
		var err error
		var c string

		PatchConvey("invalid key", func() {
			_, err = xutil.GetConfigFromArgs("1a")
			So(err.Error(), ShouldEqual, "key must match regexp: ^[a-zA-Z_][a-zA-Z0-9_.-]*$")

			_, err = xutil.GetConfigFromArgs("#a")
			So(err.Error(), ShouldEqual, "key must match regexp: ^[a-zA-Z_][a-zA-Z0-9_.-]*$")
		})

		PatchConvey("xconfig not  found", func() {
			Mock(xutil.GetOsArgs).Return(make([]string, 0)).Build()
			_, err = xutil.GetConfigFromArgs("x")
			So(err.Error(), ShouldEqual, "arg not found, there is no arg")
		})

		PatchConvey("xconfig test", func() {
			Mock(xutil.GetOsArgs).Return(strings.Split("-x.y.z=a_bc --baaa ww ---b===#123 -z", " ")).Build()
			_, err = xutil.GetConfigFromArgs("z")
			So(err.Error(), ShouldEqual, "arg not found, arg not set")

			c, err = xutil.GetConfigFromArgs("baaa")
			So(c, ShouldEqual, "ww")

			c, err = xutil.GetConfigFromArgs("b")
			So(c, ShouldEqual, "#123")

			c, err = xutil.GetConfigFromArgs("x.y.z")
			So(c, ShouldEqual, "a_bc")

			c, err = xutil.GetConfigFromArgs("a")
			So(err.Error(), ShouldEqual, "arg not found")
		})
	})
}

// go test -gcflags="all=-N -l" -run ^TestGetConfigFromArgsWithArg -args server.config.location=/a/b/application.yml
func TestGetConfigFromArgsWithArg(t *testing.T) {
	t.Skipf("跳过TestGetConfigFromArgsWithArg，请注释后，手动运行上面参数")
	PatchConvey("TestGetConfigFromArgsWithArg", t, func() {
		c, err := xutil.GetConfigFromArgs("server.config.location")
		So(err, ShouldBeNil)
		So(c, ShouldEqual, "/a/b/application.yml")
	})
}
