package xlog

import (
	"context"

	"github.com/xiaoshicae/xone/v3/xutil"
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

// ObserverHandle AddObserver 返回的句柄，用于注销
type ObserverHandle struct {
	id uint64
}

// registeredObserver 带标识的观察者，标识用于精确注销
type registeredObserver struct {
	id uint64
	fn Observer
}

var (
	// observers 采用读时无锁的写时复制，日志热路径只做一次原子读
	observers atomic.Pointer[[]registeredObserver]

	// observerMu 仅保护注册过程
	observerMu sync.Mutex

	// observerSeq 观察者标识自增序列
	observerSeq atomic.Uint64
)

// AddObserver 注册日志观察者，注册后对所有级别的日志生效
// 观察者需自行按 Record.Level 过滤关心的级别
//
// 返回的句柄可传给 RemoveObserver 注销；不需要注销时忽略即可
func AddObserver(o Observer) ObserverHandle {
	if o == nil {
		return ObserverHandle{}
	}

	observerMu.Lock()
	defer observerMu.Unlock()

	id := observerSeq.Add(1)
	old := observers.Load()
	var next []registeredObserver
	if old != nil {
		next = make([]registeredObserver, 0, len(*old)+1)
		next = append(next, *old...)
	} else {
		next = make([]registeredObserver, 0, 1)
	}
	next = append(next, registeredObserver{id: id, fn: o})
	observers.Store(&next)
	return ObserverHandle{id: id}
}

// RemoveObserver 注销此前注册的观察者
//
// 以注册时返回的句柄为准而不是比较函数值：Go 中函数不可比较，
// 相同的闭包每次构造都是不同的实例，按值找是找不回来的
func RemoveObserver(h ObserverHandle) {
	if h.id == 0 {
		return
	}

	observerMu.Lock()
	defer observerMu.Unlock()

	old := observers.Load()
	if old == nil {
		return
	}
	next := make([]registeredObserver, 0, len(*old))
	for _, o := range *old {
		if o.id != h.id {
			next = append(next, o)
		}
	}
	observers.Store(&next)
}

// notifyObservers 通知全部观察者，无观察者时仅一次原子读
func notifyObservers(ctx context.Context, r Record) {
	p := observers.Load()
	if p == nil {
		return
	}
	for _, o := range *p {
		invokeObserver(ctx, o.fn, r)
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
