package xflow

import (
	"fmt"
	"time"
)

// Monitor 监控接口，Flow 可注入自定义实现以观测执行过程
type Monitor interface {
	// OnProcessDone Process 执行完成时调用（成功 err=nil，失败 err!=nil）
	OnProcessDone(flowName, processorName string, dependency Dependency, err error, duration time.Duration)
	// OnRollbackDone Rollback 执行完成时调用
	OnRollbackDone(flowName, processorName string, dependency Dependency, err error, duration time.Duration)
	// OnFlowDone Flow 整体执行完成时调用（包含回滚耗时）
	OnFlowDone(flowName string, result *ExecuteResult, duration time.Duration)
}

// defaultMonitor 默认实现，打印到标准输出
type defaultMonitor struct{}

func (d *defaultMonitor) OnProcessDone(flowName, processorName string, dependency Dependency, err error, duration time.Duration) {
	if err != nil {
		fmt.Printf("[xflow] flow=[%s] processor=[%s] dependency=[%s] duration=[%s] process=[failed] err=[%v]\n",
			flowName, processorName, dependency, duration, err)
		return
	}
	fmt.Printf("[xflow] flow=[%s] processor=[%s] dependency=[%s] duration=[%s] process=[success]\n",
		flowName, processorName, dependency, duration)
}

func (d *defaultMonitor) OnRollbackDone(flowName, processorName string, dependency Dependency, err error, duration time.Duration) {
	if err != nil {
		fmt.Printf("[xflow] flow=[%s] processor=[%s] dependency=[%s] duration=[%s] rollback=[failed] err=[%v]\n",
			flowName, processorName, dependency, duration, err)
		return
	}
	fmt.Printf("[xflow] flow=[%s] processor=[%s] dependency=[%s] duration=[%s] rollback=[success]\n",
		flowName, processorName, dependency, duration)
}

func (d *defaultMonitor) OnFlowDone(flowName string, result *ExecuteResult, duration time.Duration) {
	status := "success"
	if !result.Success() {
		status = "failed"
	}
	fmt.Printf("[xflow] flow=[%s] duration=[%s] status=[%s] rolled=[%t]\n",
		flowName, duration, status, result.Rolled)
}

var defaultMonitorInstance Monitor = &defaultMonitor{}
