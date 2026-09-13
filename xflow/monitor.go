package xflow

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/xiaoshicae/xone/v2/xlog"
	"github.com/xiaoshicae/xone/v2/xutil"
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
//
// 各回调均被 panic 隔离：监控实现出错只丢一次观测，不会打断业务流程。
type Monitor interface {
	// OnProcessDone Process 执行完成时调用（成功 Err=nil，失败 Err!=nil）
	OnProcessDone(ctx context.Context, event *StepEvent)
	// OnRollbackDone Rollback 执行完成时调用
	OnRollbackDone(ctx context.Context, event *StepEvent)
	// OnFlowDone Flow 整体执行完成时调用（包含回滚耗时）
	OnFlowDone(ctx context.Context, event *FlowEvent)
}

// stepKind 区分单步事件回调的种类
type stepKind int

const (
	monitorOnProcess stepKind = iota
	monitorOnRollback
)

// notifyStepDone 投递单步事件，监控实现 panic 不影响流程
func (f *Flow[T]) notifyStepDone(ctx context.Context, monitor Monitor, kind stepKind, p Processor[T], err error, start time.Time) {
	if monitor == nil {
		return
	}
	event := &StepEvent{
		FlowName:      f.name,
		ProcessorName: p.Name(),
		Dependency:    p.Dependency(),
		Err:           err,
		Duration:      time.Since(start),
	}
	safeNotify(func() {
		if kind == monitorOnRollback {
			monitor.OnRollbackDone(ctx, event)
			return
		}
		monitor.OnProcessDone(ctx, event)
	})
}

// notifyFlowDone 投递流程完成事件，监控实现 panic 不影响流程
func (f *Flow[T]) notifyFlowDone(ctx context.Context, monitor Monitor, result *ExecuteResult, start time.Time) {
	if monitor == nil {
		return
	}
	event := &FlowEvent{FlowName: f.name, Result: result, Duration: time.Since(start)}
	safeNotify(func() { monitor.OnFlowDone(ctx, event) })
}

// safeNotify 隔离监控实现的 panic
func safeNotify(fn func()) {
	defer func() {
		if r := recover(); r != nil {
			xutil.ErrorIfEnableDebug("XOne xflow monitor panicked, recovered, %v", r)
		}
	}()
	fn()
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
		e.FlowName, e.Duration, status, e.Result.IsRolled())
}

// defaultMonitorInstance 全局 Monitor，原子读写：Execute 的热点路径上无需加锁
var defaultMonitorInstance = func() *atomic.Pointer[Monitor] {
	v := &atomic.Pointer[Monitor]{}
	var m Monitor = &defaultMonitor{}
	v.Store(&m)
	return v
}()

// SetDefaultMonitor 设置全局默认 Monitor 实现，替换内置的 xlog 打印
//
// 传 nil 等同于关闭监控。
func SetDefaultMonitor(m Monitor) {
	if m == nil {
		defaultMonitorInstance.Store(nil)
		return
	}
	defaultMonitorInstance.Store(&m)
}

// GetDefaultMonitor 获取全局默认 Monitor 实现，未设置时返回 nil
func GetDefaultMonitor() Monitor {
	m := defaultMonitorInstance.Load()
	if m == nil {
		return nil
	}
	return *m
}

// resolveMonitor 返回本次执行使用的 Monitor，监控关闭时返回 nil（零开销）
func resolveMonitor() Monitor {
	if !monitorEnabled.Load() {
		return nil
	}
	return GetDefaultMonitor()
}
