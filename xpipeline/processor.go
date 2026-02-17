package xpipeline

import "context"

// Processor Pipeline 中的处理器接口，每个 Processor 在独立 goroutine 中运行
type Processor interface {
	// Name 处理器名称，用于日志和监控
	Name() string
	// Process 处理逻辑：从 input 读取 Frame，处理后写入 output
	// ctx 取消时处理器应尽快退出
	Process(ctx context.Context, input <-chan Frame, output chan<- Frame) error
}
