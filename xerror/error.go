// Package xerror 提供 XOne 框架统一错误类型
package xerror

import (
	"errors"
	"fmt"
	"strings"
)

// XOneError 统一错误类型，包含模块名、操作名和原始错误
type XOneError struct {
	Module string // 模块名，如 "xconfig", "xgorm"
	Op     string // 操作名，如 "init", "close"
	Err    error  // 原始错误
}

// Error 实现 error 接口
func (e *XOneError) Error() string {
	var b strings.Builder
	b.Grow(32 + len(e.Module) + len(e.Op))
	b.WriteString("XOne ")
	b.WriteString(e.Module)
	b.WriteByte(' ')
	b.WriteString(e.Op)
	b.WriteString(" failed")
	if e.Err != nil {
		b.WriteString(", err=[")
		b.WriteString(e.Err.Error())
		b.WriteByte(']')
	}
	return b.String()
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
//
// 遍历整条链而不是只看最外层：模块之间会互相包装错误
// （xgorm 初始化失败里裹着 xconfig 的错误），只比对第一个 XOneError
// 会让 Is(err, "xconfig") 在这种链上返回 false，与本函数的语义不符
func Is(err error, module string) bool {
	for err != nil {
		var xe *XOneError
		if !errors.As(err, &xe) {
			return false
		}
		if xe.Module == module {
			return true
		}
		err = xe.Err // 从当前 XOneError 的内层继续找
	}
	return false
}

// Module 提取最外层 XOneError 的模块名，若非 XOneError 则返回空字符串
//
// 取最外层而非遍历整条链：错误一路向上包装，最外层代表「谁最终报出了这个错误」，
// 这正是调用方要分流处理的依据。要判断链中是否涉及某个模块，用 Is
func Module(err error) string {
	var xe *XOneError
	if errors.As(err, &xe) {
		return xe.Module
	}
	return ""
}
