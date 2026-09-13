package xhook

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/xiaoshicae/xone/v2/xerror"
	"github.com/xiaoshicae/xone/v2/xutil"
)

const (
	// maxHookNum 单个阶段可注册的 Hook 数量上限
	maxHookNum = 1000

	// defaultStopTimeout BeforeStop 全部 Hook 的总超时时间
	defaultStopTimeout = 60 * time.Second
)

// stopTimeout BeforeStop 的总超时，可由 SetStopTimeout 调整
var stopTimeout = func() *atomic.Int64 {
	v := &atomic.Int64{}
	v.Store(int64(defaultStopTimeout))
	return v
}()

var (
	startRegistry = newRegistry("BeforeStart")
	stopRegistry  = newRegistry("BeforeStop")
)

// HookFunc Hook 函数类型定义
type HookFunc func() error

type hook struct {
	HookFunc HookFunc
	Options  *options
}

// registry 单个阶段的 Hook 注册表
//
// 排序延迟到实际执行时进行：注册阶段只标记失效，避免每次注册都重排。
type registry struct {
	name    string // 阶段名称，用于日志与错误信息
	maxHook int    // 可注册的 Hook 数量上限

	mu     sync.Mutex
	hooks  []hook
	sorted bool // 空列表视为已排序
}

func newRegistry(name string) *registry {
	return &registry{name: name, maxHook: maxHookNum, sorted: true}
}

// SetStopTimeout 设置 BeforeStop hooks 的总超时时间（线程安全）
func SetStopTimeout(timeout time.Duration) {
	if timeout > 0 {
		stopTimeout.Store(int64(timeout))
	}
}

// BeforeStart 注册 BeforeStart Hook
func BeforeStart(f HookFunc, opts ...Option) {
	startRegistry.add(f, opts)
}

// BeforeStop 注册 BeforeStop Hook
func BeforeStop(f HookFunc, opts ...Option) {
	stopRegistry.add(f, opts)
}

