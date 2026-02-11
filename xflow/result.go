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

// ExecuteResult 流程执行结果
type ExecuteResult struct {
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
func (r *ExecuteResult) Success() bool {
	return r.Err == nil
}

// HasSkippedErrors 是否存在弱依赖跳过的错误
func (r *ExecuteResult) HasSkippedErrors() bool {
	return len(r.SkippedErrors) > 0
}

// HasRollbackErrors 是否存在回滚错误
func (r *ExecuteResult) HasRollbackErrors() bool {
	return len(r.RollbackErrors) > 0
}

// String 返回格式化的结果摘要，实现 fmt.Stringer 接口
func (r *ExecuteResult) String() string {
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
