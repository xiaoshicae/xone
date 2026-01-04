package xutil

import (
	"reflect"
	"runtime"
	"strings"
)

// IsSlice 是否为slice类型
func IsSlice(v interface{}) bool {
	if v == nil {
		return false
	}
	return reflect.TypeOf(v).Kind() == reflect.Slice
}

func GetFuncName(fc interface{}) (name string) {
	_, _, name = GetFuncInfo(fc)
	return name
}

func GetFuncInfo(fc interface{}) (file string, line int, name string) {
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

	pc := f.Pointer()
	if pc == 0 {
		return "", 0, ""
	}

	fn := runtime.FuncForPC(pc)
	if fn == nil {
		return "", 0, ""
	}

	fullName := fn.Name()
	if idx := strings.LastIndex(fullName, "/"); idx != -1 {
		fullName = fullName[idx+1:]
	}
	if idx := strings.Index(fullName, "."); idx == -1 {
		return "", 0, ""
	} else {
		name = fullName[idx+1:]
	}

	file, line = fn.FileLine(pc)
	return file, line, name
}
