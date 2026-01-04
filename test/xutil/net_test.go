package xutil

import (
	"testing"

	"github.com/xiaoshicae/xone/xutil"

	. "github.com/bytedance/mockey"
	. "github.com/smartystreets/goconvey/convey"
)

func TestGetLocalIp(t *testing.T) {
	PatchConvey("TestGetLocalIp", t, func() {
		ip, err := xutil.GetLocalIp()
		So(err, ShouldBeNil)
		t.Log(ip)
	})
}

func TestExtractRealIP(t *testing.T) {
	PatchConvey("TestExtractRealIP", t, func() {
		ip, err := xutil.ExtractRealIP("192.168.0.1")
		So(err, ShouldBeNil)
		So(ip, ShouldEqual, "192.168.0.1")

		ip, err = xutil.ExtractRealIP("192.168.0.1:8080")
		So(err, ShouldBeNil)
		So(ip, ShouldEqual, "192.168.0.1")

		ip, err = xutil.ExtractRealIP("[::1]:8080")
		So(err, ShouldBeNil)
		So(ip, ShouldEqual, "::1")

		_, err = xutil.ExtractRealIP("not-an-ip")
		So(err, ShouldNotBeNil)
		So(err.Error(), ShouldContainSubstring, "is invalid")
	})
}
