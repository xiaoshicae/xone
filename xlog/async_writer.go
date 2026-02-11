package xlog

import (
	"io"
	"sync"
)

const (
	// defaultAsyncBufferSize 异步写入缓冲区大小
	defaultAsyncBufferSize = 4096

	// maxPoolBufSize 归还 pool 的 buffer 上限（超过则丢弃，避免持有过多内存）
	maxPoolBufSize = 8192
)

// logBufPool 复用日志 buffer，减少每条日志的堆分配
var logBufPool = sync.Pool{
	New: func() any {
		return make([]byte, 0, 1024)
	},
}

// asyncWriter 异步写入器，通过 channel + goroutine 将同步写入转为异步
// 实现 io.WriteCloser 接口
type asyncWriter struct {
	ch       chan []byte
	writer   io.WriteCloser
	wg       sync.WaitGroup
	once     sync.Once
	closeErr error
}

// newAsyncWriter 创建异步写入器
func newAsyncWriter(w io.WriteCloser, bufferSize int) *asyncWriter {
	if bufferSize <= 0 {
		bufferSize = defaultAsyncBufferSize
	}
	aw := &asyncWriter{
		ch:     make(chan []byte, bufferSize),
		writer: w,
	}
	aw.wg.Add(1)
	go aw.loop()
	return aw
}

// Write 将数据拷贝后发送到 channel，非阻塞（channel 满时阻塞）
func (aw *asyncWriter) Write(p []byte) (int, error) {
	// 从 pool 获取 buffer，必须拷贝（调用方可能复用 buffer）
	buf := logBufPool.Get().([]byte)
	if cap(buf) < len(p) {
		buf = make([]byte, len(p))
	} else {
		buf = buf[:len(p)]
	}
	copy(buf, p)
	aw.ch <- buf
	return len(p), nil
}

// Close 关闭 channel，等待所有数据写完，再关闭底层 writer
// 多次调用安全，底层 writer 只关闭一次
func (aw *asyncWriter) Close() error {
	aw.once.Do(func() {
		close(aw.ch)
		aw.wg.Wait()
		aw.closeErr = aw.writer.Close()
	})
	return aw.closeErr
}

// loop 消费 channel 中的数据，写入底层 writer，写完后归还 buffer 到 pool
func (aw *asyncWriter) loop() {
	defer aw.wg.Done()
	for buf := range aw.ch {
		_, _ = aw.writer.Write(buf)
		// 不归还过大的 buffer，避免 pool 持有过多内存
		if cap(buf) <= maxPoolBufSize {
			logBufPool.Put(buf[:0])
		}
	}
}
