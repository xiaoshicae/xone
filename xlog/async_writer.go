package xlog

import (
	"errors"
	"io"
	"sync"
)

const (
	// defaultAsyncBufferSize 异步写入缓冲区大小
	defaultAsyncBufferSize = 4096

	// defaultBufCap 单条日志 buffer 的初始容量
	defaultBufCap = 1024

	// maxPoolBufSize 归还 pool 的 buffer 上限（超过则丢弃，避免持有过多内存）
	maxPoolBufSize = 8192
)

// logBufPool 复用日志 buffer，减少每条日志的堆分配
// 存放 *[]byte 而非 []byte：切片头是值类型，直接 Put 会因装箱产生一次额外分配
var logBufPool = sync.Pool{
	New: func() any {
		b := make([]byte, 0, defaultBufCap)
		return &b
	},
}

// getBuf 取出一个长度为 n 的 buffer
func getBuf(n int) *[]byte {
	bp := logBufPool.Get().(*[]byte)
	if cap(*bp) < n {
		*bp = make([]byte, n)
	} else {
		*bp = (*bp)[:n]
	}
	return bp
}

// putBuf 归还 buffer，过大的不归还以免 pool 长期占用内存
func putBuf(bp *[]byte) {
	if cap(*bp) > maxPoolBufSize {
		return
	}
	*bp = (*bp)[:0]
	logBufPool.Put(bp)
}

var errAsyncWriterClosed = errors.New("async writer is closed")

// asyncWriter 异步写入器，通过 channel + goroutine 将同步写入转为异步
// 实现 io.WriteCloser 接口
type asyncWriter struct {
	ch     chan *[]byte
	writer io.WriteCloser

	// done 关闭信号，先于 ch 关闭，使阻塞中的 Write 能及时退出
	done chan struct{}

	// mu 读锁由 Write 持有、写锁由 Close 持有，
	// 保证 close(ch) 时不存在正在发送的 Write，从而无需 recover 兜底
	mu sync.RWMutex

	wg        sync.WaitGroup
	closeOnce sync.Once
	writeOnce sync.Once
	writeErr  error
	closeErr  error
}

// newAsyncWriter 创建异步写入器
func newAsyncWriter(w io.WriteCloser, bufferSize int) *asyncWriter {
	if bufferSize <= 0 {
		bufferSize = defaultAsyncBufferSize
	}
	aw := &asyncWriter{
		ch:     make(chan *[]byte, bufferSize),
		writer: w,
		done:   make(chan struct{}),
	}
	aw.wg.Add(1)
	go aw.loop()
	return aw
}

// Write 将数据拷贝后异步投递，缓冲区满时阻塞直到有空位或写入器被关闭
func (aw *asyncWriter) Write(p []byte) (int, error) {
	// 持读锁期间 Close 无法执行 close(ch)，发送必然安全；
	// 读锁不互斥，多个 Write 仍可并发
	aw.mu.RLock()
	defer aw.mu.RUnlock()

	select {
	case <-aw.done:
		return 0, errAsyncWriterClosed
	default:
	}

	bp := getBuf(len(p))
	copy(*bp, p)

	select {
	case aw.ch <- bp:
		return len(p), nil
	case <-aw.done:
		// 缓冲区满且此时被关闭，放弃本条日志
		putBuf(bp)
		return 0, errAsyncWriterClosed
	}
}

// Close 停止接收新日志，等待缓冲区写完后关闭底层 writer
// 多次调用安全，底层 writer 只关闭一次
func (aw *asyncWriter) Close() error {
	aw.closeOnce.Do(func() {
		// 1. 先广播关闭，阻塞在缓冲区满的 Write 会立即返回
		close(aw.done)
		// 2. 取写锁，等待所有进行中的 Write 退出后再关闭 channel
		aw.mu.Lock()
		close(aw.ch)
		aw.mu.Unlock()
		// 3. 等待消费协程把已入队数据写完
		aw.wg.Wait()
		aw.closeErr = aw.writer.Close()
	})

	if aw.closeErr != nil {
		return aw.closeErr
	}
	return aw.writeErr
}

// loop 消费 channel 中的数据写入底层 writer，写完归还 buffer
func (aw *asyncWriter) loop() {
	defer aw.wg.Done()
	for bp := range aw.ch {
		if _, err := aw.writer.Write(*bp); err != nil {
			aw.writeOnce.Do(func() {
				aw.writeErr = err
			})
		}
		putBuf(bp)
	}
}
