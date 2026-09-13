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

// Go 向任务池提交一个返回结果的任务，返回 Future 用于异步获取结果
//
// 任务池已关闭时立即以 ErrPoolClosed 完成，而不是留下一个永远不会
// 被关闭的 Future——那会让每个 Get 的调用方永久阻塞。
func Go[T any](p *Pool, fn func() (T, error)) *Future[T] {
	f := newFuture[T]()
	if !p.Submit(func() { f.complete(safeCall(fn)) }) {
		var zero T
		f.complete(zero, xerror.New("xutil", "Go", ErrPoolClosed))
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
