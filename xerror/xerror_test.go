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

func TestIs_Nested(t *testing.T) {
	PatchConvey("TestIs-嵌套错误链", t, func() {
		// 模块之间会互相包装错误（xgorm 初始化失败里裹着 xconfig 的错误），
		// 只比对第一个 XOneError 会让 Is(err, "xconfig") 返回 false
		inner := New("xconfig", "load", errors.New("file not found"))
		middle := New("xgorm", "init", inner)
		outer := New("xserver", "run", middle)

		PatchConvey("链上每一层都能命中", func() {
			So(Is(outer, "xserver"), ShouldBeTrue)
			So(Is(outer, "xgorm"), ShouldBeTrue)
			So(Is(outer, "xconfig"), ShouldBeTrue)
		})

		PatchConvey("不在链上的模块返回 false", func() {
			So(Is(outer, "xredis"), ShouldBeFalse)
		})

		PatchConvey("非 XOneError 返回 false", func() {
			So(Is(errors.New("plain"), "xgorm"), ShouldBeFalse)
			So(Is(nil, "xgorm"), ShouldBeFalse)
		})

		PatchConvey("穿透 %w 包装", func() {
			wrapped := New("xgorm", "init", fmt.Errorf("wrapped: %w", inner))
			So(Is(wrapped, "xgorm"), ShouldBeTrue)
			So(Is(wrapped, "xconfig"), ShouldBeTrue)
		})

		PatchConvey("Module 取最外层", func() {
			So(Module(outer), ShouldEqual, "xserver")
		})
	})
}
