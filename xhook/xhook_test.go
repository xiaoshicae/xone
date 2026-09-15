package xhook

import (
	"context"
	"errors"
	"math"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/xiaoshicae/xone/v3/internal/hookorder"

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
	startRegistry.reset()
	stopRegistry.reset()
	SetStopTimeout(defaultStopTimeout)
}

func TestXHookBeforeStart(t *testing.T) {
	PatchConvey("TestXHookBeforeStart-Panic", t, func() {
		resetHooks()
		defer resetHooks()

		var h HookFunc
		So(func() { BeforeStart(h) }, ShouldPanicWith, "XOne BeforeStart hook can not be nil")

		startRegistry.maxHook = 1

		BeforeStart(IntFunc1)
		So(func() { BeforeStart(IntFunc2) }, ShouldPanicWith, "XOne BeforeStart hook can not be more than 1")
	})

	PatchConvey("TestXHookBeforeStart-Sort", t, func() {
		resetHooks()
		defer resetHooks()

		h1 := func() error { println("h1"); return errors.New("h1") }
		h2 := func() error { println("h2"); return errors.New("h2") }
		h3 := func() error { println("h3"); return errors.New("h3") }
		BeforeStart(h1, Order(1))
		BeforeStart(h3, Order(3))
		BeforeStart(h2, Order(2))
		// 使用 getSortedHooks 获取排序后的副本进行测试
		hooks := startRegistry.sortedHooks()
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

		stopRegistry.maxHook = 1

		BeforeStop(IntFunc1)
		So(func() { BeforeStop(IntFunc2) }, ShouldPanicWith, "XOne BeforeStop hook can not be more than 1")
	})

	PatchConvey("TestXHookBeforeStop-Sort", t, func() {
		resetHooks()
		defer resetHooks()

		h1 := func() error { println("h1"); return errors.New("h1") }
		h2 := func() error { println("h2"); return errors.New("h2") }
		h3 := func() error { println("h3"); return errors.New("h3") }
		BeforeStop(h1, Order(1))
		BeforeStop(h3, Order(3))
		BeforeStop(h2, Order(2))
		// getSortedHooks 仍按 Order 升序排列（底层排序逻辑不变）
		hooks := stopRegistry.sortedHooks()
		for i, h := range hooks {
			err := h.HookFunc()
			So(err.Error(), ShouldEqual, "h"+strconv.Itoa(i+1))
		}
	})

	PatchConvey("TestXHookBeforeStop-ReverseExecution", t, func() {
		resetHooks()
		defer resetHooks()

		// 模拟模块按 init 顺序依次注册 BeforeStop（默认 Order=100）
		// 期望执行时反序：h3 → h2 → h1
		order := make([]string, 0, 3)
		h1 := func() error { order = append(order, "h1"); return nil }
		h2 := func() error { order = append(order, "h2"); return nil }
		h3 := func() error { order = append(order, "h3"); return nil }
		BeforeStop(h1)
		BeforeStop(h2)
		BeforeStop(h3)

		err := InvokeBeforeStopHook()
		So(err, ShouldBeNil)
		So(order, ShouldResemble, []string{"h3", "h2", "h1"})
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
		BeforeStart(f1, MustSucceed(false))
		BeforeStart(f2, MustSucceed(false))
		BeforeStart(f3, MustSucceed(false))
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
		BeforeStop(f1, MustSucceed(false))
		BeforeStop(f2, MustSucceed(false))
		BeforeStop(f3, MustSucceed(false))
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
		hooks := stopRegistry.sortedHooks()
		go func() {
			invokeBeforeStopHook(ctx, hooks, stopErrChan)
		}()
		err := <-stopErrChan
		So(err.Error(), ShouldContainSubstring, "BeforeStop-Invoke-Err")
		So(err.Error(), ShouldContainSubstring, "BeforeStop-Invoke-Panic")
	})

	PatchConvey("TestInvokeBeforeStopHook-Timeout", t, func() {
		resetHooks()
		SetStopTimeout(1 * time.Second)
		defer func() {
			resetHooks()
			SetStopTimeout(60 * time.Second)
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
		originalTimeout := time.Duration(stopTimeout.Load())
		defer func() {
			SetStopTimeout(originalTimeout)
		}()

		// 测试设置有效超时时间
		SetStopTimeout(5 * time.Second)
		So(time.Duration(stopTimeout.Load()), ShouldEqual, 5*time.Second)

		// 测试设置无效超时时间（<=0 应该被忽略）
		SetStopTimeout(0)
		So(time.Duration(stopTimeout.Load()), ShouldEqual, 5*time.Second)

		SetStopTimeout(-1 * time.Second)
		So(time.Duration(stopTimeout.Load()), ShouldEqual, 5*time.Second)
	})
}

func TestHookRegistration(t *testing.T) {
	PatchConvey("TestHookRegistration-同一函数可重复注册", t, func() {
		resetHooks()
		defer resetHooks()

		// 不再按函数指针去重：重复注册由调用方自行保证
		f := func() error { return nil }
		BeforeStart(f, Order(1))
		BeforeStart(f, Order(2))
		So(len(startRegistry.sortedHooks()), ShouldEqual, 2)
	})

	PatchConvey("TestHookRegistration-循环中注册的闭包不会丢失", t, func() {
		// 回归：同一函数字面量产生的闭包共享代码指针，
		// 曾因此被按指针去重误判为重复而静默丢弃
		resetHooks()
		defer resetHooks()

		var got []int
		for i := 0; i < 3; i++ {
			n := i
			BeforeStart(func() error {
				got = append(got, n)
				return nil
			})
		}
		So(len(startRegistry.sortedHooks()), ShouldEqual, 3)
		So(InvokeBeforeStartHook(), ShouldBeNil)
		So(got, ShouldResemble, []int{0, 1, 2})
	})

	PatchConvey("TestHookRegistration-不同函数各自注册", t, func() {
		resetHooks()
		defer resetHooks()

		BeforeStart(IntFunc1)
		BeforeStart(IntFunc2)
		So(len(startRegistry.sortedHooks()), ShouldEqual, 2)
	})

	PatchConvey("TestHookRegistration-两个阶段互相独立", t, func() {
		resetHooks()
		defer resetHooks()

		BeforeStart(IntFunc1)
		BeforeStop(IntFunc1)
		So(len(startRegistry.sortedHooks()), ShouldEqual, 1)
		So(len(stopRegistry.sortedHooks()), ShouldEqual, 1)
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
		SetStopTimeout(10 * time.Second)
		defer func() {
			resetHooks()
			SetStopTimeout(60 * time.Second)
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

// TestBeforeStopOrderSemantics 回归防护：
// Order 表示资源层级（值越小越底层），启停必须对称——
// BeforeStart 按 Order 升序，BeforeStop 按 Order 降序，
// 使一个资源只需声明一个 Order 就能做到"先启动、后关闭"。
// TestOrderReservedBand 回归防护：
// 负值区为框架保留，业务 Hook 无法进入，
// 因此不可能先于 xconfig 启动，也不可能晚于 xlog 关闭。
func TestOrderReservedBand(t *testing.T) {
	PatchConvey("TestOrderReservedBand", t, func() {
		PatchConvey("负 Order 直接 panic", func() {
			So(func() { Order(-1) }, ShouldPanicWith,
				"XOne hook order can not be less than 0, negative order is reserved for the framework")
			So(func() { Order(math.MinInt) }, ShouldPanicWith,
				"XOne hook order can not be less than 0, negative order is reserved for the framework")
		})

		PatchConvey("0 与正数正常接受", func() {
			So(func() { Order(0) }, ShouldNotPanic)
			So(func() { Order(math.MaxInt) }, ShouldNotPanic)
		})

		PatchConvey("保留层级始终先启动、后关闭", func() {
			resetHooks()
			defer resetHooks()

			tk := hookorder.Token{}
			var startSeq, stopSeq []string
			// 业务 Hook 取业务区最小值，仍排在保留层级之后
			BeforeStart(func() error { startSeq = append(startSeq, "业务"); return nil }, Order(0))
			BeforeStop(func() error { stopSeq = append(stopSeq, "业务"); return nil }, Order(0))
			BeforeStart(func() error { startSeq = append(startSeq, "日志"); return nil }, ReservedOrder(tk, hookorder.Log))
			BeforeStop(func() error { stopSeq = append(stopSeq, "日志"); return nil }, ReservedOrder(tk, hookorder.Log))
			BeforeStart(func() error { startSeq = append(startSeq, "配置"); return nil }, ReservedOrder(tk, hookorder.Config))

			So(InvokeBeforeStartHook(), ShouldBeNil)
			So(InvokeBeforeStopHook(), ShouldBeNil)
			So(startSeq, ShouldResemble, []string{"配置", "日志", "业务"})
			So(stopSeq, ShouldResemble, []string{"业务", "日志"})
		})
	})
}

func TestBeforeStopOrderSemantics(t *testing.T) {
	PatchConvey("TestBeforeStopOrderSemantics", t, func() {
		PatchConvey("Order 小的后关闭", func() {
			resetHooks()
			defer resetHooks()

			var seq []string
			BeforeStop(func() error { seq = append(seq, "日志"); return nil }, Order(10))
			BeforeStop(func() error { seq = append(seq, "普通A"); return nil })
			BeforeStop(func() error { seq = append(seq, "普通B"); return nil })

			So(InvokeBeforeStopHook(), ShouldBeNil)
			// 低 Order 的日志模块最后关闭，同 Order 的普通模块按注册逆序
			So(seq, ShouldResemble, []string{"普通B", "普通A", "日志"})
		})

		PatchConvey("同 Order 内按注册顺序逆序执行", func() {
			resetHooks()
			defer resetHooks()

			var seq []string
			for _, name := range []string{"第一", "第二", "第三"} {
				n := name
				BeforeStop(func() error { seq = append(seq, n); return nil })
			}

			So(InvokeBeforeStopHook(), ShouldBeNil)
			So(seq, ShouldResemble, []string{"第三", "第二", "第一"})
		})

		PatchConvey("启停对称：BeforeStop 是 BeforeStart 的镜像", func() {
			resetHooks()
			defer resetHooks()

			var startSeq, stopSeq []string
			BeforeStart(func() error { startSeq = append(startSeq, "低"); return nil }, Order(1))
			BeforeStart(func() error { startSeq = append(startSeq, "高"); return nil }, Order(9999))
			BeforeStop(func() error { stopSeq = append(stopSeq, "低"); return nil }, Order(1))
			BeforeStop(func() error { stopSeq = append(stopSeq, "高"); return nil }, Order(9999))

			So(InvokeBeforeStartHook(), ShouldBeNil)
			So(InvokeBeforeStopHook(), ShouldBeNil)
			So(startSeq, ShouldResemble, []string{"低", "高"})
			So(stopSeq, ShouldResemble, []string{"高", "低"})
		})

		PatchConvey("全部默认 Order 时等价于注册顺序与其逆序", func() {
			resetHooks()
			defer resetHooks()

			// 模拟 import 顺序：xconfig → xlog → xhttp → xgorm
			modules := []string{"xconfig", "xlog", "xhttp", "xgorm"}
			var startSeq, stopSeq []string
			for _, name := range modules {
				n := name
				BeforeStart(func() error { startSeq = append(startSeq, n); return nil })
				BeforeStop(func() error { stopSeq = append(stopSeq, n); return nil })
			}

			So(InvokeBeforeStartHook(), ShouldBeNil)
			So(InvokeBeforeStopHook(), ShouldBeNil)
			So(startSeq, ShouldResemble, modules)
			So(stopSeq, ShouldResemble, []string{"xgorm", "xhttp", "xlog", "xconfig"})
		})
	})
}

// TestBeforeStopExhaustedDeadline 回归防护：
// 全局剩余时间耗尽时，min(个体超时, 剩余) 会得到非正数，
// 而 invokeHookWithTimeout 对非正数的处理是「不设超时同步执行」，
// 卡住的 Hook 会永久阻塞该协程。
func TestBeforeStopExhaustedDeadline(t *testing.T) {
	PatchConvey("TestBeforeStopExhaustedDeadline", t, func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
		defer cancel()
		time.Sleep(10 * time.Millisecond) // 确保 deadline 已过

		blocked := make(chan struct{})
		defer close(blocked)
		hooks := []hook{{
			HookFunc: func() error { <-blocked; return nil },
			Options:  defaultOptions(),
		}}

		stopErrChan := make(chan error, 1)
		done := make(chan struct{})
		go func() {
			invokeBeforeStopHook(ctx, hooks, stopErrChan)
			close(done)
		}()

		select {
		case <-done:
			err := <-stopErrChan
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "interrupted due to timeout")
		case <-time.After(2 * time.Second):
			t.Fatal("剩余时间耗尽时未中断，协程被卡住的 Hook 永久阻塞")
		}
	})
}

func TestCompareHookOrder(t *testing.T) {
	PatchConvey("TestCompareHookOrder", t, func() {
		mk := func(order int) hook { return hook{Options: &options{Order: order}} }
		So(compareHookOrder(mk(1), mk(2)), ShouldBeLessThan, 0)
		So(compareHookOrder(mk(2), mk(1)), ShouldBeGreaterThan, 0)
		So(compareHookOrder(mk(1), mk(1)), ShouldEqual, 0)
		// 极端值不能因相减溢出而得到错误符号
		So(compareHookOrder(mk(math.MinInt), mk(math.MaxInt)), ShouldBeLessThan, 0)
		So(compareHookOrder(mk(math.MaxInt), mk(math.MinInt)), ShouldBeGreaterThan, 0)
	})
}

func TestInvokeBeforeStopHookEmpty(t *testing.T) {
	PatchConvey("TestInvokeBeforeStopHookEmpty", t, func() {
		resetHooks()
		defer resetHooks()
		// 无 Hook 时直接返回，不应创建 context 与协程
		So(InvokeBeforeStopHook(), ShouldBeNil)
	})
}

func TestBeforeStopDeadlineRaceWindow(t *testing.T) {
	PatchConvey("TestBeforeStopDeadlineRaceWindow", t, func() {
		// ctx.Done() 检查通过后、调用 Hook 前恰好超时，是无法稳定复现的竞态窗口；
		// 此处直接让剩余时间返回非正数来覆盖该兜底分支。
		// 若缺少这一兜底，非正数会被 invokeHookWithTimeout 当作「不设超时」，
		// 卡住的 Hook 将永久阻塞该协程。
		// 必须先建 ctx 再装 mock：context.WithTimeout 内部也调用 time.Until，
		// 先装 mock 会让它误判 deadline 已过而直接返回已取消的 ctx，
		// 测试就会走 ctx.Done() 分支、以错误的原因通过
		ctx, cancel := context.WithTimeout(context.Background(), time.Hour)
		defer cancel()

		Mock(time.Until).Return(-time.Second).Build()

		invoked := false
		hooks := []hook{{
			HookFunc: func() error { invoked = true; return nil },
			Options:  defaultOptions(),
		}}

		stopErrChan := make(chan error, 1)
		invokeBeforeStopHook(ctx, hooks, stopErrChan)

		err := <-stopErrChan
		So(err, ShouldNotBeNil)
		So(err.Error(), ShouldContainSubstring, "interrupted due to timeout")
		So(invoked, ShouldBeFalse)
	})
}

// TestBeforeStartInvoked 初始化是否完成必须可查询
//
// xconfig 未初始化时读配置不报错、只返回零值，所以"忘了跑 BeforeStart"
// 不会自己暴露出来，需要一个显式入口供依赖初始化结果的模块自检
func TestBeforeStartInvoked(t *testing.T) {
	PatchConvey("TestBeforeStartInvoked", t, func() {
		old := beforeStartInvoked.Load()
		defer beforeStartInvoked.Store(old)

		PatchConvey("未执行时为 false", func() {
			beforeStartInvoked.Store(false)
			So(BeforeStartInvoked(), ShouldBeFalse)
		})

		PatchConvey("执行成功后为 true", func() {
			beforeStartInvoked.Store(false)
			So(InvokeBeforeStartHook(), ShouldBeNil)
			So(BeforeStartInvoked(), ShouldBeTrue)
		})

		PatchConvey("MustSucceed 的 Hook 失败时保持 false", func() {
			beforeStartInvoked.Store(false)
			Mock(invokeHookWithTimeout).Return(errors.New("init failed")).Build()
			Mock((*registry).sortedHooks).Return([]hook{
				{HookFunc: func() error { return nil }, Options: defaultOptions()},
			}).Build()

			So(InvokeBeforeStartHook(), ShouldNotBeNil)
			So(BeforeStartInvoked(), ShouldBeFalse)
		})
	})
}
