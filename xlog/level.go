package xlog

import (
	"log/slog"
	"strings"
)

// Level 日志级别
// 该类型与底层日志库解耦，使公开 API 不暴露具体实现，便于替换日志后端
type Level uint32

// 日志级别，数值越小级别越高
const (
	PanicLevel Level = iota
	FatalLevel
	ErrorLevel
	WarnLevel
	InfoLevel
	DebugLevel
	TraceLevel
)

// slog 未定义 Trace/Fatal/Panic，按其 4 级步长向两端扩展
const (
	slogLevelTrace = slog.LevelDebug - 4
	slogLevelFatal = slog.LevelError + 4
	slogLevelPanic = slog.LevelError + 8
)

// levelNames 级别到名称的映射，索引即级别值
var levelNames = [...]string{
	PanicLevel: "panic",
	FatalLevel: "fatal",
	ErrorLevel: "error",
	WarnLevel:  "warn",
	InfoLevel:  "info",
	DebugLevel: "debug",
	TraceLevel: "trace",
}

// levelToSlog 本模块级别到 slog 级别的映射
// 两者方向相反（本模块数值越小级别越高，slog 相反），故用表而非算术转换
var levelToSlog = [...]slog.Level{
	PanicLevel: slogLevelPanic,
	FatalLevel: slogLevelFatal,
	ErrorLevel: slog.LevelError,
	WarnLevel:  slog.LevelWarn,
	InfoLevel:  slog.LevelInfo,
	DebugLevel: slog.LevelDebug,
	TraceLevel: slogLevelTrace,
}

// String 返回级别名称，未知级别返回 "unknown"
func (l Level) String() string {
	if !l.IsValid() {
		return "unknown"
	}
	return levelNames[l]
}

// IsValid 判断是否为合法级别
func (l Level) IsValid() bool {
	return int(l) < len(levelNames)
}

// ParseLevel 解析级别名称，大小写不敏感
// 同时兼容 "warning" 这一常见别名；无法识别时返回 InfoLevel 和 false
func ParseLevel(s string) (Level, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "panic":
		return PanicLevel, true
	case "fatal":
		return FatalLevel, true
	case "error":
		return ErrorLevel, true
	case "warn", "warning":
		return WarnLevel, true
	case "info":
		return InfoLevel, true
	case "debug":
		return DebugLevel, true
	case "trace":
		return TraceLevel, true
	default:
		return InfoLevel, false
	}
}

// toSlog 转换为 slog 级别
func (l Level) toSlog() slog.Level {
	if !l.IsValid() {
		return slog.LevelInfo
	}
	return levelToSlog[l]
}

// fromSlogLevel 由 slog 级别转换而来，供旁路扩展点使用
// 取最接近且不高于入参的已知级别，使自定义级别也能归类
func fromSlogLevel(l slog.Level) Level {
	switch {
	case l >= slogLevelPanic:
		return PanicLevel
	case l >= slogLevelFatal:
		return FatalLevel
	case l >= slog.LevelError:
		return ErrorLevel
	case l >= slog.LevelWarn:
		return WarnLevel
	case l >= slog.LevelInfo:
		return InfoLevel
	case l >= slog.LevelDebug:
		return DebugLevel
	default:
		return TraceLevel
	}
}
