package xflow

import (
	"context"
	"fmt"
	"runtime/debug"
	"time"
)

// Flow 流程编排器，按顺序执行 Processor，支持强弱依赖和自动回滚
type Flow[Req, Resp any] struct {
	// Name 流程名称，用于日志和监控
	Name string
	// Processors 按执行顺序排列的处理器列表
	Processors []Processor[Req, Resp]
}

// New 函数式构建 Flow
func New[Req, Resp any](name string, processors ...Processor[Req, Resp]) *Flow[Req, Resp] {
	return &Flow[Req, Resp]{
		Name:       name,
		Processors: processors,
	}
}

// Execute 执行流程，接收 Req 返回 *ExecuteResult[Resp]
func (f *Flow[Req, Resp]) Execute(ctx context.Context, req Req) *ExecuteResult[Resp] {
	if ctx == nil {
		ctx = context.Background()
	}

	monitor := f.resolveMonitor()
	result := &ExecuteResult[Resp]{}

	// 构造 FlowData，Response 为零值，extra 惰性初始化
	data := &FlowData[Req, Resp]{Request: req}

	var flowStart time.Time
	if monitor != nil {
		flowStart = time.Now()
	}

	// 记录已成功执行的处理器，用于回滚
	succeeded := make([]Processor[Req, Resp], 0, len(f.Processors))

	for _, p := range f.Processors {
		var start time.Time
		if monitor != nil {
			start = time.Now()
		}

		err := safeProcess(p, ctx, data)

		if monitor != nil {
			monitor.OnProcessDone(ctx, &StepEvent{
				FlowName:      f.Name,
				ProcessorName: p.Name(),
				Dependency:    p.Dependency(),
				Err:           err,
				Duration:      time.Since(start),
			})
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
				monitor.OnFlowDone(ctx, &FlowEvent{FlowName: f.Name, Result: result, Duration: time.Since(flowStart)})
			}
			return result
		}

		succeeded = append(succeeded, p)
	}

	// 流程成功，将 Response 填充到结果
	result.Data = data.Response

	if monitor != nil {
		monitor.OnFlowDone(ctx, &FlowEvent{FlowName: f.Name, Result: result, Duration: time.Since(flowStart)})
	}

	return result
}

// rollback 逆序回滚已成功的处理器
func (f *Flow[Req, Resp]) rollback(ctx context.Context, data *FlowData[Req, Resp], succeeded []Processor[Req, Resp], result *ExecuteResult[Resp], monitor Monitor) {
	result.Rolled = true

	for i := len(succeeded) - 1; i >= 0; i-- {
		p := succeeded[i]

		var start time.Time
		if monitor != nil {
			start = time.Now()
		}

		err := safeRollback(p, ctx, data)

		if monitor != nil {
			monitor.OnRollbackDone(ctx, &StepEvent{
				FlowName:      f.Name,
				ProcessorName: p.Name(),
				Dependency:    p.Dependency(),
				Err:           err,
				Duration:      time.Since(start),
			})
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
func (f *Flow[Req, Resp]) resolveMonitor() Monitor {
	if GetConfig().DisableMonitor {
		return nil
	}
	return GetDefaultMonitor()
}

// safeProcess 安全执行 Process，捕获 panic 并附带堆栈
func safeProcess[Req, Resp any](p Processor[Req, Resp], ctx context.Context, data *FlowData[Req, Resp]) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v\n%s", r, debug.Stack())
		}
	}()
	return p.Process(ctx, data)
}

// safeRollback 安全执行 Rollback，捕获 panic 并附带堆栈
func safeRollback[Req, Resp any](p Processor[Req, Resp], ctx context.Context, data *FlowData[Req, Resp]) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v\n%s", r, debug.Stack())
		}
	}()
	return p.Rollback(ctx, data)
}
