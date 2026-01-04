package xhook

import (
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
		So(err.Error(), ShouldEqual, "panic occurred, for test")
	})
}

func MyIntFunc1() error {
	return nil
}

func PanicFunc() error {
	panic("for test")
	return nil
}

func TestXHookBeforeStart(t *testing.T) {
	PatchConvey("TestXHookBeforeStart-Panic", t, func() {
		beforeStartHooks = beforeStartHooks[:0]
		defer func() {
			beforeStartHooks = beforeStartHooks[:0]
		}()

		var h HookFunc
		So(func() { BeforeStart(h) }, ShouldPanicWith, "XOne BeforeStart hook can not be nil")

		maxHookNum = 1

		h = func() error { return nil }
		BeforeStart(h)
		So(func() { BeforeStart(h) }, ShouldPanicWith, "XOne BeforeStart hook can not be more than 1")
	})

	PatchConvey("TestXHookBeforeStart-Sort", t, func() {
		beforeStartHooks = beforeStartHooks[:0]
		defer func() {
			beforeStartHooks = beforeStartHooks[:0]
		}()
		maxHookNum = 10

		h1 := func() error { println("h1"); return errors.New("h1") }
		h2 := func() error { println("h2"); return errors.New("h2") }
		h3 := func() error { println("h3"); return errors.New("h3") }
		BeforeStart(h1, Order(1))
		BeforeStart(h3, Order(3))
		BeforeStart(h2, Order(2))
		for i, h := range beforeStartHooks {
			err := h.HookFunc()
			So(err.Error(), ShouldEqual, "h"+strconv.Itoa(i+1))
		}
	})
}

func TestXHookBeforeStop(t *testing.T) {
	PatchConvey("TestXHookBeforeStop-Panic", t, func() {
		beforeStopHooks = beforeStopHooks[:0]
		defer func() {
			beforeStopHooks = beforeStopHooks[:0]
		}()

		var h HookFunc
		So(func() { BeforeStop(h) }, ShouldPanicWith, "XOne BeforeStop hook can not be nil")

		maxHookNum = 1

		h = func() error { return nil }
		BeforeStop(h)
		So(func() { BeforeStop(h) }, ShouldPanicWith, "XOne BeforeStop hook can not be more than 1")
	})

	PatchConvey("TestXHookBeforeStop-Sort", t, func() {
		beforeStopHooks = beforeStopHooks[:0]
		defer func() {
			beforeStopHooks = beforeStopHooks[:0]
		}()
		maxHookNum = 10

		h1 := func() error { println("h1"); return errors.New("h1") }
		h2 := func() error { println("h2"); return errors.New("h2") }
		h3 := func() error { println("h3"); return errors.New("h3") }
		BeforeStop(h1, Order(1))
		BeforeStop(h3, Order(3))
		BeforeStop(h2, Order(2))
		for i, h := range beforeStopHooks {
			err := h.HookFunc()
			So(err.Error(), ShouldEqual, "h"+strconv.Itoa(i+1))
		}
	})
}

func TestInvokeBeforeStartHook(t *testing.T) {
	PatchConvey("TestInvokeBeforeStartHook-Err", t, func() {
		beforeStartHooks = beforeStartHooks[:0]
		defer func() {
			beforeStartHooks = beforeStartHooks[:0]
		}()
		f := func() error {
			return errors.New("BeforeStart-Invoke-Err")
		}
		BeforeStart(f)
		err := InvokeBeforeStartHook()
		So(err.Error(), ShouldContainSubstring, "XOne invoke before start hook failed")
		So(err.Error(), ShouldContainSubstring, "BeforeStart-Invoke-Err")
	})

	PatchConvey("TestInvokeBeforeStartHook-PanicErr", t, func() {
		beforeStartHooks = beforeStartHooks[:0]
		defer func() {
			beforeStartHooks = beforeStartHooks[:0]
		}()
		f := func() error {
			panic("BeforeStart-Invoke-Panic")
		}
		BeforeStart(f)
		err := InvokeBeforeStartHook()
		So(err.Error(), ShouldContainSubstring, "XOne invoke before start hook failed")
		So(err.Error(), ShouldContainSubstring, "panic occurred, BeforeStart-Invoke-Panic")
	})

	PatchConvey("TestInvokeBeforeStartHook-Success", t, func() {
		beforeStartHooks = beforeStartHooks[:0]
		defer func() {
			beforeStartHooks = beforeStartHooks[:0]
		}()
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
		beforeStopHooks = beforeStopHooks[:0]
		defer func() {
			beforeStopHooks = beforeStopHooks[:0]
		}()
		f := func() error {
			return errors.New("BeforeStop-Invoke-Err")
		}
		BeforeStop(f)
		err := InvokeBeforeStopHook()
		So(err.Error(), ShouldContainSubstring, "XOne invoke before stop hook failed")
		So(err.Error(), ShouldContainSubstring, "BeforeStop-Invoke-Err")
	})

	PatchConvey("TestInvokeBeforeStopHook-PanicErr", t, func() {
		beforeStopHooks = beforeStopHooks[:0]
		defer func() {
			beforeStopHooks = beforeStopHooks[:0]
		}()
		f := func() error {
			panic("BeforeStop-Invoke-Panic")
		}
		BeforeStop(f)
		err := InvokeBeforeStopHook()
		So(err.Error(), ShouldContainSubstring, "XOne invoke before stop hook failed")
		So(err.Error(), ShouldContainSubstring, "panic occurred, BeforeStop-Invoke-Panic")
	})

	PatchConvey("TestInvokeBeforeStopHook-Success", t, func() {
		beforeStopHooks = beforeStopHooks[:0]
		defer func() {
			beforeStopHooks = beforeStopHooks[:0]
		}()
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
		So(err.Error(), ShouldContainSubstring, "XOne invoke before stop hook failed")
		So(err.Error(), ShouldContainSubstring, "BeforeStop-Invoke-Err")
		So(err.Error(), ShouldContainSubstring, "BeforeStop-Invoke-Panic")
	})

	PatchConvey("TestInvokeBeforeStopHook-MergeErr", t, func() {
		beforeStopHooks = beforeStopHooks[:0]
		defer func() {
			beforeStopHooks = beforeStopHooks[:0]
		}()
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
		go func() {
			invokeBeforeStopHook(beforeStopHooks, stopErrChan)
		}()
		err := <-stopErrChan
		So(err.Error(), ShouldContainSubstring, "BeforeStop-Invoke-Err")
		So(err.Error(), ShouldContainSubstring, "BeforeStop-Invoke-Panic")
	})

	PatchConvey("TestInvokeBeforeStopHook-Timeout", t, func() {
		beforeStopHooks = beforeStopHooks[:0]
		defaultStopTimeout = 1 * time.Second
		defer func() {
			beforeStopHooks = beforeStopHooks[:0]
			defaultStopTimeout = 30 * time.Second
		}()
		f := func() error {
			time.Sleep(2 * time.Second)
			return nil
		}
		BeforeStop(f)
		err := InvokeBeforeStopHook()
		So(err.Error(), ShouldEqual, "XOne invoke before stop hook failed, due to timeout")
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