// add 注册一个 Hook
//
// 不做去重：Hook 多为闭包，而同一函数字面量产生的所有闭包共享同一代码指针，
// 按指针去重会把循环或工厂中注册的不同闭包误判为重复而静默丢弃。
// 重复注册同一函数由调用方自行保证。
func (r *registry) add(f HookFunc, opts []Option) {
	if f == nil {
		panic(fmt.Sprintf("XOne %s hook can not be nil", r.name))
	}

	o := defaultOptions()
	for _, opt := range opts {
		opt(o)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if len(r.hooks) >= r.maxHook {
		panic(fmt.Sprintf("XOne %s hook can not be more than %d", r.name, r.maxHook))
	}

	r.hooks = append(r.hooks, hook{HookFunc: f, Options: o})
	r.sorted = false
}

// sortedHooks 返回按 Order 升序排列的 Hook 副本
func (r *registry) sortedHooks() []hook {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.sorted {
		slices.SortStableFunc(r.hooks, compareHookOrder)
		r.sorted = true
	}
	return slices.Clone(r.hooks)
}

// reversedSortedHooks 返回 BeforeStop 的执行顺序，即 BeforeStart 顺序的镜像
//
// 先按 Order 稳定升序排（与 BeforeStart 同一份顺序），再整体反转：
// Order 大的先关闭，同 Order 内按注册顺序逆序关闭（LIFO）。
//
// Order 表示资源层级而非阶段内优先级：值越小越底层，启动越早、关闭越晚。
// 这样一个资源只需声明一个 Order，启停自然对称；若两个阶段都按升序执行，
// 「早启动且晚关闭」就得在两个阶段各填一个方向相反的值。
func (r *registry) reversedSortedHooks() []hook {
	hooks := r.sortedHooks()
	slices.Reverse(hooks)
	return hooks
}

// InvokeBeforeStartHook 执行所有 BeforeStart Hook，每个 Hook 独立超时
func InvokeBeforeStartHook() error {
	for _, h := range startRegistry.sortedHooks() {
		if err := invokeHookWithTimeout(h, h.Options.Timeout); err != nil {
			funcName := getInvokeFuncFullName(h.HookFunc)
			if h.Options.MustInvokeSuccess {
				xutil.ErrorIfEnableDebug("XOne invoke before start hook failed, func=[%v], err=[%v]", funcName, err)
				return xerror.Newf("xhook", "BeforeStart", "func=[%v], err=[%v]", funcName, err)
			}
			xutil.WarnIfEnableDebug("XOne invoke before start hook failed, case MustInvokeSuccess=false, before start hook will continue to invoke, func=[%v], err=[%v]", funcName, err)
			continue
		}
		xutil.InfoIfEnableDebug("XOne invoke before start hook success, func=[%v]", getInvokeFuncFullName(h.HookFunc))
	}
	return nil
}

// InvokeBeforeStopHook 执行所有 BeforeStop Hook
// 同 Order 的 Hook 按注册顺序逆序执行，确保与 BeforeStart 对称
func InvokeBeforeStopHook() error {
	hooks := stopRegistry.reversedSortedHooks()
	if len(hooks) == 0 {
		return nil
	}

	timeout := time.Duration(stopTimeout.Load())
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	stopErrChan := make(chan error, 1)
	go invokeBeforeStopHook(ctx, hooks, stopErrChan)

	select {
	case err := <-stopErrChan:
		return err // invokeBeforeStopHook 已返回 xerror
	case <-ctx.Done():
		return xerror.Newf("xhook", "BeforeStop", "timeout after %v", timeout)
	}
}

func invokeBeforeStopHook(ctx context.Context, hooks []hook, stopResultChan chan<- error) {
	errMsgList := make([]string, 0)
	for i, h := range hooks {
		// 已取消或已超时则提前退出
		select {
		case <-ctx.Done():
			stopResultChan <- xerror.Newf("xhook", "BeforeStop", "interrupted due to timeout, completed %d/%d hooks", i, len(hooks))
			return
		default:
		}

		// 取 min(个体超时, 全局剩余时间) 作为本次 hook 超时
		// 剩余时间耗尽时同样中断：把非正数传给 invokeHookWithTimeout
		// 会被当作「不设超时」而同步执行，卡住的 Hook 将永久阻塞该协程。
		// 上面的 Done 检查无法覆盖「检查通过后、调用前恰好超时」这一窗口
		hookTimeout := h.Options.Timeout
		if deadline, ok := ctx.Deadline(); ok {
			remaining := time.Until(deadline)
			if remaining <= 0 {
				stopResultChan <- xerror.Newf("xhook", "BeforeStop", "interrupted due to timeout, completed %d/%d hooks", i, len(hooks))
				return
			}
			hookTimeout = min(hookTimeout, remaining)
		}

		if err := invokeHookWithTimeout(h, hookTimeout); err != nil {
			funcName := getInvokeFuncFullName(h.HookFunc)
			xutil.ErrorIfEnableDebug("XOne invoke before stop hook failed, func=[%v], err=[%v]", funcName, err)
			errMsgList = append(errMsgList, fmt.Sprintf("func=[%v], err=[%v]", funcName, err))
			continue
		}
		xutil.InfoIfEnableDebug("XOne invoke before stop hook success, func=[%v]", getInvokeFuncFullName(h.HookFunc))
	}

	if len(errMsgList) > 0 {
		stopResultChan <- xerror.Newf("xhook", "BeforeStop", "%s", strings.Join(errMsgList, "; "))
		return
	}
	stopResultChan <- nil
}

// invokeHookWithTimeout 在指定超时内执行单个 Hook
// 注意：超时仅代表"放弃等待"，并不会取消正在运行的 Hook 函数。
// 如果 Hook 函数长时间阻塞（如死锁），其 goroutine 将持续存在直到函数返回。
// Hook 实现者应确保函数能在合理时间内返回。
func invokeHookWithTimeout(h hook, timeout time.Duration) error {
	if timeout <= 0 {
		return safeInvokeHook(h.HookFunc)
	}

	ch := make(chan error, 1)
	go func() {
		ch <- safeInvokeHook(h.HookFunc)
	}()

	// Go 1.23 起 time.After 的定时器也能被 GC 回收，这里用 Timer 只是为了
	// 在 Hook 提前返回时立刻释放定时器，而不必等它自然到期
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case err := <-ch:
		return err
	case <-timer.C:
		return xerror.Newf("xhook", "invokeHook", "hook timeout after %v, func=[%v]", timeout, getInvokeFuncFullName(h.HookFunc))
	}
}

func safeInvokeHook(h HookFunc) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = xerror.Newf("xhook", "invokeHook", "panic occurred, %v", r)
		}
	}()
	return h()
}

// compareHookOrder 按 Order 升序比较
// 用 cmp.Compare 而非相减，避免极端 Order 值下的整数溢出
func compareHookOrder(a, b hook) int {
	return cmp.Compare(a.Options.Order, b.Options.Order)
}

func getInvokeFuncFullName(hf HookFunc) string {
	file, line, name := xutil.GetFuncInfo(hf)
	return fmt.Sprintf("%s:%d %s()", file, line, name)
}

// reset 清空注册表并恢复默认上限，仅供测试使用
func (r *registry) reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.hooks = r.hooks[:0]
	r.sorted = true
	r.maxHook = maxHookNum
}
