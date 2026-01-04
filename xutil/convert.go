package xutil

import (
	"reflect"
	"strings"
	"time"

	"github.com/spf13/cast"
)

// ToPrt 获取指针
func ToPrt[T any](t T) *T {
	return &t
}

// GetOrDefault 如果v为0值，则返回defaultV
func GetOrDefault[T any](v T, defaultV T) T {
	rv := reflect.ValueOf(v)
	if !rv.IsValid() {
		return defaultV
	}
	if rv.IsZero() {
		return defaultV
	}
	return v
}

// ToDuration 兼容d类型时长，如"1d"
func ToDuration(i interface{}) time.Duration {
	if i == nil {
		return 0
	}

	if duration, ok := i.(string); ok {
		return strToDuration(duration)
	}

	if duration, ok := i.(*string); ok && duration != nil {
		return strToDuration(*duration)
	}

	return cast.ToDuration(i)
}

func strToDuration(duration string) time.Duration {
	if strings.Contains(duration, "d") {
		day, left, _ := strings.Cut(duration, "d")
		dayDuration, _ := cast.ToIntE(day)
		return time.Duration(dayDuration)*24*time.Hour + cast.ToDuration(left)
	}
	return cast.ToDuration(duration)
}
