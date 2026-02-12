package xlog

import (
	"context"
	"fmt"
	"io"
	"path"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/xiaoshicae/xone/v2/xutil"

	"github.com/sirupsen/logrus"
)

// 控制台颜色
const (
	colorRed    = 31
	colorYellow = 33
	colorBlue   = 36
	colorGray   = 37
)

type xLogHook struct {
	IP                 string
	ServerName         string
	PidStr             string // 缓存 Pid 字符串，避免重复转换
	SuffixToIgnore     []string
	Console            bool
	ConsoleFormatIsRaw bool
	Writer             io.Writer
}

func (m *xLogHook) Levels() []logrus.Level {
	return logrus.AllLevels
}

func (m *xLogHook) Fire(entry *logrus.Entry) error {
	if _, ok := entry.Data["servername"]; !ok {
		entry.Data["servername"] = m.ServerName
	}

	entry.Data["ip"] = m.IP
	entry.Data["pid"] = m.PidStr

	// 设置文件名和行号
	caller := m.ensureCaller(entry)
	entry.Data["filename"] = path.Base(caller.File)
	entry.Data["lineid"] = strconv.Itoa(caller.Line)

	// 设置trace信息
	entry.Data["traceid"] = xutil.GetTraceIDFromCtx(entry.Context)
	entry.Data["spanid"] = xutil.GetSpanIDFromCtx(entry.Context)

	// 设置一些ctx中kv
	for k, v := range getXLogContainerFromCtx(entry.Context) {
		entry.Data[k] = v
	}

	// 打印到控制台
	if m.Console {
		return m.consolePrint(entry, caller)
	}

	return nil
}

func (m *xLogHook) consolePrint(entry *logrus.Entry, caller *runtime.Frame) error {
	line, err := entry.Bytes()
	if err != nil {
		return err
	}

	fileName := callerPretty(caller)

	levelColor := getLogConsoleLogColor(entry.Level)
	levelText := strings.ToUpper(entry.Level.String())
	logTimeText := entry.Time.Format("2006-01-02 15:04:05.999")
	traceId := entry.Data["traceid"]
	panicStack := entry.Data["panic_stack"]

	// 预分配容量
	msg := make([]byte, 0, len(line)+128)

	// 打印原始json格式（纯 JSON，无前缀）
	if m.ConsoleFormatIsRaw {
		msg = append(msg, line...)
	} else {
		msg = fmt.Appendf(msg, "\x1b[%dm%s\x1b[0m[%s] \x1b[34m%s\x1b[0m %s %s\n", levelColor, levelText, logTimeText, fileName, traceId, entry.Message)
		if panicStack != nil {
			msg = fmt.Appendf(msg, "%s\n", panicStack)
		}
	}

	_, err = m.Writer.Write(msg)
	return err
}

// ensureCaller 确保获取到调用者信息，避免重复代码
func (m *xLogHook) ensureCaller(entry *logrus.Entry) *runtime.Frame {
	if entry.Caller != nil {
		return entry.Caller
	}
	return m.getCaller(0)
}

// getCaller retrieves the name of the first non-logrus calling function
func (m *xLogHook) getCaller(callDepth int) *runtime.Frame {
	return xutil.GetLogCaller(callDepth, m.SuffixToIgnore)
}

type timeFormatter struct {
	logrus.Formatter
	Location *time.Location
}

// Format 时间format
// 复制 entry 避免修改共享状态，保证多 writer 并发调用安全
// time.In() 是幂等操作，多次调用结果一致，无需标记防重复
func (t timeFormatter) Format(e *logrus.Entry) ([]byte, error) {
	entryCopy := *e
	if entryCopy.Context == nil {
		entryCopy.Context = context.Background()
	}
	if t.Location != nil {
		entryCopy.Time = entryCopy.Time.In(t.Location)
	}
	return t.Formatter.Format(&entryCopy)
}

func getLogConsoleLogColor(l logrus.Level) int {
	switch l {
	case logrus.DebugLevel, logrus.TraceLevel:
		return colorGray
	case logrus.WarnLevel:
		return colorYellow
	case logrus.ErrorLevel, logrus.FatalLevel, logrus.PanicLevel:
		return colorRed
	default:
		return colorBlue
	}
}

func callerPretty(f *runtime.Frame) string {
	if f == nil {
		return "???"
	}
	return fmt.Sprintf("%s:%d", path.Base(f.File), f.Line)
}

func getXLogContainerFromCtx(ctx context.Context) map[string]any {
	kvContainer, ok := ctx.Value(XLogCtxKVContainerKey).(map[string]any)
	if !ok || kvContainer == nil {
		return nil
	}
	return kvContainer
}
