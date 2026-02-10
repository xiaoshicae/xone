package xerror

import (
	"errors"
	"fmt"
	"testing"

	. "github.com/bytedance/mockey"
	. "github.com/smartystreets/goconvey/convey"
)

func TestXOneError_Error(t *testing.T) {
	PatchConvey("TestXOneError_Error", t, func() {
		PatchConvey("包含原始错误", func() {
			err := New("xconfig", "init", errors.New("file not found"))
			So(err.Error(), ShouldEqual, "XOne xconfig init failed, err=[file not found]")
		})

		PatchConvey("无原始错误", func() {
			err := New("xgorm", "close", nil)
			So(err.Error(), ShouldEqual, "XOne xgorm close failed")
		})
	})
}

func TestXOneError_Unwrap(t *testing.T) {
	PatchConvey("TestXOneError_Unwrap", t, func() {
		inner := errors.New("inner error")
		err := New("xlog", "init", inner)
		So(errors.Is(err, inner), ShouldBeTrue)
	})
}

func TestNew(t *testing.T) {
	PatchConvey("TestNew", t, func() {
		err := New("xhttp", "init", errors.New("timeout"))
		So(err.Module, ShouldEqual, "xhttp")
		So(err.Op, ShouldEqual, "init")
		So(err.Err.Error(), ShouldEqual, "timeout")
	})
}

func TestNewf(t *testing.T) {
	PatchConvey("TestNewf", t, func() {
		err := Newf("xgorm", "init", "connect failed, host=[%s]", "localhost")
		So(err.Module, ShouldEqual, "xgorm")
		So(err.Op, ShouldEqual, "init")
		So(err.Err.Error(), ShouldEqual, "connect failed, host=[localhost]")
	})
}

func TestIs(t *testing.T) {
	PatchConvey("TestIs", t, func() {
		PatchConvey("匹配模块名", func() {
			err := New("xconfig", "init", errors.New("parse error"))
			So(Is(err, "xconfig"), ShouldBeTrue)
			So(Is(err, "xlog"), ShouldBeFalse)
		})

		PatchConvey("wrapped 错误链", func() {
			inner := New("xtrace", "init", errors.New("exporter failed"))
			wrapped := fmt.Errorf("outer: %w", inner)
			So(Is(wrapped, "xtrace"), ShouldBeTrue)
			So(Is(wrapped, "xconfig"), ShouldBeFalse)
		})

		PatchConvey("非 XOneError", func() {
			err := errors.New("plain error")
			So(Is(err, "xconfig"), ShouldBeFalse)
		})
	})
}

func TestModule(t *testing.T) {
	PatchConvey("TestModule", t, func() {
		PatchConvey("XOneError 提取模块名", func() {
			err := New("xcache", "close", nil)
			So(Module(err), ShouldEqual, "xcache")
		})

		PatchConvey("非 XOneError 返回空", func() {
			err := errors.New("plain")
			So(Module(err), ShouldEqual, "")
		})
	})
}
