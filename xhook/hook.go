package xhook

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/xiaoshicae/xone/v2/xerror"
	"github.com/xiaoshicae/xone/v2/xutil"

	"golang.org/x/exp/slices"
)

var (
	defaultStopTimeout = 60 * time.Second
	maxHookNum         = 1000
)

var (
	beforeStartHooks       = make([]hook, 0)
	beforeStartHooksSorted = true // 空列表视为已排序
	beforeStopHooks        = make([]hook, 0)
	beforeStopHooksSorted  = true                     // 空列表视为已排序
	registeredFuncs        = make(map[uintptr]string) // 函数指针 -> hookType，用于去重检测
	hooksMu                sync.RWMutex
)

// HookFunc Hook 函数类型定义
type HookFunc func() error

type hook struct {
	HookFunc HookFunc
	Options  *options
}

// SetStopTimeout 设置 BeforeStop hooks 的超时时间
func SetStopTimeout(timeout time.Duration) {
	if timeout > 0 {
		defaultStopTimeout = timeout
	}
}

// BeforeStart 注册 BeforeStart Hook
func BeforeStart(f HookFunc, opts ...Option) {
	registerHook(f, opts, &beforeStartHooks, &beforeStartHooksSorted, "BeforeStart")
}

// BeforeStop 注册 BeforeStop Hook
func BeforeStop(f HookFunc, opts ...Option) {
	registerHook(f, opts, &beforeStopHooks, &beforeStopHooksSorted, "BeforeStop")
}

// registerHook 通用的 hook 注册函数，减少代码重复
func registerHook(f HookFunc, opts []Option, hooks *[]hook, sorted *bool, hookType string) {
	if f == nil {
		panic(fmt.Sprintf("XOne %s hook can not be nil", hookType))
	}

	o := defaultOptions()
	for _, opt := range opts {
		opt(o)
	}

	hooksMu.Lock()
	defer hooksMu.Unlock()

	// 去重检测：通过函数指针判断是否重复注册
	fp := reflect.ValueOf(f).Pointer()
	if existType, ok := registeredFuncs[fp]; ok {
		xutil.WarnIfEnableDebug("XOne %s hook duplicate registration detected, already registered as %s, skipping", hookType, existType)
		return
	}
	registeredFuncs[fp] = hookType

	if len(*hooks) >= maxHookNum {
		panic(fmt.Sprintf("XOne %s hook can not be more than %d", hookType, maxHookNum))
	}

	*hooks = append(*hooks, hook{HookFunc: f, Options: o})
	*sorted = false // 标记需要重新排序
}

// InvokeBeforeStartHook 执行所有 BeforeStart Hook，每个 Hook 独立超时
func InvokeBeforeStartHook() error {
	hooks := getSortedHooks(&beforeStartHooks, &beforeStartHooksSorted)

	for _, h := range hooks {
		funcName := getInvokeFuncFullName(h.HookFunc)
		if err := invokeHookWithTimeout(h, h.Options.Timeout); err != nil {
			if h.Options.MustInvokeSuccess {
				xutil.ErrorIfEnableDebug("XOne invoke before start hook failed, func=[%v], err=[%v]", funcName, err)
				return xerror.Newf("xhook", "BeforeStart", "func=[%v], err=[%v]", funcName, err)
			}
			xutil.WarnIfEnableDebug("XOne invoke before start hook failed, case MustInvokeSuccess=false, before start hook will continue to invoke, func=[%v], err=[%v]", funcName, err)
		} else {
			xutil.InfoIfEnableDebug("XOne invoke before start hook success, func=[%v]", funcName)
		}
	}
	return nil
}

// InvokeBeforeStopHook 执行所有 BeforeStop Hook
func InvokeBeforeStopHook() error {
	hooks := getSortedHooks(&beforeStopHooks, &beforeStopHooksSorted)

	if len(hooks) == 0 {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), defaultStopTimeout)
	defer cancel()

	stopErrChan := make(chan error, 1)

	go func() {
		invokeBeforeStopHook(ctx, hooks, stopErrChan)
	}()

	select {
	case err := <-stopErrChan:
		return err // invokeBeforeStopHook 已返回 xerror
	case <-ctx.Done():
		return xerror.Newf("xhook", "BeforeStop", "timeout after %v", defaultStopTimeout)
	}
}

// getSortedHooks 获取排序后的 hooks 副本，延迟排序优化
func getSortedHooks(hooks *[]hook, sorted *bool) []hook {
	hooksMu.Lock()
	defer hooksMu.Unlock()

	// 仅在未排序时进行排序
	if !*sorted {
		slices.SortStableFunc(*hooks, compareHookOrder)
		*sorted = true
	}

	return slices.Clone(*hooks)
}

func invokeBeforeStopHook(ctx context.Context, hooks []hook, stopResultChan chan<- error) {
	errMsgList := make([]string, 0)
	completed := 0
	for _, h := range hooks {
		// 检查是否已超时，如果超时则提前退出
		select {
		case <-ctx.Done():
			stopResultChan <- xerror.Newf("xhook", "BeforeStop", "interrupted due to timeout, completed %d/%d hooks", completed, len(hooks))
			return
		default:
		}

		// 取 min(个体超时, 全局剩余时间) 作为本次 hook 超时
		hookTimeout := h.Options.Timeout
		if deadline, ok := ctx.Deadline(); ok {
			remaining := time.Until(deadline)
			if remaining < hookTimeout {
				hookTimeout = remaining
			}
		}

		funcName := getInvokeFuncFullName(h.HookFunc)
		if err := invokeHookWithTimeout(h, hookTimeout); err != nil {
			xutil.ErrorIfEnableDebug("XOne invoke before stop hook failed, func=[%v], err=[%v]", funcName, err)
			errMsgList = append(errMsgList, fmt.Sprintf("func=[%v], err=[%v]", funcName, err))
		} else {
			xutil.InfoIfEnableDebug("XOne invoke before stop hook success, func=[%v]", funcName)
		}
		completed++
	}
	if len(errMsgList) > 0 {
		stopResultChan <- xerror.Newf("xhook", "BeforeStop", "%s", strings.Join(errMsgList, "; "))
	} else {
		stopResultChan <- nil
	}
}

// invokeHookWithTimeout 在指定超时内执行单个 Hook
func invokeHookWithTimeout(h hook, timeout time.Duration) error {
	if timeout <= 0 {
		return safeInvokeHook(h.HookFunc)
	}

	ch := make(chan error, 1)
	go func() {
		ch <- safeInvokeHook(h.HookFunc)
	}()

	select {
	case err := <-ch:
		return err
	case <-time.After(timeout):
		funcName := getInvokeFuncFullName(h.HookFunc)
		return xerror.Newf("xhook", "invokeHook", "hook timeout after %v, func=[%v]", timeout, funcName)
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

func compareHookOrder(a, b hook) int {
	if a.Options.Order < b.Options.Order {
		return -1
	}
	if a.Options.Order > b.Options.Order {
		return 1
	}
	return 0
}

func getInvokeFuncFullName(hf HookFunc) string {
	file, line, name := xutil.GetFuncInfo(hf)
	return fmt.Sprintf("%s:%d %s()", file, line, name)
}
