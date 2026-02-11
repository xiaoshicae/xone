package xflow

import (
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
	// EnableMonitor 是否启用监控，默认 false（零开销）
	EnableMonitor bool
	// Monitor 可选自定义监控实现，nil 时使用 defaultMonitor
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

// SetEnableMonitor 设置是否启用监控
func (f *Flow[T]) SetEnableMonitor(enable bool) {
	f.EnableMonitor = enable
}

// AddProcessor 添加 Processor
// 注意：非并发安全，必须在 Execute 前完成配置
func (f *Flow[T]) AddProcessor(processor Processor[T]) {
	f.Processors = append(f.Processors, processor)
}

// Execute 执行流程，返回执行结果
func (f *Flow[T]) Execute(fc *FlowContext[T]) *ExecuteResult {
	if fc == nil {
		return &ExecuteResult{Err: fmt.Errorf("flow=[%s] FlowContext must not be nil", f.Name)}
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

		err := safeProcess(p, fc)

		if monitor != nil {
			monitor.OnProcessDone(f.Name, p.Name(), p.Dependency(), err, time.Since(start))
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
			f.rollback(fc, succeeded, result, monitor)

			if monitor != nil {
				monitor.OnFlowDone(f.Name, result, time.Since(flowStart))
			}
			return result
		}

		succeeded = append(succeeded, p)
	}

	if monitor != nil {
		monitor.OnFlowDone(f.Name, result, time.Since(flowStart))
	}

	return result
}

// rollback 逆序回滚已成功的处理器
func (f *Flow[T]) rollback(fc *FlowContext[T], succeeded []Processor[T], result *ExecuteResult, monitor Monitor) {
	result.Rolled = true

	for i := len(succeeded) - 1; i >= 0; i-- {
		p := succeeded[i]

		var start time.Time
		if monitor != nil {
			start = time.Now()
		}

		err := safeRollback(p, fc)

		if monitor != nil {
			monitor.OnRollbackDone(f.Name, p.Name(), p.Dependency(), err, time.Since(start))
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

// resolveMonitor 返回有效的 Monitor 实例，EnableMonitor=false 时返回 nil（零开销）
func (f *Flow[T]) resolveMonitor() Monitor {
	if !f.EnableMonitor {
		return nil
	}
	if f.Monitor != nil {
		return f.Monitor
	}
	return defaultMonitorInstance
}

// safeProcess 安全执行 Process，捕获 panic 并附带堆栈
func safeProcess[T any](p Processor[T], fc *FlowContext[T]) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v\n%s", r, debug.Stack())
		}
	}()
	return p.Process(fc)
}

// safeRollback 安全执行 Rollback，捕获 panic 并附带堆栈
func safeRollback[T any](p Processor[T], fc *FlowContext[T]) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v\n%s", r, debug.Stack())
		}
	}()
	return p.Rollback(fc)
}
