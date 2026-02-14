package xflow

import "fmt"

// StepError 单步执行错误，包含处理器名称、依赖类型和原始错误
type StepError struct {
	ProcessorName string
	Dependency    Dependency
	Err           error
}

// Error 返回格式化的错误信息
func (se *StepError) Error() string {
	return fmt.Sprintf("processor=[%s], dependency=[%s], err=[%v]", se.ProcessorName, se.Dependency, se.Err)
}

// Unwrap 返回原始错误，支持 errors.Is / errors.As
func (se *StepError) Unwrap() error {
	return se.Err
}

// ResultSummary 非泛型的执行结果摘要接口，供 Monitor 使用
type ResultSummary interface {
	// Success 流程是否成功完成
	Success() bool
	// IsRolled 是否触发了回滚
	IsRolled() bool
	// HasSkippedErrors 是否存在弱依赖跳过的错误
	HasSkippedErrors() bool
	// HasRollbackErrors 是否存在回滚错误
	HasRollbackErrors() bool
	fmt.Stringer
}

// ExecuteResult 流程执行结果，携带泛型 Response
type ExecuteResult[Resp any] struct {
	// Data 用户自定义返回值，流程成功时由 Processor 填充的 Response
	Data Resp
	// Err 致命错误（强依赖失败），nil 表示流程完成
	Err error
	// SkippedErrors 弱依赖跳过的错误
	SkippedErrors []*StepError
	// RollbackErrors 回滚过程中的错误
	RollbackErrors []*StepError
	// Rolled 是否触发了回滚
	Rolled bool
}

// Success 流程是否成功完成（无强依赖失败）
func (r *ExecuteResult[Resp]) Success() bool {
	return r.Err == nil
}

// IsRolled 是否触发了回滚
func (r *ExecuteResult[Resp]) IsRolled() bool {
	return r.Rolled
}

// HasSkippedErrors 是否存在弱依赖跳过的错误
func (r *ExecuteResult[Resp]) HasSkippedErrors() bool {
	return len(r.SkippedErrors) > 0
}

// HasRollbackErrors 是否存在回滚错误
func (r *ExecuteResult[Resp]) HasRollbackErrors() bool {
	return len(r.RollbackErrors) > 0
}

// String 返回格式化的结果摘要，实现 fmt.Stringer 接口
func (r *ExecuteResult[Resp]) String() string {
	if r.Err == nil {
		return ""
	}
	msg := fmt.Sprintf("flow failed: %v", r.Err)
	if r.Rolled {
		msg += ", rolled back"
	}
	if len(r.RollbackErrors) > 0 {
		msg += fmt.Sprintf(", rollback errors=[%d]", len(r.RollbackErrors))
	}
	return msg
}
