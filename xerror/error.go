// Package xerror 提供 XOne 框架统一错误类型
package xerror

import (
	"errors"
	"fmt"
)

// XOneError 统一错误类型，包含模块名、操作名和原始错误
type XOneError struct {
	Module string // 模块名，如 "xconfig", "xgorm"
	Op     string // 操作名，如 "init", "close"
	Err    error  // 原始错误
}

// Error 实现 error 接口
func (e *XOneError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("XOne %s %s failed, err=[%v]", e.Module, e.Op, e.Err)
	}
	return fmt.Sprintf("XOne %s %s failed", e.Module, e.Op)
}

// Unwrap 支持 errors.Is / errors.As 链式判断
func (e *XOneError) Unwrap() error {
	return e.Err
}

// New 创建 XOneError
func New(module, op string, err error) *XOneError {
	return &XOneError{Module: module, Op: op, Err: err}
}

// Newf 创建带格式化消息的 XOneError
func Newf(module, op, format string, args ...any) *XOneError {
	return &XOneError{Module: module, Op: op, Err: fmt.Errorf(format, args...)}
}

// Is 判断 err 链中是否包含指定模块的 XOneError
func Is(err error, module string) bool {
	var xe *XOneError
	if errors.As(err, &xe) {
		return xe.Module == module
	}
	return false
}

// Module 从 err 链中提取模块名，若非 XOneError 则返回空字符串
func Module(err error) string {
	var xe *XOneError
	if errors.As(err, &xe) {
		return xe.Module
	}
	return ""
}
