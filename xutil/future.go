package xutil

import (
	"context"
	"runtime/debug"
	"sync"
	"time"

	"github.com/xiaoshicae/xone/v2/xerror"
)

// Future 表示一个异步计算的结果，支持阻塞等待和超时等待
type Future[T any] struct {
	done chan struct{}
	once sync.Once
	val  T
	err  error
}

// Async 启动一个异步任务，返回 Future 用于获取结果
//
// 任务中的 panic 会被捕获并转成 error 由 Get 返回，不会崩掉进程。
func Async[T any](fn func() (T, error)) *Future[T] {
	f := newFuture[T]()
	go func() {
		f.complete(safeCall(fn))
	}()
	return f
}

// AsyncWithPool 在任务池中执行一个返回结果的任务，返回 Future 用于异步获取结果
//
// 与 Async 的区别只在执行载体：Async 每次新起一个 goroutine，没有并发上限；
// 本函数复用池中的 worker，并发数受池大小约束，适合「处理一万个 item 但只要
// 十个并发」这类需要限流的场景。
//
// 队列满时会阻塞调用方，直到有空位或任务池被关闭。池由调用方自己创建、
// 自己知道容量，这里的背压是合理的；不想等就用 TryAsyncWithPool。
//
// 任务池已关闭时立即以 ErrPoolClosed 完成，而不是留下一个永远不会
// 被关闭的 Future——那会让每个 Get 的调用方永久阻塞。
func AsyncWithPool[T any](p *Pool, fn func() (T, error)) *Future[T] {
	f := newFuture[T]()
	if !p.Submit(func() { f.complete(safeCall(fn)) }) {
		var zero T
		f.complete(zero, xerror.New("xutil", "AsyncWithPool", ErrPoolClosed))
	}
	return f
}

// TryAsyncWithPool 同 AsyncWithPool，但队列满时不阻塞，立即以 ErrPoolFull 完成
//
// 适合「宁可降级也不要等」的场景：拿到 ErrPoolFull 说明池子来不及处理，
// 调用方可以同步执行、丢弃或记一次指标，而不是被拖住。
func TryAsyncWithPool[T any](p *Pool, fn func() (T, error)) *Future[T] {
	f := newFuture[T]()
	if !p.TrySubmit(func() { f.complete(safeCall(fn)) }) {
		// 先判已关闭：关闭是永久状态、不必重试，队列满则是瞬时的，
		// 两者对调用方的处置完全不同，不能笼统报一个"没收下"
		cause := ErrPoolFull
		if p.isClosed() {
			cause = ErrPoolClosed
		}
		var zero T
		f.complete(zero, xerror.New("xutil", "TryAsyncWithPool", cause))
	}
	return f
}

func newFuture[T any]() *Future[T] {
	return &Future[T]{done: make(chan struct{})}
}

// complete 写入结果并唤醒等待方，重复调用只有首次生效
func (f *Future[T]) complete(val T, err error) {
	f.once.Do(func() {
		f.val, f.err = val, err
		close(f.done)
	})
}

// Get 阻塞等待异步任务完成，返回结果和错误
func (f *Future[T]) Get() (T, error) {
	<-f.done
	return f.val, f.err
}

// GetWithContext 等待异步任务完成，ctx 结束时返回 ctx.Err()
func (f *Future[T]) GetWithContext(ctx context.Context) (T, error) {
	select {
	case <-f.done:
		return f.val, f.err
	case <-ctx.Done():
		var zero T
		return zero, ctx.Err()
	}
}

// GetWithTimeout 等待异步任务完成，超时返回 context.DeadlineExceeded
func (f *Future[T]) GetWithTimeout(timeout time.Duration) (T, error) {
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case <-f.done:
		return f.val, f.err
	case <-timer.C:
		var zero T
		return zero, context.DeadlineExceeded
	}
}

// IsDone 非阻塞检查异步任务是否已完成
func (f *Future[T]) IsDone() bool {
	select {
	case <-f.done:
		return true
	default:
		return false
	}
}

// safeCall 执行任务并把 panic 转成 error
func safeCall[T any](fn func() (T, error)) (val T, err error) {
	defer func() {
		if r := recover(); r != nil {
			var zero T
			val = zero
			err = xerror.Newf("xutil", "async", "panic occurred, %v\n%s", r, debug.Stack())
		}
	}()
	return fn()
}
