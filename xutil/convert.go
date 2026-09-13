package xutil

import (
	"strconv"
	"strings"
	"time"
)

// durationUnitChars time.ParseDuration 支持的单位字符
//
// 不含这些字符的纯数字按纳秒处理，与此前依赖的 cast.ToDuration 行为一致，
// 例如 "5" 等价于 "5ns"。
const durationUnitChars = "nsuµmh"

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

// ToDuration 解析时长，在 time.ParseDuration 之上额外支持天（如 "1d"、"2d12h"）
//
// 接受 string / *string / time.Duration / 整数与浮点数（按纳秒计），
// 其余类型或解析失败一律返回 0。
func ToDuration(i any) time.Duration {
	switch v := i.(type) {
	case nil:
		return 0
	case string:
		return strToDuration(v)
	case *string:
		if v == nil {
			return 0
		}
		return strToDuration(*v)
	case time.Duration:
		return v
	case int:
		return time.Duration(v)
	case int8:
		return time.Duration(v)
	case int16:
		return time.Duration(v)
	case int32:
		return time.Duration(v)
	case int64:
		return time.Duration(v)
	case uint:
		return time.Duration(v)
	case uint8:
		return time.Duration(v)
	case uint16:
		return time.Duration(v)
	case uint32:
		return time.Duration(v)
	case uint64:
		return time.Duration(v)
	case float32:
		return time.Duration(v)
	case float64:
		return time.Duration(v)
	default:
		return 0
	}
}

// strToDuration 解析字符串时长，兼容 "1d" 这类带天的写法
func strToDuration(duration string) time.Duration {
	if !strings.Contains(duration, "d") {
		return parseDuration(duration)
	}

	day, left, _ := strings.Cut(duration, "d")
	dayCount, err := strconv.Atoi(day)
	if err != nil {
		// 天数解析失败时记录日志，尝试解析剩余部分
		ErrorIfEnableDebug("strToDuration parse day failed, day=[%s], err=[%v], fallback to parse left=[%s]", day, err, left)
		return parseDuration(left)
	}
	return time.Duration(dayCount)*24*time.Hour + parseDuration(left)
}

// parseDuration 解析不含天的时长，无单位的纯数字按纳秒处理，解析失败返回 0
func parseDuration(duration string) time.Duration {
	if !strings.ContainsAny(duration, durationUnitChars) {
		duration += "ns"
	}
	d, err := time.ParseDuration(duration)
	if err != nil {
		return 0
	}
	return d
}
