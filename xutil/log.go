package xutil

import (
	"fmt"
	"os"
	"path"
	"regexp"
	"runtime"
	"strings"
	"sync"

	"github.com/sirupsen/logrus"
)

// 日志相关的方法，注意这里的日志主要用来做XOne debug用，因此只会打印在屏幕上

var (
	logger     *logrus.Logger
	initOnce   sync.Once // 确保初始化只执行一次
	regexOnce  sync.Once // 确保正则编译只执行一次
)

func init() {
	initOnce.Do(initLogger)
	regexOnce.Do(initCallerIgnoreFileName)
}

func ErrorIfEnableDebug(msg string, args ...interface{}) {
	LogIfEnableDebug(logrus.ErrorLevel, msg, args...)
}

func InfoIfEnableDebug(msg string, args ...interface{}) {
	LogIfEnableDebug(logrus.InfoLevel, msg, args...)
}

func WarnIfEnableDebug(msg string, args ...interface{}) {
	LogIfEnableDebug(logrus.WarnLevel, msg, args...)
}

func LogIfEnableDebug(level logrus.Level, msg string, args ...interface{}) {
	if EnableDebug() {
		logger.Logf(level, msg, args...)
	}
}

func GetLogCaller(callDepth int, suffixToIgnore []string) (frame *runtime.Frame) {
	pcs := make([]uintptr, maximumCallerDepth)
	depth := runtime.Callers(minimumCallerDepth+callDepth, pcs)
	frames := runtime.CallersFrames(pcs[:depth])
OUTER:
	for f, hasMore := frames.Next(); hasMore; f, hasMore = frames.Next() {
		frame = &f

		// If the caller isn't part of this package, we're done
		for _, s := range suffixToIgnore {
			if strings.HasSuffix(f.File, s) {
				continue OUTER
			}
		}
		for _, s := range defaultSuffixedRegList {
			if s.MatchString(f.File) {
				continue OUTER
			}
		}
		break
	}

	return
}

const (
	currentFilePath        = "/xutil/log.go"
	maximumCallerDepth int = 25
	minimumCallerDepth int = 5 // should be logrus.entry.go:237
)

func callerPretty(_ *runtime.Frame) (string, string) {
	frame := GetLogCaller(0, []string{currentFilePath})
	if frame == nil {
		return "???", "???"
	}
	// funcVal := path.Base(frame.Function)
	fName := path.Base(frame.File)
	if fName == "" {
		fName = "???"
	}
	fileName := fmt.Sprintf("%s:%d", fName, frame.Line)
	coloredFileName := fmt.Sprintf(" \x1b[34m%s\x1b[0m", fileName)
	return "", coloredFileName
}

var (
	defaultSuffixedRegList        []*regexp.Regexp
	defaultSuffixesRegPatternList = []string{
		`go-redis/(.*)/string_commands\.go`,
		`go-redis/(.*)/redis\.go`,
		`xmysql(|@v.*)/logger\.go`,
		`xredis(|@v.*)/logger\.go`,
		`logrus(|@v.*)/hooks\.go`,
		`logrus(|@v.*)/entry\.go`,
		`logrus(|@v.*)/logger\.go`,
		`logrus(|@v.*)/exported\.go`,
		`gorm(|@v.*)/callbacks\.go`,
		`gorm(|@v.*)/finisher_api\.go`,
		`mongo-driver(|@v.*)/operation.*go$`,
		`mongo-driver(|@v.*)/database\.go`,
		`mongo-driver(|@v.*)/client\.go`,
		`mongo-driver(|@v.*)/collection\.go`,
		`mongo-driver(|@v.*)/cursor\.go`,
		`asm_amd64\.s`,
	}
)

func initLogger() {
	l := logrus.New()
	l.Formatter = &logrus.TextFormatter{
		ForceColors:      true,                      // 强制输出颜色
		FullTimestamp:    true,                      // 完整的时间戳
		TimestampFormat:  "2006-01-02 15:04:05.999", // 自定义时间戳格式
		CallerPrettyfier: callerPretty,              // 自定义文件名和行号格式
	}
	l.SetReportCaller(true)
	l.SetLevel(logrus.InfoLevel)
	l.SetOutput(os.Stdout)
	logger = l
}

// initCallerIgnoreFileName 初始化获取caller忽略的文件名
func initCallerIgnoreFileName() {
	defaultSuffixedRegList = make([]*regexp.Regexp, 0, len(defaultSuffixesRegPatternList))
	for _, pattern := range defaultSuffixesRegPatternList {
		defaultSuffixedRegList = append(defaultSuffixedRegList, regexp.MustCompile(pattern))
	}
}
