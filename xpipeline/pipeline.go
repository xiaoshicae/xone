package xpipeline

import (
	"context"
	"fmt"
	"runtime/debug"
	"sync"
	"time"
)

// Pipeline Frame + Processor 流式编排器，将多个 Processor 串联成 channel 链
type Pipeline struct {
	// Name Pipeline 名称，用于日志和监控
	Name string
	// Processors 按执行顺序排列的处理器列表
	Processors []Processor
}

// New 函数式构建 Pipeline
func New(name string, processors ...Processor) *Pipeline {
	return &Pipeline{
		Name:       name,
		Processors: processors,
	}
}

// Run 启动 Pipeline：每个 Processor 在独立 goroutine 中运行，返回最终输出 channel
// ctx 取消时所有 Processor 应停止处理
func (p *Pipeline) Run(ctx context.Context, input <-chan Frame) <-chan Frame {
	if ctx == nil {
		ctx = context.Background()
	}

	if len(p.Processors) == 0 {
		ch := make(chan Frame)
		close(ch)
		return ch
	}

	cfg := GetConfig()
	monitor := p.resolveMonitor()

	// 创建中间 channel
	channels := make([]chan Frame, len(p.Processors))
	for i := range channels {
		channels[i] = make(chan Frame, cfg.BufferSize)
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	var stepErrors []*StepError
	pipelineStart := time.Now()

	// 串联：input → proc[0] → ch[0] → proc[1] → ch[1] → ... → proc[n-1] → ch[n-1]
	for i, proc := range p.Processors {
		var in <-chan Frame
		if i == 0 {
			in = input
		} else {
			in = channels[i-1]
		}
		out := channels[i]

		wg.Add(1)
		go func(pr Processor, inch <-chan Frame, outch chan Frame) {
			defer wg.Done()
			defer close(outch)

			var start time.Time
			if monitor != nil {
				start = time.Now()
			}

			err := safeProcess(pr, ctx, inch, outch)

			if err != nil {
				mu.Lock()
				stepErrors = append(stepErrors, &StepError{ProcessorName: pr.Name(), Err: err})
				mu.Unlock()
			}

			if monitor != nil {
				monitor.OnProcessorDone(ctx, &StepEvent{
					PipelineName:  p.Name,
					ProcessorName: pr.Name(),
					Err:           err,
					Duration:      time.Since(start),
				})
			}
		}(proc, in, out)
	}

	// 后台等待所有处理器结束，触发 OnPipelineDone
	if monitor != nil {
		go func() {
			wg.Wait()
			monitor.OnPipelineDone(ctx, &PipelineEvent{
				PipelineName: p.Name,
				Result:       &RunResult{Errors: stepErrors},
				Duration:     time.Since(pipelineStart),
			})
		}()
	}

	return channels[len(p.Processors)-1]
}

// resolveMonitor 返回有效的 Monitor 实例，config 禁用时返回 nil（零开销）
func (p *Pipeline) resolveMonitor() Monitor {
	if GetConfig().DisableMonitor {
		return nil
	}
	return GetDefaultMonitor()
}

// safeProcess 安全执行 Process，捕获 panic 并附带堆栈
func safeProcess(pr Processor, ctx context.Context, input <-chan Frame, output chan<- Frame) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v\n%s", r, debug.Stack())
		}
	}()
	return pr.Process(ctx, input, output)
}
