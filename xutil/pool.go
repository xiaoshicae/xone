package xutil

import (
	"errors"
	"sync"
)

const (
	// DefaultPoolSize 默认任务池 worker 数量
	DefaultPoolSize = 100

	// taskQueuePerWorker 每个 worker 对应的任务队列长度
	// 留出缓冲，避免生产者在 worker 短暂繁忙时立刻阻塞
	taskQueuePerWorker = 16
)

// ErrPoolClosed 任务池已关闭，任务未被接收
var ErrPoolClosed = errors.New("xutil: pool is closed")

// defaultPool 全局默认任务池，首次使用时才创建
//
// 不在包初始化时创建：xutil 会被所有模块 import，包级启动意味着每个
// 使用 xone 的服务都常驻 DefaultPoolSize 个 worker goroutine，
// 哪怕从不提交任务——既占内存，也污染每一次 goroutine dump。
var defaultPool = sync.OnceValue(func() *Pool { return NewPool(DefaultPoolSize) })

// Submit 向全局默认任务池提交一个任务，返回是否提交成功
func Submit(task func()) bool {
	return defaultPool().Submit(task)
}

// Pool 是一个固定 worker 数量的异步任务池
// 通过 Submit 提交任务，后台 worker 并发执行
type Pool struct {
	tasks chan func()
	wg    sync.WaitGroup

	// mu 保证「向 tasks 发送」与「关闭 tasks」互斥
	//
	// 发送方持读锁、关闭方持写锁：关闭发生时不可能有发送在途，
	// 因此不会出现 send on closed channel。
	mu       sync.RWMutex
	closed   bool
	stopOnce sync.Once
}

// NewPool 创建一个包含指定数量 worker 的任务池
// workerCount 必须 >= 1，否则默认为 1
func NewPool(workerCount int) *Pool {
	if workerCount < 1 {
		workerCount = 1
	}
	p := &Pool{
		tasks: make(chan func(), workerCount*taskQueuePerWorker),
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
		safeRun(task)
	}
}

// Submit 提交一个任务到任务池，任务将由空闲 worker 执行
//
// 返回 false 表示任务未被接收：任务为 nil，或任务池已关闭。
// 队列满时会阻塞到有空位，这与 Shutdown 的「等待已提交任务全部完成」语义一致。
func (p *Pool) Submit(task func()) bool {
	if task == nil {
		return false
	}

	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.closed {
		return false
	}
	p.tasks <- task
	return true
}

// Shutdown 优雅关闭任务池：停止接收新任务，等待已提交的任务全部完成
// 多次调用安全，仅首次生效
func (p *Pool) Shutdown() {
	p.stopOnce.Do(func() {
		// 取写锁即等待所有在途 Submit 退出，之后关闭才是安全的
		p.mu.Lock()
		p.closed = true
		close(p.tasks)
		p.mu.Unlock()
	})
	p.wg.Wait()
}

// safeRun 执行任务并隔离 panic
//
// 任务池里的 panic 会同时炸掉进程和这个 worker，
// 一个业务函数里的空指针不该有这种影响面。
func safeRun(task func()) {
	defer func() {
		if r := recover(); r != nil {
			ErrorIfEnableDebug("XOne xutil task panicked, recovered, %v", r)
		}
	}()
	task()
}
