package xhook

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/xiaoshicae/xone/xutil"

	"golang.org/x/exp/slices"
)

var defaultStopTimeout = 30 * time.Second
var maxHookNum = 1000

var beforeStartHooks = make([]hook, 0)
var beforeStopHooks = make([]hook, 0)
var hooksMu sync.RWMutex

type HookFunc func() error

type hook struct {
	HookFunc HookFunc
	Options  *options
}

func BeforeStart(f HookFunc, opts ...Option) {
	if f == nil {
		panic("XOne BeforeStart hook can not be nil")
	}

	o := defaultOptions()
	for _, opt := range opts {
		opt(o)
	}

	hooksMu.Lock()
	defer hooksMu.Unlock()
	if len(beforeStartHooks) >= maxHookNum {
		panic(fmt.Sprintf("XOne BeforeStart hook can not be more than %d", maxHookNum))
	}

	beforeStartHooks = append(beforeStartHooks, hook{HookFunc: f, Options: o})

	slices.SortStableFunc(beforeStartHooks, func(a, b hook) int {
		return compareHookOrder(a, b)
	})
}

func BeforeStop(f HookFunc, opts ...Option) {
	if f == nil {
		panic("XOne BeforeStop hook can not be nil")
	}

	o := defaultOptions()
	for _, opt := range opts {
		opt(o)
	}

	hooksMu.Lock()
	defer hooksMu.Unlock()
	if len(beforeStopHooks) >= maxHookNum {
		panic(fmt.Sprintf("XOne BeforeStop hook can not be more than %d", maxHookNum))
	}

	beforeStopHooks = append(beforeStopHooks, hook{HookFunc: f, Options: o})

	slices.SortStableFunc(beforeStopHooks, func(a, b hook) int {
		return compareHookOrder(a, b)
	})
}

func InvokeBeforeStartHook() error {
	hooksMu.RLock()
	hooks := append([]hook(nil), beforeStartHooks...)
	hooksMu.RUnlock()

	for _, h := range hooks {
		if err := safeInvokeHook(h.HookFunc); err != nil {
			if h.Options.MustInvokeSuccess {
				xutil.ErrorIfEnableDebug("XOne invoke before start hook failed, func=[%v], err=[%v]", getInvokeFuncFullName(h.HookFunc), err)
				return fmt.Errorf("XOne invoke before start hook failed, func=[%v], err=[%v]", getInvokeFuncFullName(h.HookFunc), err)
			}
			xutil.WarnIfEnableDebug("XOne invoke before start hook failed, case MustInvokeSuccess=false, before start hook will continue to invoke, func=[%v], err=[%v]", getInvokeFuncFullName(h.HookFunc), err)
		} else {
			xutil.InfoIfEnableDebug("XOne invoke before start hook success, func=[%v]", xutil.GetFuncName(h.HookFunc))
		}
	}
	return nil
}

func InvokeBeforeStopHook() error {
	hooksMu.RLock()
	hooks := append([]hook(nil), beforeStopHooks...)
	hooksMu.RUnlock()

	if len(hooks) == 0 {
		return nil
	}

	stopErrChan := make(chan error, 1)
	stopTimeout := defaultStopTimeout

	go func() {
		invokeBeforeStopHook(hooks, stopErrChan)
	}()

	select {
	case err := <-stopErrChan:
		if err != nil {
			return fmt.Errorf("XOne invoke before stop hook failed, %v", err)
		}
		return nil
	case <-time.After(stopTimeout):
		return errors.New("XOne invoke before stop hook failed, due to timeout")
	}
}

func invokeBeforeStopHook(hooks []hook, stopResultChan chan<- error) {
	errMsgList := make([]string, 0)
	for _, h := range hooks {
		if err := safeInvokeHook(h.HookFunc); err != nil {
			xutil.ErrorIfEnableDebug("XOne invoke before stop hook failed, func=[%v], err=[%v]", getInvokeFuncFullName(h.HookFunc), err)
			errMsgList = append(errMsgList, fmt.Sprintf("func=[%v], err=[%v]", getInvokeFuncFullName(h.HookFunc), err))
		} else {
			xutil.InfoIfEnableDebug("XOne invoke before stop hook success, func=[%v]", xutil.GetFuncName(h.HookFunc))
		}
	}
	if len(errMsgList) > 0 {
		stopResultChan <- fmt.Errorf("%s", strings.Join(errMsgList, "; "))
	} else {
		stopResultChan <- nil
	}
}

func safeInvokeHook(h HookFunc) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic occurred, %v", r)
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
