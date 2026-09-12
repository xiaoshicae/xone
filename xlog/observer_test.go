package xlog

import (
	"context"
	"sync"
	"testing"

	"github.com/bytedance/mockey"
	"github.com/sirupsen/logrus"
	c "github.com/smartystreets/goconvey/convey"
)

// withCleanObservers 在隔离的观察者列表上执行，避免用例之间互相污染
func withCleanObservers(f func()) {
	saved := observers.Load()
	observers.Store(nil)
	defer observers.Store(saved)
	f()
}

func TestAddObserver(t *testing.T) {
	mockey.PatchConvey("TestAddObserver", t, func() {
		mockey.PatchConvey("TestAddObserver-Nil被忽略", func() {
			withCleanObservers(func() {
				AddObserver(nil)
				c.So(observers.Load(), c.ShouldBeNil)
			})
		})

		mockey.PatchConvey("TestAddObserver-多个观察者按注册顺序触发", func() {
			withCleanObservers(func() {
				var order []string
				AddObserver(func(context.Context, Record) { order = append(order, "first") })
				AddObserver(func(context.Context, Record) { order = append(order, "second") })

				notifyObservers(context.Background(), Record{Level: ErrorLevel})
				c.So(order, c.ShouldResemble, []string{"first", "second"})
			})
		})

		mockey.PatchConvey("TestAddObserver-无观察者时不panic", func() {
			withCleanObservers(func() {
				notifyObservers(context.Background(), Record{Level: InfoLevel})
			})
		})

		mockey.PatchConvey("TestAddObserver-并发注册安全", func() {
			withCleanObservers(func() {
				var wg sync.WaitGroup
				for i := 0; i < 20; i++ {
					wg.Add(1)
					go func() {
						defer wg.Done()
						AddObserver(func(context.Context, Record) {})
					}()
				}
				wg.Wait()
				c.So(lenOf(observers.Load()), c.ShouldEqual, 20)
			})
		})
	})
}

// TestObserverReceivesLogRecord 回归防护：
// 日志必须能被旁路观察者感知，xmetric 的错误指标依赖该链路。
// 此前 xlog 改用私有 logrus 实例后，注册在全局 logrus 上的 hook 静默失效过。
func TestObserverReceivesLogRecord(t *testing.T) {
	mockey.PatchConvey("TestObserverReceivesLogRecord", t, func() {
		withCleanObservers(func() {
			var got []Record
			AddObserver(func(_ context.Context, r Record) { got = append(got, r) })

			c.So(initXLogByConfig(&Config{Level: "debug"}), c.ShouldBeNil)
			logger.ReplaceHooks(logrus.LevelHooks{})
			logger.AddHook(&xLogHook{SuffixToIgnore: findFrameIgnoreFileNames})

			Error(context.Background(), "业务错误")
			Info(context.Background(), "普通信息")

			c.So(len(got), c.ShouldEqual, 2)
			c.So(got[0].Level, c.ShouldEqual, ErrorLevel)
			c.So(got[0].Message, c.ShouldEqual, "业务错误")
			// 调用位置应指向业务代码而非 xlog 内部
			c.So(got[0].File, c.ShouldEqual, "observer_test.go")
			c.So(got[0].Line, c.ShouldBeGreaterThan, 0)
			c.So(got[1].Level, c.ShouldEqual, InfoLevel)
		})
	})
}
