package xutil

import (
	"context"
	"time"
)

// Future 表示一个异步计算的结果，支持阻塞等待和超时等待
type Future[T any] struct {
	done chan struct{}
	val  T
	err  error
}

// Async 启动一个异步任务，返回 Future 用于获取结果
func Async[T any](fn func() (T, error)) *Future[T] {
	f := &Future[T]{
		done: make(chan struct{}),
	}
	go func() {
		f.val, f.err = fn()
		close(f.done)
	}()
	return f
}

// Get 阻塞等待异步任务完成，返回结果和错误
func (f *Future[T]) Get() (T, error) {
	<-f.done
	return f.val, f.err
}

// GetWithTimeout 等待异步任务完成，超时返回 context.DeadlineExceeded
func (f *Future[T]) GetWithTimeout(timeout time.Duration) (T, error) {
	select {
	case <-f.done:
		return f.val, f.err
	case <-time.After(timeout):
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
