package xutil

import (
	"fmt"
	"os"
	"path"
	"runtime"
	"strings"

	"github.com/sirupsen/logrus"
)

// 日志相关的方法，注意这里的日志主要用来做XOne debug用，因此只会打印在屏幕上

var logger *logrus.Logger

func init() {
	initLogger()
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
// 该函数位于日志热路径，匹配逻辑使用字符串比较而非正则，避免每帧回溯开销
func GetLogCaller(callDepth int, suffixToIgnore []string) *runtime.Frame {
	var pcs [maximumCallerDepth]uintptr
	depth := runtime.Callers(minimumCallerDepth+callDepth, pcs[:])
	if depth == 0 {
		return nil
	}
	frames := runtime.CallersFrames(pcs[:depth])
	for {
		f, hasMore := frames.Next()
		if !shouldIgnoreCallerFile(f.File, suffixToIgnore) {
			return &f
		}
		if !hasMore {
			// 所有栈帧都被忽略，返回最后一帧兜底，避免调用方拿到 nil
			return &f
		}
	}
}

// shouldIgnoreCallerFile 判断栈帧文件是否应被跳过
func shouldIgnoreCallerFile(file string, suffixToIgnore []string) bool {
	for _, s := range suffixToIgnore {
		if strings.HasSuffix(file, s) {
			return true
		}
	}
	base := path.Base(file)
	for i := range callerIgnoreRules {
		r := &callerIgnoreRules[i]
		if r.pkg != "" && !strings.Contains(file, r.pkg) {
			continue
		}
		for _, prefix := range r.filePrefixes {
			if strings.HasPrefix(base, prefix) {
				return true
			}
		}
	}
	return false
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

// callerIgnoreRule 调用栈忽略规则
// pkg 为空表示不限定路径，仅按文件名前缀匹配
type callerIgnoreRule struct {
	pkg          string   // 栈帧文件路径中需包含的库标识
	filePrefixes []string // 文件名（basename）前缀，命中任一即忽略
}

// callerIgnoreRules 需要从调用栈中过滤的第三方库文件
// 原为正则列表，因位于日志热路径改为字符串匹配：栈深 15 层时正则需回溯近百次
var callerIgnoreRules = []callerIgnoreRule{
	{pkg: "go-redis/", filePrefixes: []string{"string_commands.go", "redis.go"}},
	{pkg: "xmysql", filePrefixes: []string{"logger.go"}},
	{pkg: "xredis", filePrefixes: []string{"logger.go"}},
	{pkg: "logrus", filePrefixes: []string{"hooks.go", "entry.go", "logger.go", "exported.go"}},
	{pkg: "gorm", filePrefixes: []string{"callbacks.go", "finisher_api.go"}},
	{pkg: "mongo-driver", filePrefixes: []string{"operation", "database", "client", "collection", "cursor"}},
	{pkg: "", filePrefixes: []string{"asm_"}},
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
