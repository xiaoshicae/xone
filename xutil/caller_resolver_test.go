package xutil

import (
	"runtime"
	"testing"

	"github.com/bytedance/mockey"
	c "github.com/smartystreets/goconvey/convey"
)

func TestCallerResolver(t *testing.T) {
	mockey.PatchConvey("TestCallerResolver", t, func() {
		mockey.PatchConvey("与非缓存实现结果一致", func() {
			// 逐 PC 解析需正确处理内联展开，结果必须与整段解析一致
			ignore := []string{"/xutil/caller_resolver_test.go"}
			r := NewCallerResolver(ignore)

			cached := r.Caller(0)
			uncached := GetLogCaller(0, ignore)

			c.So(cached, c.ShouldNotBeNil)
			c.So(uncached, c.ShouldNotBeNil)
			c.So(cached.File, c.ShouldEqual, uncached.File)
		})

		mockey.PatchConvey("命中缓存后结果稳定", func() {
			// 同一调用点重复解析，第二次起走缓存，结果必须一致
			r := NewCallerResolver(nil)
			var frames []*runtime.Frame
			for i := 0; i < 3; i++ {
				frames = append(frames, callerAtLineA(r))
			}
			c.So(frames[0], c.ShouldNotBeNil)
			c.So(frames[1].File, c.ShouldEqual, frames[0].File)
			c.So(frames[1].Line, c.ShouldEqual, frames[0].Line)
			c.So(frames[2].Line, c.ShouldEqual, frames[0].Line)
		})

		mockey.PatchConvey("不同调用点互不干扰", func() {
			// 缓存以 PC 为键，不应把一个调用点的结果返回给另一个
			r := NewCallerResolver(nil)
			a := callerAtLineA(r)
			b := callerAtLineB(r)
			c.So(a, c.ShouldNotBeNil)
			c.So(b, c.ShouldNotBeNil)
			c.So(a.Line, c.ShouldNotEqual, b.Line)
		})

		mockey.PatchConvey("全部帧被忽略时返回兜底帧", func() {
			// 忽略所有 .go 文件，应返回最后一帧而非 nil
			r := NewCallerResolver([]string{".go", ".s"})
			c.So(r.Caller(0), c.ShouldNotBeNil)
		})

		mockey.PatchConvey("栈深度不足时返回 nil", func() {
			r := NewCallerResolver(nil)
			c.So(r.Caller(1000), c.ShouldBeNil)
		})

		mockey.PatchConvey("并发解析安全", func() {
			r := NewCallerResolver(nil)
			done := make(chan struct{})
			for i := 0; i < 20; i++ {
				go func() {
					defer func() { done <- struct{}{} }()
					_ = r.Caller(0)
				}()
			}
			for i := 0; i < 20; i++ {
				<-done
			}
		})
	})
}

//go:noinline
func callerAtLineA(r *CallerResolver) *runtime.Frame { return r.Caller(0) }

//go:noinline
func callerAtLineB(r *CallerResolver) *runtime.Frame { return r.Caller(0) }
