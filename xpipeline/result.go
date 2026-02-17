package xpipeline

import "fmt"

// StepError 处理器执行错误，包含处理器名称和原始错误
type StepError struct {
	ProcessorName string
	Err           error
}

// Error 返回格式化的错误信息
func (se *StepError) Error() string {
	return fmt.Sprintf("processor=[%s], err=[%v]", se.ProcessorName, se.Err)
}

// Unwrap 返回原始错误，支持 errors.Is / errors.As
func (se *StepError) Unwrap() error {
	return se.Err
}

// ResultSummary 非泛型的执行结果摘要接口，供 Monitor 使用
type ResultSummary interface {
	// Success Pipeline 是否全部处理器成功完成
	Success() bool
	// HasErrors 是否存在处理器错误
	HasErrors() bool
	fmt.Stringer
}

// RunResult Pipeline 运行结果
type RunResult struct {
	// Errors 所有处理器的错误
	Errors []*StepError
}

// Success 所有处理器是否成功完成
func (r *RunResult) Success() bool {
	return len(r.Errors) == 0
}

// HasErrors 是否存在处理器错误
func (r *RunResult) HasErrors() bool {
	return len(r.Errors) > 0
}

// String 返回格式化的结果摘要
func (r *RunResult) String() string {
	if len(r.Errors) == 0 {
		return ""
	}
	return fmt.Sprintf("pipeline failed: errors=[%d]", len(r.Errors))
}
