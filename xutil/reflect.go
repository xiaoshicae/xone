package xutil

import (
	"reflect"
	"runtime"
	"strings"
)

// IsSlice 是否为slice类型
func IsSlice(v any) bool {
	if v == nil {
		return false
	}
	return reflect.TypeOf(v).Kind() == reflect.Slice
}

// GetFuncName 获取函数名称，传入 nil 或非函数类型时返回空字符串
func GetFuncName(fc any) (name string) {
	_, _, name = GetFuncInfo(fc)
	return name
}

// GetFuncInfo 获取函数的源文件路径、行号和名称
// 传入 nil 或非函数类型时返回零值
func GetFuncInfo(fc any) (file string, line int, name string) {
	if fc == nil {
		return "", 0, ""
	}
	f := reflect.ValueOf(fc)
	if f.Kind() != reflect.Func {
		return "", 0, ""
	}
	if f.IsNil() {
		return "", 0, ""
	}

	fn := runtime.FuncForPC(f.Pointer())
	if fn == nil {
		return "", 0, ""
	}

	fullName := fn.Name()
	if idx := strings.LastIndex(fullName, "/"); idx != -1 {
		fullName = fullName[idx+1:]
	}
	_, after, found := strings.Cut(fullName, ".")
	if !found {
		return "", 0, ""
	}
	name = after

	file, line = fn.FileLine(f.Pointer())
	return file, line, name
}
