package xlog

import (
	"io"
	"sync"
)

const (
	// defaultAsyncBufferSize 异步写入缓冲区大小
	defaultAsyncBufferSize = 4096
)

// asyncWriter 异步写入器，通过 channel + goroutine 将同步写入转为异步
// 实现 io.WriteCloser 接口
type asyncWriter struct {
	ch     chan []byte
	writer io.WriteCloser
	wg     sync.WaitGroup
	once   sync.Once
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
	// 必须拷贝，因为调用方可能复用 buffer
	buf := make([]byte, len(p))
	copy(buf, p)
	aw.ch <- buf
	return len(p), nil
}

// Close 关闭 channel，等待所有数据写完，再关闭底层 writer
func (aw *asyncWriter) Close() error {
	aw.once.Do(func() {
		close(aw.ch)
	})
	aw.wg.Wait()
	return aw.writer.Close()
}

// loop 消费 channel 中的数据，写入底层 writer
func (aw *asyncWriter) loop() {
	defer aw.wg.Done()
	for buf := range aw.ch {
		_, _ = aw.writer.Write(buf)
	}
}