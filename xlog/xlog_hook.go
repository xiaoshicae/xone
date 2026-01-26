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

	"github.com/xiaoshicae/xone/xutil"

	"github.com/sirupsen/logrus"
)

// 常量统一定义
const (
	// Context key
	timeFormatedCtxKey = "__time_formated__"

	// 控制台颜色
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
		return m.ConsolePrint(entry)
	}

	return nil
}

func (m *xLogHook) ConsolePrint(entry *logrus.Entry) error {
	line, err := entry.Bytes()
	if err != nil {
		return err
	}

	// 获取文件名和行号
	caller := m.ensureCaller(entry)
	_, fileName := callerPretty(caller)

	levelColor := getLogConsoleLogColor(entry.Level)
	levelText := strings.ToUpper(entry.Level.String())
	logTimeText := entry.Time.Format("2006-01-02 15:04:05.999")
	traceId := entry.Data["traceid"]
	panicStack := entry.Data["panic_stack"]

	// 预分配容量
	msg := make([]byte, 0, len(line)+128)

	// 打印原始json格式
	if m.ConsoleFormatIsRaw {
		prefix := fmt.Sprintf("\x1b[%dm%s\x1b[0m[%s] ", levelColor, levelText, logTimeText)
		msg = append([]byte(prefix), line...)
	} else {
		prefix := fmt.Sprintf("\x1b[%dm%s\x1b[0m[%s] \x1b[34m%s\x1b[0m %s ", levelColor, levelText, logTimeText, fileName, traceId)
		msg = append([]byte(prefix), []byte(entry.Message)...)
		msg = append(msg, '\n')
		if panicStack != nil {
			msg = append(msg, []byte(fmt.Sprintf("%v", panicStack))...)
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
// 由于可能会存在多个writer，每个write都会调用该Format，可能会导致重复处理，最终导致时间正确，因此需要修正
func (t timeFormatter) Format(e *logrus.Entry) ([]byte, error) {
	// 初始化
	if ctx := e.Context; ctx == nil {
		e.Context = context.Background()
	}

	// 如果已经format过了，则不用再次处理
	v, ok := e.Context.Value(timeFormatedCtxKey).(bool)
	if ok && v {
		return t.Formatter.Format(e)
	}

	// 转换到配置的时区
	if t.Location != nil {
		e.Time = e.Time.In(t.Location)
	}

	// 设置一下标记，防止多次处理
	e.Context = context.WithValue(e.Context, timeFormatedCtxKey, true)

	return t.Formatter.Format(e)
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

func callerPretty(f *runtime.Frame) (string, string) {
	if f == nil {
		return "???", "???"
	}
	funcVal := path.Base(f.Function)
	fileVal := fmt.Sprintf("%s:%d", path.Base(f.File), f.Line)
	return fmt.Sprintf("%s()", funcVal), fileVal
}

func getXLogContainerFromCtx(ctx context.Context) map[string]interface{} {
	kvContainer, ok := ctx.Value(XLogCtxKVContainerKey).(map[string]interface{})
	if !ok || kvContainer == nil {
		return nil
	}
	return kvContainer
}
