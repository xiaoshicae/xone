package xutil

import (
	"strings"
	"time"

	"github.com/spf13/cast"
)

// ToPtr 获取值的指针
func ToPtr[T any](t T) *T {
	return &t
}

// GetOrDefault 如果v为零值，则返回defaultV（无反射，零分配）
func GetOrDefault[T comparable](v T, defaultV T) T {
	var zero T
	if v == zero {
		return defaultV
	}
	return v
}

// ToDuration 兼容d类型时长，如"1d"
func ToDuration(i any) time.Duration {
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
	if !strings.Contains(duration, "d") {
		return cast.ToDuration(duration)
	}

	day, left, _ := strings.Cut(duration, "d")
	dayDuration, err := cast.ToIntE(day)
	if err != nil {
		// 天数解析失败时记录日志，尝试解析剩余部分
		ErrorIfEnableDebug("strToDuration parse day failed, day=[%s], err=[%v], fallback to parse left=[%s]", day, err, left)
		return cast.ToDuration(left)
	}
	return time.Duration(dayDuration)*24*time.Hour + cast.ToDuration(left)
}
