package xutil

import (
	"testing"

	"github.com/xiaoshicae/xone/xutil"

	. "github.com/bytedance/mockey"
	. "github.com/smartystreets/goconvey/convey"
)

func TestJson(t *testing.T) {
	PatchConvey("TestJson", t, func() {
		s := xutil.ToJsonString("null")
		So(s, ShouldEqual, `"null"`)

		s = xutil.ToJsonString(&JsonStruct{X: "abc", YZ: 123})
		So(s, ShouldEqual, `{"x":"abc","y_z":123}`)

		s = xutil.ToJsonStringIndent(&JsonStruct{X: "abc", YZ: 123})
		ss := `{
	"x": "abc",
	"y_z": 123
}`
		So(s, ShouldEqual, ss)
	})
}

type JsonStruct struct {
	X  string `json:"x"`
	YZ int    `json:"y_z"`
}
