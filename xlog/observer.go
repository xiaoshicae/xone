package xlog

import (
	"context"

	"github.com/xiaoshicae/xone/v2/xutil"
	"sync"
	"sync/atomic"
)

// Record 一条日志的只读描述，供观察者旁路消费
//
// 只暴露与日志内容无关的元信息，不依赖任何具体日志库类型，
// 便于后续替换日志后端时保持扩展点稳定。
type Record struct {
	Level   Level
	Message string
	File    string // 文件名（不含路径），如 main.go
	Line    int
	TraceID string
	SpanID  string
}

// Observer 日志观察者，在日志写出时被同步调用
//
// 用于 metric 上报、告警等旁路能力，不影响日志本身的输出。
// 实现方需注意：
//   - 必须快速返回，耗时操作应自行异步化，否则会拖慢日志写入
//   - 不得在其中调用 xlog 的日志函数，否则会无限递归
type Observer func(ctx context.Context, r Record)

var (
	// observers 采用读时无锁的写时复制，日志热路径只做一次原子读
	observers atomic.Pointer[[]Observer]

	// observerMu 仅保护注册过程
	observerMu sync.Mutex
)

// AddObserver 注册日志观察者，注册后对所有级别的日志生效
// 观察者需自行按 Record.Level 过滤关心的级别
func AddObserver(o Observer) {
	if o == nil {
		return
	}

	observerMu.Lock()
	defer observerMu.Unlock()

	old := observers.Load()
	next := make([]Observer, 0, lenOf(old)+1)
	if old != nil {
		next = append(next, *old...)
	}
	next = append(next, o)
	observers.Store(&next)
}

// notifyObservers 通知全部观察者，无观察者时仅一次原子读
func notifyObservers(ctx context.Context, r Record) {
	p := observers.Load()
	if p == nil {
		return
	}
	for _, o := range *p {
		invokeObserver(ctx, o, r)
	}
}

// invokeObserver 调用单个观察者并隔离其 panic
//
// 观察者由使用方提供，其 panic 不应顺着日志调用把业务协程带崩，
// 也不应影响后续观察者与日志本身的输出
func invokeObserver(ctx context.Context, o Observer, r Record) {
	defer func() {
		if rec := recover(); rec != nil {
			xutil.ErrorIfEnableDebug("XOne log observer panicked, recovered=[%v]", rec)
		}
	}()
	o(ctx, r)
}

func lenOf(p *[]Observer) int {
	if p == nil {
		return 0
	}
	return len(*p)
}
