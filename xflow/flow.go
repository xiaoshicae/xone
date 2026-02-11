package xflow

import (
	"context"
	"fmt"
	"runtime/debug"
	"time"
)

// Flow 流程编排器，按顺序执行 Processor，支持强弱依赖和自动回滚
type Flow[T any] struct {
	// Name 流程名称，用于日志和监控
	Name string
	// Processors 按执行顺序排列的处理器列表
	Processors []Processor[T]
	// Monitor 可选自定义监控实现，nil 时使用全局默认 Monitor
	Monitor Monitor
}

// New 函数式构建 Flow
func New[T any](name string, processors ...Processor[T]) *Flow[T] {
	return &Flow[T]{
		Name:       name,
		Processors: processors,
	}
}

// SetName 设置 Name
func (f *Flow[T]) SetName(name string) {
	f.Name = name
}

// SetMonitor 设置自定义 Monitor
func (f *Flow[T]) SetMonitor(monitor Monitor) {
	f.Monitor = monitor
}

// AddProcessor 添加 Processor
// 注意：非并发安全，必须在 Execute 前完成配置
func (f *Flow[T]) AddProcessor(processor Processor[T]) {
	f.Processors = append(f.Processors, processor)
}

// Execute 执行流程，返回执行结果
func (f *Flow[T]) Execute(ctx context.Context, data T) *ExecuteResult {
	if ctx == nil {
		ctx = context.Background()
	}

	monitor := f.resolveMonitor()
	result := &ExecuteResult{}

	var flowStart time.Time
	if monitor != nil {
		flowStart = time.Now()
	}

	// 记录已成功执行的处理器，用于回滚
	succeeded := make([]Processor[T], 0, len(f.Processors))

	for _, p := range f.Processors {
		var start time.Time
		if monitor != nil {
			start = time.Now()
		}

		err := safeProcess(p, ctx, data)

		if monitor != nil {
			monitor.OnProcessDone(ctx, f.Name, p.Name(), p.Dependency(), err, time.Since(start))
		}

		if err != nil {
			se := &StepError{
				ProcessorName: p.Name(),
				Dependency:    p.Dependency(),
				Err:           err,
			}

			if p.Dependency() == Weak {
				// 弱依赖：记录错误，加入 succeeded（可回滚），继续执行
				result.SkippedErrors = append(result.SkippedErrors, se)
				succeeded = append(succeeded, p)
				continue
			}

			// 强依赖：中断流程，触发回滚
			result.Err = se
			f.rollback(ctx, data, succeeded, result, monitor)

			if monitor != nil {
				monitor.OnFlowDone(ctx, f.Name, result, time.Since(flowStart))
			}
			return result
		}

		succeeded = append(succeeded, p)
	}

	if monitor != nil {
		monitor.OnFlowDone(ctx, f.Name, result, time.Since(flowStart))
	}

	return result
}

// rollback 逆序回滚已成功的处理器
func (f *Flow[T]) rollback(ctx context.Context, data T, succeeded []Processor[T], result *ExecuteResult, monitor Monitor) {
	result.Rolled = true

	for i := len(succeeded) - 1; i >= 0; i-- {
		p := succeeded[i]

		var start time.Time
		if monitor != nil {
			start = time.Now()
		}

		err := safeRollback(p, ctx, data)

		if monitor != nil {
			monitor.OnRollbackDone(ctx, f.Name, p.Name(), p.Dependency(), err, time.Since(start))
		}

		if err != nil {
			se := &StepError{
				ProcessorName: p.Name(),
				Dependency:    p.Dependency(),
				Err:           err,
			}
			result.RollbackErrors = append(result.RollbackErrors, se)
		}
	}
}

// resolveMonitor 返回有效的 Monitor 实例，config 禁用时返回 nil（零开销）
func (f *Flow[T]) resolveMonitor() Monitor {
	if GetConfig().DisableMonitor {
		return nil
	}
	if f.Monitor != nil {
		return f.Monitor
	}
	return GetDefaultMonitor()
}

// safeProcess 安全执行 Process，捕获 panic 并附带堆栈
func safeProcess[T any](p Processor[T], ctx context.Context, data T) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v\n%s", r, debug.Stack())
		}
	}()
	return p.Process(ctx, data)
}

// safeRollback 安全执行 Rollback，捕获 panic 并附带堆栈
func safeRollback[T any](p Processor[T], ctx context.Context, data T) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v\n%s", r, debug.Stack())
		}
	}()
	return p.Rollback(ctx, data)
}
