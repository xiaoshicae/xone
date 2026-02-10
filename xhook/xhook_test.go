package xhook

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	. "github.com/bytedance/mockey"
	. "github.com/smartystreets/goconvey/convey"
)

func TestGetInvokeFuncFullName(t *testing.T) {
	PatchConvey("TestGetInvokeFuncFullName", t, func() {
		name := getInvokeFuncFullName(MyIntFunc1)
		t.Log(name)
		So(strings.Contains(name, "xhook_test.go"), ShouldBeTrue)
		So(strings.Contains(name, "MyIntFunc1"), ShouldBeTrue)
	})
}

func TestSafeInvokeHook(t *testing.T) {
	PatchConvey("TestSafeInvokeHook", t, func() {
		err := safeInvokeHook(PanicFunc)
		So(err, ShouldNotBeNil)
		So(err.Error(), ShouldEqual, "XOne xhook invokeHook failed, err=[panic occurred, for test]")
	})
}

func MyIntFunc1() error {
	return nil
}

func PanicFunc() error {
	panic("for test")
}

// resetHooks 重置所有 hooks 状态，用于测试
func resetHooks() {
	beforeStartHooks = beforeStartHooks[:0]
	beforeStartHooksSorted = true
	beforeStopHooks = beforeStopHooks[:0]
	beforeStopHooksSorted = true
	registeredFuncs = make(map[uintptr]string)
}

func TestXHookBeforeStart(t *testing.T) {
	PatchConvey("TestXHookBeforeStart-Panic", t, func() {
		resetHooks()
		defer resetHooks()

		var h HookFunc
		So(func() { BeforeStart(h) }, ShouldPanicWith, "XOne BeforeStart hook can not be nil")

		maxHookNum = 1

		BeforeStart(IntFunc1)
		So(func() { BeforeStart(IntFunc2) }, ShouldPanicWith, "XOne BeforeStart hook can not be more than 1")
	})

	PatchConvey("TestXHookBeforeStart-Sort", t, func() {
		resetHooks()
		defer resetHooks()
		maxHookNum = 10

		h1 := func() error { println("h1"); return errors.New("h1") }
		h2 := func() error { println("h2"); return errors.New("h2") }
		h3 := func() error { println("h3"); return errors.New("h3") }
		BeforeStart(h1, Order(1))
		BeforeStart(h3, Order(3))
		BeforeStart(h2, Order(2))
		// 使用 getSortedHooks 获取排序后的副本进行测试
		hooks := getSortedHooks(&beforeStartHooks, &beforeStartHooksSorted)
		for i, h := range hooks {
			err := h.HookFunc()
			So(err.Error(), ShouldEqual, "h"+strconv.Itoa(i+1))
		}
	})
}

func TestXHookBeforeStop(t *testing.T) {
	PatchConvey("TestXHookBeforeStop-Panic", t, func() {
		resetHooks()
		defer resetHooks()

		var h HookFunc
		So(func() { BeforeStop(h) }, ShouldPanicWith, "XOne BeforeStop hook can not be nil")

		maxHookNum = 1

		BeforeStop(IntFunc1)
		So(func() { BeforeStop(IntFunc2) }, ShouldPanicWith, "XOne BeforeStop hook can not be more than 1")
	})

	PatchConvey("TestXHookBeforeStop-Sort", t, func() {
		resetHooks()
		defer resetHooks()
		maxHookNum = 10

		h1 := func() error { println("h1"); return errors.New("h1") }
		h2 := func() error { println("h2"); return errors.New("h2") }
		h3 := func() error { println("h3"); return errors.New("h3") }
		BeforeStop(h1, Order(1))
		BeforeStop(h3, Order(3))
		BeforeStop(h2, Order(2))
		// 使用 getSortedHooks 获取排序后的副本进行测试
		hooks := getSortedHooks(&beforeStopHooks, &beforeStopHooksSorted)
		for i, h := range hooks {
			err := h.HookFunc()
			So(err.Error(), ShouldEqual, "h"+strconv.Itoa(i+1))
		}
	})
}

