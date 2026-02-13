package xutil

import (
	"fmt"
	"os"
	"path"
	"regexp"
	"runtime"
	"strings"

	"github.com/sirupsen/logrus"
)

// 日志相关的方法，注意这里的日志主要用来做XOne debug用，因此只会打印在屏幕上

var logger *logrus.Logger

func init() {
	initLogger()
	initCallerIgnoreRegList()
}

// ErrorIfEnableDebug 当开启 debug 模式时输出 Error 级别日志
func ErrorIfEnableDebug(msg string, args ...any) {
	LogIfEnableDebug(logrus.ErrorLevel, msg, args...)
}

// InfoIfEnableDebug 当开启 debug 模式时输出 Info 级别日志
func InfoIfEnableDebug(msg string, args ...any) {
	LogIfEnableDebug(logrus.InfoLevel, msg, args...)
}

// WarnIfEnableDebug 当开启 debug 模式时输出 Warn 级别日志
func WarnIfEnableDebug(msg string, args ...any) {
	LogIfEnableDebug(logrus.WarnLevel, msg, args...)
}

// xoneDebugPrefix XOne 框架调试日志的醒目前缀（紫色高亮）
const xoneDebugPrefix = "\x1b[35m[XOne-Debug]\x1b[0m "

// LogIfEnableDebug 当开启 debug 模式时按指定级别输出日志
func LogIfEnableDebug(level logrus.Level, msg string, args ...any) {
	if EnableXOneDebug() {
		logger.Logf(level, xoneDebugPrefix+msg, args...)
	}
}

// GetLogCaller 获取日志调用方的栈帧，跳过 suffixToIgnore 和内置忽略列表中匹配的文件
func GetLogCaller(callDepth int, suffixToIgnore []string) (frame *runtime.Frame) {
	pcs := make([]uintptr, maximumCallerDepth)
	depth := runtime.Callers(minimumCallerDepth+callDepth, pcs)
	frames := runtime.CallersFrames(pcs[:depth])
OUTER:
	for f, hasMore := frames.Next(); hasMore; f, hasMore = frames.Next() {
		frame = &f

		// 跳过匹配忽略列表的调用帧
		for _, s := range suffixToIgnore {
			if strings.HasSuffix(f.File, s) {
				continue OUTER
			}
		}
		for _, r := range callerIgnoreRegList {
			if r.MatchString(f.File) {
				continue OUTER
			}
		}
		break
	}

	return
}

const (
	currentFilePath        = "/xutil/log.go"
	unknownCaller          = "???"
	maximumCallerDepth int = 25
	minimumCallerDepth int = 5 // logrus.entry.go:237
)

func callerPretty(_ *runtime.Frame) (string, string) {
	frame := GetLogCaller(0, []string{currentFilePath})
	if frame == nil {
		return unknownCaller, unknownCaller
	}
	fName := path.Base(frame.File)
	if fName == "" {
		fName = unknownCaller
	}
	return "", fmt.Sprintf(" \x1b[34m%s:%d\x1b[0m", fName, frame.Line)
}

// callerIgnoreRegList 预编译的调用栈忽略正则列表
var callerIgnoreRegList []*regexp.Regexp

// callerIgnorePatterns 需要从调用栈中过滤的第三方库文件模式
// 同一库的多个文件合并为一个正则，减少匹配次数
var callerIgnorePatterns = []string{
	`go-redis/(.*)/(?:string_commands|redis)\.go`,
	`(?:xmysql|xredis)(|@v.*)/logger\.go`,
	`logrus(|@v.*)/(?:hooks|entry|logger|exported)\.go`,
	`gorm(|@v.*)/(?:callbacks|finisher_api)\.go`,
	`mongo-driver(|@v.*)/(?:operation|database|client|collection|cursor).*\.go`,
	`asm_\w+\.s`,
}

func initLogger() {
	l := logrus.New()
	l.Formatter = &logrus.TextFormatter{
		ForceColors:      true,
		FullTimestamp:    true,
		TimestampFormat:  "2006-01-02 15:04:05.999",
		CallerPrettyfier: callerPretty,
	}
	l.SetReportCaller(true)
	l.SetLevel(logrus.InfoLevel)
	l.SetOutput(os.Stdout)
	logger = l
}

// initCallerIgnoreRegList 预编译调用栈忽略正则
func initCallerIgnoreRegList() {
	callerIgnoreRegList = make([]*regexp.Regexp, len(callerIgnorePatterns))
	for i, pattern := range callerIgnorePatterns {
		callerIgnoreRegList[i] = regexp.MustCompile(pattern)
	}
}
