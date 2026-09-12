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

// consoleTimeLayout 控制台与 JSON 共用的时间格式
const consoleTimeLayout = "2006-01-02 15:04:05.999"

// consoleLineExtra 控制台行在日志内容之外的额外容量（颜色码、时间、文件名等）
const consoleLineExtra = 128

// xLogHook 承担字段补全与日志分发
//
// 设计说明：logrus 每条日志会无条件调用一次 Logger.Formatter 并写入 Logger.Out。
// 本模块将 Logger.Formatter 置为 nopFormatter、Out 置为 io.Discard，
// 由本 hook 自行持有 JSON 序列化器并按需序列化，确保每条日志最多只序列化一次。
type xLogHook struct {
	ServerName     string
	IP             string
	PidStr         string // 缓存 Pid 字符串，避免每条日志重复转换
	SuffixToIgnore []string

	// jsonFormatter JSON 序列化器，仅在确实需要 JSON 输出时调用
	jsonFormatter logrus.Formatter

	// location 日志时间所用时区，控制台与 JSON 共用，保证两者时间一致
	location *time.Location

	// consoleWriter 为 nil 表示不输出到控制台
	consoleWriter io.Writer
	consoleRaw    bool

	// fileWriter 为 nil 表示不写入日志文件
	fileWriter io.Writer
}

func (m *xLogHook) Levels() []logrus.Level {
	// 返回全部级别，实际过滤由 Logger.SetLevel 在上游完成，避免出现两套级别真相
	return logrus.AllLevels
}

func (m *xLogHook) Fire(entry *logrus.Entry) error {
	// 时区在此统一应用：entry 为每条日志独立副本，修改安全
	// 之所以不放在 Formatter 中，是为了让控制台输出与 JSON 输出的时间保持一致
	if m.location != nil {
		entry.Time = entry.Time.In(m.location)
	}

	if _, ok := entry.Data["servername"]; !ok {
		entry.Data["servername"] = m.ServerName
	}
	entry.Data["ip"] = m.IP
	entry.Data["pid"] = m.PidStr

	caller := m.ensureCaller(entry)
	if caller != nil {
		entry.Data["filename"] = path.Base(caller.File)
		entry.Data["lineid"] = strconv.Itoa(caller.Line)
	}

	entry.Data["traceid"] = xutil.GetTraceIDFromCtx(entry.Context)
	entry.Data["spanid"] = xutil.GetSpanIDFromCtx(entry.Context)

	for k, v := range getXLogContainerFromCtx(entry.Context) {
		entry.Data[k] = v
	}

	// 仅在确有 JSON 输出目标时才序列化，控制台使用可读格式时无需 JSON
	var jsonLine []byte
	if m.fileWriter != nil || (m.consoleWriter != nil && m.consoleRaw) {
		var err error
		if jsonLine, err = m.jsonFormatter.Format(entry); err != nil {
			return err
		}
	}

	var firstErr error
	if m.fileWriter != nil {
		if _, err := m.fileWriter.Write(jsonLine); err != nil {
			firstErr = err
		}
	}
	if m.consoleWriter != nil {
		// 控制台写入失败不应掩盖文件写入的错误，保留先发生的错误
		if err := m.writeConsole(entry, caller, jsonLine); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// writeConsole 输出到控制台，raw 模式直接复用已序列化的 JSON
func (m *xLogHook) writeConsole(entry *logrus.Entry, caller *runtime.Frame, jsonLine []byte) error {
	if m.consoleRaw {
		_, err := m.consoleWriter.Write(jsonLine)
		return err
	}

	msg := make([]byte, 0, len(entry.Message)+consoleLineExtra)
	msg = fmt.Appendf(msg, "\x1b[%dm%s\x1b[0m[%s] \x1b[34m%s\x1b[0m %s %s\n",
		getLogConsoleLogColor(entry.Level),
		strings.ToUpper(entry.Level.String()),
		entry.Time.Format(consoleTimeLayout),
		callerPretty(caller),
		entry.Data["traceid"],
		entry.Message,
	)
	if panicStack := entry.Data["panic_stack"]; panicStack != nil {
		msg = fmt.Appendf(msg, "%s\n", panicStack)
	}

	_, err := m.consoleWriter.Write(msg)
	return err
}

// ensureCaller 确保获取到调用者信息
func (m *xLogHook) ensureCaller(entry *logrus.Entry) *runtime.Frame {
	if entry.Caller != nil {
		return entry.Caller
	}
	return xutil.GetLogCaller(0, m.SuffixToIgnore)
}

// nopFormatter 空实现，用于抑制 logrus 写入 io.Discard 前那次无意义的序列化
type nopFormatter struct{}

func (nopFormatter) Format(*logrus.Entry) ([]byte, error) { return nil, nil }

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
	if ctx == nil {
		return nil
	}
	kvContainer, ok := ctx.Value(xLogCtxKVContainerKey).(map[string]any)
	if !ok || kvContainer == nil {
		return nil
	}
	return kvContainer
}