func TestInvokeBeforeStartHook(t *testing.T) {
	PatchConvey("TestInvokeBeforeStartHook-Err", t, func() {
		resetHooks()
		defer resetHooks()
		f := func() error {
			return errors.New("BeforeStart-Invoke-Err")
		}
		BeforeStart(f)
		err := InvokeBeforeStartHook()
		So(err.Error(), ShouldContainSubstring, "XOne xhook BeforeStart failed")
		So(err.Error(), ShouldContainSubstring, "BeforeStart-Invoke-Err")
	})

	PatchConvey("TestInvokeBeforeStartHook-PanicErr", t, func() {
		resetHooks()
		defer resetHooks()
		f := func() error {
			panic("BeforeStart-Invoke-Panic")
		}
		BeforeStart(f)
		err := InvokeBeforeStartHook()
		So(err.Error(), ShouldContainSubstring, "XOne xhook BeforeStart failed")
		So(err.Error(), ShouldContainSubstring, "panic occurred, BeforeStart-Invoke-Panic")
	})

	PatchConvey("TestInvokeBeforeStartHook-Success", t, func() {
		resetHooks()
		defer resetHooks()
		f1 := func() error {
			return errors.New("BeforeStart-Invoke-Err")
		}
		f2 := func() error {
			panic("BeforeStart-Invoke-Panic")
		}
		f3 := func() error {
			return nil
		}
		BeforeStart(f1, MustInvokeSuccess(false))
		BeforeStart(f2, MustInvokeSuccess(false))
		BeforeStart(f3, MustInvokeSuccess(false))
		err := InvokeBeforeStartHook()
		So(err, ShouldBeNil)
	})
}

func TestInvokeBeforeStopHook(t *testing.T) {
	PatchConvey("TestXHookBeforeStop-Err", t, func() {
		resetHooks()
		defer resetHooks()
		f := func() error {
			return errors.New("BeforeStop-Invoke-Err")
		}
		BeforeStop(f)
		err := InvokeBeforeStopHook()
		So(err.Error(), ShouldContainSubstring, "XOne xhook BeforeStop failed")
		So(err.Error(), ShouldContainSubstring, "BeforeStop-Invoke-Err")
	})

	PatchConvey("TestInvokeBeforeStopHook-PanicErr", t, func() {
		resetHooks()
		defer resetHooks()
		f := func() error {
			panic("BeforeStop-Invoke-Panic")
		}
		BeforeStop(f)
		err := InvokeBeforeStopHook()
		So(err.Error(), ShouldContainSubstring, "XOne xhook BeforeStop failed")
		So(err.Error(), ShouldContainSubstring, "panic occurred, BeforeStop-Invoke-Panic")
	})

	PatchConvey("TestInvokeBeforeStopHook-Success", t, func() {
		resetHooks()
		defer resetHooks()
		f1 := func() error {
			return errors.New("BeforeStop-Invoke-Err")
		}
		f2 := func() error {
			panic("BeforeStop-Invoke-Panic")
		}
		f3 := func() error {
			return nil
		}
		BeforeStop(f1, MustInvokeSuccess(false))
		BeforeStop(f2, MustInvokeSuccess(false))
		BeforeStop(f3, MustInvokeSuccess(false))
		err := InvokeBeforeStopHook()
		So(err.Error(), ShouldContainSubstring, "XOne xhook BeforeStop failed")
		So(err.Error(), ShouldContainSubstring, "BeforeStop-Invoke-Err")
		So(err.Error(), ShouldContainSubstring, "BeforeStop-Invoke-Panic")
	})

	PatchConvey("TestInvokeBeforeStopHook-MergeErr", t, func() {
		resetHooks()
		defer resetHooks()
		f1 := func() error {
			return errors.New("BeforeStop-Invoke-Err")
		}
		f2 := func() error {
			panic("BeforeStop-Invoke-Panic")
		}
		f3 := func() error {
			return nil
		}
		BeforeStop(f1)
		BeforeStop(f2)
		BeforeStop(f3)
		stopErrChan := make(chan error, 1)
		ctx := context.Background()
		hooks := getSortedHooks(&beforeStopHooks, &beforeStopHooksSorted)
		go func() {
			invokeBeforeStopHook(ctx, hooks, stopErrChan)
		}()
		err := <-stopErrChan
		So(err.Error(), ShouldContainSubstring, "BeforeStop-Invoke-Err")
		So(err.Error(), ShouldContainSubstring, "BeforeStop-Invoke-Panic")
	})

	PatchConvey("TestInvokeBeforeStopHook-Timeout", t, func() {
		resetHooks()
		defaultStopTimeout = 1 * time.Second
		defer func() {
			resetHooks()
			defaultStopTimeout = 30 * time.Second
		}()
		f := func() error {
			time.Sleep(2 * time.Second)
			return nil
		}
		BeforeStop(f)
		err := InvokeBeforeStopHook()
		So(err.Error(), ShouldContainSubstring, "XOne xhook BeforeStop failed")
		So(err.Error(), ShouldContainSubstring, "timeout")
	})
}

func TestInvokeBeforeStopHookTimeoutMessage(t *testing.T) {
	PatchConvey("TestInvokeBeforeStopHookTimeoutMessage", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		stopErrChan := make(chan error, 1)
		hooks := []hook{
			{HookFunc: func() error { return nil }, Options: defaultOptions()},
		}
		invokeBeforeStopHook(ctx, hooks, stopErrChan)
		err := <-stopErrChan
		So(err, ShouldNotBeNil)
		So(err.Error(), ShouldContainSubstring, "completed 0/1 hooks")
	})
}

