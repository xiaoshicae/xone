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

// TrySubmit 向全局默认任务池提交一个任务，队列满时直接返回 false，不会阻塞
//
// 全局池由进程内所有调用方共用。共享池上不做阻塞背压是有意的：
// 一个业务的慢任务把队列填满后，阻塞会传播给毫不相干的另一个业务，
// 而后者既不知道前者的存在，也没有"慢下来"的余地——它只是想扔个埋点。
//
// 需要背压（满了就让生产者等）请用 NewPool 自建池并调用 Pool.Submit，
// 那是调用方独占的池子，知道容量也控制得了生产速度。
//
// 全局池有 DefaultPoolSize*taskQueuePerWorker 个排队位，还能填满说明
// 这不是"零散轻量任务"的场景，同样应该自建池。
func TrySubmit(task func()) bool {
	ok := defaultPool().TrySubmit(task)
	if !ok && task != nil {
		ErrorIfEnableDebug("XOne xutil global pool is full, task dropped")
	}
	return ok
}

// Pool 是一个固定 worker 数量的异步任务池
// 通过 Submit 提交任务，后台 worker 并发执行
type Pool struct {
	tasks chan func()
	wg    sync.WaitGroup

	// done 关闭信号，先于 tasks 关闭，使阻塞在队列满上的 Submit 能及时退出
	//
	// 只有 mu 是不够的：Submit 在队列满时会持着读锁阻塞在发送上，
	// Shutdown 就永远取不到写锁；而 Go 的 RWMutex 写者优先，
	// 等待中的 Shutdown 还会连带把后续所有 Submit 一起挡住——
	// 一个卡住的任务足以让进程里每一处 Submit 全部挂起。
	done chan struct{}

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
		done:  make(chan struct{}),
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
// 队列满时阻塞到有空位或任务池被关闭，这与 Shutdown 的
// 「等待已提交任务全部完成」语义一致。不想等就用 TrySubmit。
//
// 返回 false 表示任务未被接收：任务为 nil，或任务池已关闭。
// 返回 true 则任务一定会被执行——发送在读锁内完成，此时 tasks 不可能已关闭。
func (p *Pool) Submit(task func()) bool {
	if task == nil {
		return false
	}

	p.mu.RLock()
	defer p.mu.RUnlock()

	select {
	case <-p.done:
		return false
	default:
	}

	select {
	case p.tasks <- task:
		return true
	case <-p.done:
		// 队列满且此时被关闭：放弃本任务，同时让出读锁，Shutdown 才能继续
		return false
	}
}

// TrySubmit 提交一个任务，队列满时直接返回 false，不会阻塞
//
// 返回 false 表示任务未被接收：任务为 nil、任务池已关闭，或队列已满。
func (p *Pool) TrySubmit(task func()) bool {
	if task == nil {
		return false
	}

	p.mu.RLock()
	defer p.mu.RUnlock()

	select {
	case <-p.done:
		return false
	default:
	}

	select {
	case p.tasks <- task:
		return true
	default:
		return false
	}
}

// Shutdown 优雅关闭任务池：停止接收新任务，等待已提交的任务全部完成
// 多次调用安全，仅首次生效
func (p *Pool) Shutdown() {
	p.stopOnce.Do(func() {
		// 先广播关闭：阻塞在队列满上的 Submit 会立即返回并让出读锁
		close(p.done)
		// 再取写锁，此时所有在途 Submit 都已退出，关闭 tasks 是安全的
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
