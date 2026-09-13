package xflow

import (
	"context"
	"runtime/debug"
	"time"

	"github.com/xiaoshicae/xone/v2/xerror"
)

// Flow 流程编排器，按顺序执行 Processor，支持强弱依赖和自动回滚
//
// 构建后字段不再变化，可被并发 Execute。
type Flow[T any] struct {
	name       string
	processors []Processor[T]
}

// New 构建 Flow，processors 按传入顺序执行
//
// 传入 nil Processor 直接 panic：它只能在运行到该步时炸成空指针，
// 而那时错误早已脱离构建现场，越早暴露越好。
func New[T any](name string, processors ...Processor[T]) *Flow[T] {
	for i, p := range processors {
		if p == nil {
			panic(xerror.Newf("xflow", "New", "processor at index %d is nil, flow=[%s]", i, name).Error())
		}
	}
	return &Flow[T]{
		name:       name,
		processors: processors,
	}
}

// Name 返回流程名称
func (f *Flow[T]) Name() string {
	return f.name
}

// Execute 按顺序执行各 Processor，data 贯穿全程供处理器读写
//
// 强依赖失败或 ctx 被取消时中断流程并逆序回滚已执行的处理器；
// 弱依赖失败则记录到 SkippedErrors 后继续，但同样会被纳入回滚范围。
func (f *Flow[T]) Execute(ctx context.Context, data T) *ExecuteResult {
	if ctx == nil {
		ctx = context.Background()
	}

	monitor := resolveMonitor()
	result := &ExecuteResult{}

	var flowStart time.Time
	if monitor != nil {
		flowStart = time.Now()
	}

	// 记录已执行的处理器，用于回滚
	executed := make([]Processor[T], 0, len(f.processors))

	for _, p := range f.processors {
		// 调用方已放弃等待时不再启动新的处理器，但已完成的部分仍需回滚
		if err := ctx.Err(); err != nil {
			result.Err = xerror.Newf("xflow", "Execute",
				"flow=[%s] canceled before processor=[%s], err=[%w]", f.name, p.Name(), err)
			f.rollback(ctx, data, executed, result, monitor)
			f.notifyFlowDone(ctx, monitor, result, flowStart)
			return result
		}

		var start time.Time
		if monitor != nil {
			start = time.Now()
		}

		err := safeProcess(p, ctx, data)

		f.notifyStepDone(ctx, monitor, monitorOnProcess, p, err, start)

		if err == nil {
			executed = append(executed, p)
			continue
		}

		se := &StepError{ProcessorName: p.Name(), Dependency: p.Dependency(), Err: err}

		if p.Dependency() == Weak {
			// 弱依赖：记录错误，仍纳入回滚范围，继续执行
			result.SkippedErrors = append(result.SkippedErrors, se)
			executed = append(executed, p)
			continue
		}

		// 强依赖：中断流程，触发回滚
		result.Err = se
		f.rollback(ctx, data, executed, result, monitor)
		f.notifyFlowDone(ctx, monitor, result, flowStart)
		return result
	}

	f.notifyFlowDone(ctx, monitor, result, flowStart)
	return result
}

// rollback 逆序回滚已执行的处理器
//
// 回滚用的 context 剥离了原 context 的取消与超时：补偿逻辑（退款、还库存、
// 解冻额度）最需要执行的时机恰恰是请求超时之后，沿用已取消的 context
// 会让每个补偿调用一进去就被拒绝，资源就真的漏掉了。
// 剥离后改用 RollbackTimeout 单独限时，避免补偿无限期挂住。
func (f *Flow[T]) rollback(ctx context.Context, data T, executed []Processor[T], result *ExecuteResult, monitor Monitor) {
	result.Rolled = true

	rollbackCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), rollbackTimeout())
	defer cancel()

	for i := len(executed) - 1; i >= 0; i-- {
		p := executed[i]

		// 回滚预算耗尽：把未补偿的处理器逐个记下来，否则调用方无从得知哪些资源还悬着
		if err := rollbackCtx.Err(); err != nil {
			result.RollbackErrors = append(result.RollbackErrors, &StepError{
				ProcessorName: p.Name(),
				Dependency:    p.Dependency(),
				Err: xerror.Newf("xflow", "rollback",
					"rollback budget exhausted before this processor, err=[%w]", err),
			})
			continue
		}

		var start time.Time
		if monitor != nil {
			start = time.Now()
		}

		err := safeRollback(p, rollbackCtx, data)

		f.notifyStepDone(rollbackCtx, monitor, monitorOnRollback, p, err, start)

		if err != nil {
			result.RollbackErrors = append(result.RollbackErrors, &StepError{
				ProcessorName: p.Name(),
				Dependency:    p.Dependency(),
				Err:           err,
			})
		}
	}
}

// safeProcess 安全执行 Process，捕获 panic 并附带堆栈
func safeProcess[T any](p Processor[T], ctx context.Context, data T) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = xerror.Newf("xflow", "Process", "panic occurred, %v\n%s", r, debug.Stack())
		}
	}()
	return p.Process(ctx, data)
}

// safeRollback 安全执行 Rollback，捕获 panic 并附带堆栈
func safeRollback[T any](p Processor[T], ctx context.Context, data T) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = xerror.Newf("xflow", "Rollback", "panic occurred, %v\n%s", r, debug.Stack())
		}
	}()
	return p.Rollback(ctx, data)
}