func TestSetStopTimeout(t *testing.T) {
	PatchConvey("TestSetStopTimeout", t, func() {
		originalTimeout := defaultStopTimeout
		defer func() {
			defaultStopTimeout = originalTimeout
		}()

		// 测试设置有效超时时间
		SetStopTimeout(5 * time.Second)
		So(defaultStopTimeout, ShouldEqual, 5*time.Second)

		// 测试设置无效超时时间（<=0 应该被忽略）
		SetStopTimeout(0)
		So(defaultStopTimeout, ShouldEqual, 5*time.Second)

		SetStopTimeout(-1 * time.Second)
		So(defaultStopTimeout, ShouldEqual, 5*time.Second)
	})
}

func TestHookDedup(t *testing.T) {
	PatchConvey("TestHookDedup-重复注册跳过", t, func() {
		resetHooks()
		defer resetHooks()

		f := func() error { return nil }
		BeforeStart(f, Order(1))
		BeforeStart(f, Order(2)) // 重复注册，应跳过
		hooks := getSortedHooks(&beforeStartHooks, &beforeStartHooksSorted)
		So(len(hooks), ShouldEqual, 1)
	})

	PatchConvey("TestHookDedup-不同函数各自注册", t, func() {
		resetHooks()
		defer resetHooks()

		BeforeStart(IntFunc1)
		BeforeStart(IntFunc2)
		hooks := getSortedHooks(&beforeStartHooks, &beforeStartHooksSorted)
		So(len(hooks), ShouldEqual, 2)
	})

	PatchConvey("TestHookDedup-同函数跨类型也检测", t, func() {
		resetHooks()
		defer resetHooks()

		BeforeStart(IntFunc1)
		BeforeStop(IntFunc1) // 同一个函数注册到 BeforeStop，应跳过
		startHooks := getSortedHooks(&beforeStartHooks, &beforeStartHooksSorted)
		stopHooks := getSortedHooks(&beforeStopHooks, &beforeStopHooksSorted)
		So(len(startHooks), ShouldEqual, 1)
		So(len(stopHooks), ShouldEqual, 0)
	})
}

func IntFunc1() error {
	return nil
}

func IntFunc2() error {
	return nil
}

func IntFunc3() error {
	return nil
}

func IntFunc200() error {
	return nil
}

func IntFuncDefault() error {
	return nil
}

func ShortRunStop() error {
	return nil
}

func LongRunStop() error {
	time.Sleep(2 * time.Second)
	return nil
}

func StopErr1() error {
	return errors.New("for test")
}

func StopErr2() error {
	panic("for test 2")
}

func StopSuccess() error {
	return nil
}

func TestHookIndividualTimeout(t *testing.T) {
	PatchConvey("TestHookIndividualTimeout-BeforeStart超时", t, func() {
		resetHooks()
		defer resetHooks()

		slowFunc := func() error {
			time.Sleep(2 * time.Second)
			return nil
		}
		BeforeStart(slowFunc, Timeout(500*time.Millisecond))
		err := InvokeBeforeStartHook()
		So(err, ShouldNotBeNil)
		So(err.Error(), ShouldContainSubstring, "hook timeout after")
	})

	PatchConvey("TestHookIndividualTimeout-BeforeStart正常完成", t, func() {
		resetHooks()
		defer resetHooks()

		fastFunc := func() error {
			return nil
		}
		BeforeStart(fastFunc, Timeout(5*time.Second))
		err := InvokeBeforeStartHook()
		So(err, ShouldBeNil)
	})

	PatchConvey("TestHookIndividualTimeout-BeforeStop个体超时", t, func() {
		resetHooks()
		defaultStopTimeout = 10 * time.Second
		defer func() {
			resetHooks()
			defaultStopTimeout = 30 * time.Second
		}()

		slowStop := func() error {
			time.Sleep(2 * time.Second)
			return nil
		}
		BeforeStop(slowStop, Timeout(500*time.Millisecond))
		err := InvokeBeforeStopHook()
		So(err, ShouldNotBeNil)
		So(err.Error(), ShouldContainSubstring, "hook timeout after")
	})
}

func TestInvokeHookWithTimeout(t *testing.T) {
	PatchConvey("TestInvokeHookWithTimeout-timeout<=0直接执行", t, func() {
		h := hook{
			HookFunc: func() error { return nil },
			Options:  defaultOptions(),
		}
		err := invokeHookWithTimeout(h, 0)
		So(err, ShouldBeNil)
	})
}
