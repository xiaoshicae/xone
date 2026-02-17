package xpipeline

import (
	"context"
	"sync"
	"time"

	"github.com/xiaoshicae/xone/v2/xlog"
)

// StepEvent 处理器执行完成事件
type StepEvent struct {
	PipelineName  string
	ProcessorName string
	Err           error
	Duration      time.Duration
}

// PipelineEvent Pipeline 执行完成事件
type PipelineEvent struct {
	PipelineName string
	Result       ResultSummary
	Duration     time.Duration
}

// Monitor 监控接口，Pipeline 可注入自定义实现以观测执行过程
// 注意：回调从 goroutine 中调用，实现必须并发安全
type Monitor interface {
	// OnProcessorDone 处理器 goroutine 结束时调用（成功 Err=nil，失败 Err!=nil）
	OnProcessorDone(ctx context.Context, event *StepEvent)
	// OnPipelineDone 所有处理器结束后调用
	OnPipelineDone(ctx context.Context, event *PipelineEvent)
}

// defaultMonitor 默认实现，使用 xlog 打印
type defaultMonitor struct{}

func (d *defaultMonitor) OnProcessorDone(ctx context.Context, e *StepEvent) {
	if e.Err != nil {
		xlog.Warn(ctx, "[xpipeline] pipeline=[%s] processor=[%s] duration=[%s] status=[failed] err=[%v]",
			e.PipelineName, e.ProcessorName, e.Duration, e.Err)
		return
	}
	xlog.Info(ctx, "[xpipeline] pipeline=[%s] processor=[%s] duration=[%s] status=[success]",
		e.PipelineName, e.ProcessorName, e.Duration)
}

func (d *defaultMonitor) OnPipelineDone(ctx context.Context, e *PipelineEvent) {
	status := "success"
	if !e.Result.Success() {
		status = "failed"
	}
	xlog.Info(ctx, "[xpipeline] pipeline=[%s] duration=[%s] status=[%s]",
		e.PipelineName, e.Duration, status)
}

var (
	defaultMonitorInstance Monitor = &defaultMonitor{}
	monitorMu              sync.RWMutex
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
