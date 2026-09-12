package xlog

import (
	"strings"

	"github.com/sirupsen/logrus"
)

// Level 日志级别
// 该类型与底层日志库解耦，使公开 API 不暴露具体实现，便于后续替换日志后端
type Level uint32

// 日志级别，数值越小级别越高，与常见日志库保持一致的排列顺序
const (
	PanicLevel Level = iota
	FatalLevel
	ErrorLevel
	WarnLevel
	InfoLevel
	DebugLevel
	TraceLevel
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

// String 返回级别名称，未知级别返回 "unknown"
func (l Level) String() string {
	if int(l) >= len(levelNames) {
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

// toLogrus 转换为底层日志库的级别
// Level 的取值顺序与 logrus 一致，可直接转换
func (l Level) toLogrus() logrus.Level {
	if !l.IsValid() {
		return logrus.InfoLevel
	}
	return logrus.Level(l)
}

// fromLogrusLevel 由底层日志库级别转换而来，供旁路扩展点使用
func fromLogrusLevel(l logrus.Level) Level {
	if int(l) >= len(levelNames) {
		return InfoLevel
	}
	return Level(l)
}
