package xutil

import (
	"context"
	"sync"
)

// DefaultPoolSize 默认任务池 worker 数量
const DefaultPoolSize = 100

var defaultPool = NewPool(DefaultPoolSize)

// Submit 向全局默认任务池提交一个任务
func Submit(task func()) {
	defaultPool.Submit(task)
}

// Pool 是一个固定 worker 数量的异步任务池
// 通过 Submit 提交任务，后台 worker 并发执行
type Pool struct {
	tasks    chan func()
	wg       sync.WaitGroup
	cancel   context.CancelFunc
	ctx      context.Context
	stopOnce sync.Once
}

// NewPool 创建一个包含指定数量 worker 的任务池
// workerCount 必须 >= 1，否则默认为 1
func NewPool(workerCount int) *Pool {
	if workerCount < 1 {
		workerCount = 1
	}
	ctx, cancel := context.WithCancel(context.Background())
	p := &Pool{
		tasks:  make(chan func(), workerCount*16),
		cancel: cancel,
		ctx:    ctx,
	}
	p.wg.Add(workerCount)
	for range workerCount {
		go p.worker()
	}
	return p
}

// worker 从任务队列中取出并执行任务
func (p *Pool) worker() {
	defer p.wg.Done()
	for task := range p.tasks {
		task()
	}
}

// Submit 提交一个任务到任务池，任务将由空闲 worker 执行
// 如果任务池已关闭，Submit 不执行任何操作
func (p *Pool) Submit(task func()) {
	select {
	case <-p.ctx.Done():
		return
	default:
	}
	select {
	case p.tasks <- task:
	case <-p.ctx.Done():
	}
}

// Go 提交一个返回结果的任务，返回 Future 用于异步获取结果
func Go[T any](p *Pool, fn func() (T, error)) *Future[T] {
	f := &Future[T]{
		done: make(chan struct{}),
	}
	p.Submit(func() {
		f.val, f.err = fn()
		close(f.done)
	})
	return f
}

// Shutdown 优雅关闭任务池：停止接收新任务，等待已提交的任务全部完成
// 多次调用安全，仅首次生效
func (p *Pool) Shutdown() {
	p.stopOnce.Do(func() {
		p.cancel()
		close(p.tasks)
	})
	p.wg.Wait()
}
