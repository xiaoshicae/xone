package xflow

import (
	"context"
	"sync"
	"time"

	"github.com/xiaoshicae/xone/v2/xlog"
)

// StepEvent 单步执行事件（Process / Rollback 共用）
type StepEvent struct {
	FlowName      string
	ProcessorName string
	Dependency    Dependency
	Err           error
	Duration      time.Duration
}

// FlowEvent 流程执行完成事件
type FlowEvent struct {
	FlowName string
	Result   *ExecuteResult
	Duration time.Duration
}

// Monitor 监控接口，Flow 可注入自定义实现以观测执行过程
type Monitor interface {
	// OnProcessDone Process 执行完成时调用（成功 Err=nil，失败 Err!=nil）
	OnProcessDone(ctx context.Context, event *StepEvent)
	// OnRollbackDone Rollback 执行完成时调用
	OnRollbackDone(ctx context.Context, event *StepEvent)
	// OnFlowDone Flow 整体执行完成时调用（包含回滚耗时）
	OnFlowDone(ctx context.Context, event *FlowEvent)
}

// defaultMonitor 默认实现，使用 xlog 打印
type defaultMonitor struct{}

func (d *defaultMonitor) OnProcessDone(ctx context.Context, e *StepEvent) {
	if e.Err != nil {
		xlog.Warn(ctx, "[xflow] flow=[%s] processor=[%s] dependency=[%s] duration=[%s] process=[failed] err=[%v]",
			e.FlowName, e.ProcessorName, e.Dependency, e.Duration, e.Err)
		return
	}
	xlog.Info(ctx, "[xflow] flow=[%s] processor=[%s] dependency=[%s] duration=[%s] process=[success]",
		e.FlowName, e.ProcessorName, e.Dependency, e.Duration)
}

func (d *defaultMonitor) OnRollbackDone(ctx context.Context, e *StepEvent) {
	if e.Err != nil {
		xlog.Warn(ctx, "[xflow] flow=[%s] processor=[%s] dependency=[%s] duration=[%s] rollback=[failed] err=[%v]",
			e.FlowName, e.ProcessorName, e.Dependency, e.Duration, e.Err)
		return
	}
	xlog.Info(ctx, "[xflow] flow=[%s] processor=[%s] dependency=[%s] duration=[%s] rollback=[success]",
		e.FlowName, e.ProcessorName, e.Dependency, e.Duration)
}

func (d *defaultMonitor) OnFlowDone(ctx context.Context, e *FlowEvent) {
	status := "success"
	if !e.Result.Success() {
		status = "failed"
	}
	xlog.Info(ctx, "[xflow] flow=[%s] duration=[%s] status=[%s] rolled=[%t]",
		e.FlowName, e.Duration, status, e.Result.Rolled)
}

var (
	defaultMonitorInstance Monitor = &defaultMonitor{}
	monitorMu             sync.RWMutex
)

// SetDefaultMonitor 设置全局默认 Monitor 实现，替换内置的 xlog 打印
func SetDefaultMonitor(m Monitor) {
	monitorMu.Lock()
	defer monitorMu.Unlock()
	defaultMonitorInstance = m
}

// GetDefaultMonitor 获取全局默认 Monitor 实现
func GetDefaultMonitor() Monitor {
	monitorMu.RLock()
	defer monitorMu.RUnlock()
	return defaultMonitorInstance
}
