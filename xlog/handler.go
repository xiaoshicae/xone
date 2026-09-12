package xlog

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"path"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/xiaoshicae/xone/v2/xutil"
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

// 日志固定字段名
const (
	fieldServerName = "servername"
	fieldIP         = "ip"
	fieldPid        = "pid"
	fieldFilename   = "filename"
	fieldLineID     = "lineid"
	fieldTraceID    = "traceid"
	fieldSpanID     = "spanid"
	fieldPanicStack = "panic_stack"
)

// xHandler 实现 slog.Handler，负责字段补全、旁路通知与日志分发
//
// 每条日志最多序列化一次：仅当需要写文件或控制台使用原始 JSON 格式时才序列化，
// 两个输出目标共用同一份结果。
type xHandler struct {
	serverName     string
	ip             string
	pidStr         string // 缓存 Pid 字符串，避免每条日志重复转换
	suffixToIgnore []string

	// location 日志时间所用时区，控制台与 JSON 共用，保证两者时间一致
	location *time.Location

	// consoleWriter 为 nil 表示不输出到控制台
	consoleWriter io.Writer
	consoleRaw    bool

	// fileWriter 为 nil 表示不写入日志文件
	fileWriter io.Writer

	level slog.Level

	// attrs 由 WithAttrs 累积，附加到每条日志
	attrs []slog.Attr
}

func (h *xHandler) Enabled(_ context.Context, l slog.Level) bool {
	return l >= h.level
}

func (h *xHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(attrs) == 0 {
		return h
	}
	next := *h
	next.attrs = make([]slog.Attr, 0, len(h.attrs)+len(attrs))
	next.attrs = append(next.attrs, h.attrs...)
	next.attrs = append(next.attrs, attrs...)
	return &next
}

// WithGroup 本模块的日志为扁平结构，分组会改变既有字段布局，故不支持
func (h *xHandler) WithGroup(string) slog.Handler {
	return h
}

func (h *xHandler) Handle(ctx context.Context, r slog.Record) error {
	// 时区在此统一应用，保证控制台输出与 JSON 输出的时间一致
	if h.location != nil {
		r.Time = r.Time.In(h.location)
	}

	caller := h.resolveCaller(r.PC)
	var fileName string
	var lineNo int
	if caller != nil {
		fileName = path.Base(caller.File)
		lineNo = caller.Line
	}

	traceID := xutil.GetTraceIDFromCtx(ctx)
	spanID := xutil.GetSpanIDFromCtx(ctx)

	// 先通知旁路观察者，确保输出失败时 metric 等旁路能力仍然生效
	notifyObservers(ctx, Record{
		Level:   fromSlogLevel(r.Level),
		Message: r.Message,
		File:    fileName,
		Line:    lineNo,
		TraceID: traceID,
		SpanID:  spanID,
	})

	// 重新组装记录：固定字段在前，其余字段去重后附加
	//
	// slog 的 attrs 是列表而非 map，同名字段会在 JSON 中重复出现；
	// 这里显式去重，保持与既有输出一致的「框架字段覆盖同名自定义字段」语义。
	serverName := h.serverName
	r.Attrs(func(a slog.Attr) bool {
		if a.Key == fieldServerName {
			serverName = a.Value.String() // 调用方显式指定的服务名优先
		}
		return true
	})

	out := slog.NewRecord(r.Time, r.Level, r.Message, r.PC)
	out.AddAttrs(
		slog.String(fieldServerName, serverName),
		slog.String(fieldIP, h.ip),
		slog.String(fieldPid, h.pidStr),
	)
	if caller != nil {
		out.AddAttrs(
			slog.String(fieldFilename, fileName),
			slog.String(fieldLineID, strconv.Itoa(lineNo)),
		)
	}
	out.AddAttrs(
		slog.String(fieldTraceID, traceID),
		slog.String(fieldSpanID, spanID),
	)
	out.AddAttrs(h.attrs...)

	for k, v := range getXLogContainerFromCtx(ctx) {
		if !isReservedField(k) {
			out.AddAttrs(slog.Any(k, v))
		}
	}
	r.Attrs(func(a slog.Attr) bool {
		if !isReservedField(a.Key) {
			out.AddAttrs(a)
		}
		return true
	})
	r = out

	// 仅在确有 JSON 输出目标时才序列化
	var jsonLine []byte
	var enc *jsonEncoder
	if h.fileWriter != nil || (h.consoleWriter != nil && h.consoleRaw) {
		enc = acquireEncoder()
		defer releaseEncoder(enc)

		if err := enc.handler.Handle(ctx, r); err != nil {
			return err
		}
		jsonLine = enc.buf.Bytes()
	}

	var firstErr error
	if h.fileWriter != nil {
		if _, err := h.fileWriter.Write(jsonLine); err != nil {
			firstErr = err
		}
	}
	if h.consoleWriter != nil {
		// 控制台写入失败不应掩盖文件写入的错误，保留先发生的错误
		if err := h.writeConsole(r, caller, jsonLine); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// writeConsole 输出到控制台，raw 模式直接复用已序列化的 JSON
func (h *xHandler) writeConsole(r slog.Record, caller *runtime.Frame, jsonLine []byte) error {
	if h.consoleRaw {
		_, err := h.consoleWriter.Write(jsonLine)
		return err
	}

	var traceID, panicStack string
	r.Attrs(func(a slog.Attr) bool {
		switch a.Key {
		case fieldTraceID:
			traceID = a.Value.String()
		case fieldPanicStack:
			panicStack = a.Value.String()
		}
		return true
	})

	msg := make([]byte, 0, len(r.Message)+consoleLineExtra)
	msg = fmt.Appendf(msg, "\x1b[%dm%s\x1b[0m[%s] \x1b[34m%s\x1b[0m %s %s\n",
		levelColor(r.Level),
		strings.ToUpper(fromSlogLevel(r.Level).String()),
		r.Time.Format(consoleTimeLayout),
		callerPretty(caller),
		traceID,
		r.Message,
	)
	if panicStack != "" {
		msg = fmt.Appendf(msg, "%s\n", panicStack)
	}

	_, err := h.consoleWriter.Write(msg)
	return err
}

// resolveCaller 解析日志调用方
// slog.Record 自带的 PC 指向 xlog 内部，需按忽略规则回溯到业务代码
func (h *xHandler) resolveCaller(uintptr) *runtime.Frame {
	return xutil.GetLogCaller(0, h.suffixToIgnore)
}

// isReservedField 判断字段名是否为框架固定字段
func isReservedField(k string) bool {
	switch k {
	case fieldServerName, fieldIP, fieldPid, fieldFilename, fieldLineID, fieldTraceID, fieldSpanID:
		return true
	default:
		return false
	}
}

// jsonEncoder 复用的 JSON 序列化器
// slog.JSONHandler 在构造时绑定 writer，故 buffer 与 handler 需成对复用
type jsonEncoder struct {
	buf     *bytes.Buffer
	handler slog.Handler
}

var encoderPool = sync.Pool{
	New: func() any {
		buf := &bytes.Buffer{}
		return &jsonEncoder{
			buf: buf,
			handler: slog.NewJSONHandler(buf, &slog.HandlerOptions{
				Level:       slogLevelTrace,
				ReplaceAttr: replaceAttr,
			}),
		}
	},
}

func acquireEncoder() *jsonEncoder {
	enc := encoderPool.Get().(*jsonEncoder)
	enc.buf.Reset()
	return enc
}

func releaseEncoder(enc *jsonEncoder) {
	if enc.buf.Cap() > maxPoolBufSize {
		return // 过大的 buffer 不归还，避免 pool 长期占用内存
	}
	encoderPool.Put(enc)
}

// replaceAttr 将 slog 默认的时间与级别表示改为本模块的既有格式
func replaceAttr(_ []string, a slog.Attr) slog.Attr {
	switch a.Key {
	case slog.TimeKey:
		return slog.String(slog.TimeKey, a.Value.Time().Format(consoleTimeLayout))
	case slog.LevelKey:
		lv, ok := a.Value.Any().(slog.Level)
		if !ok {
			return a
		}
		return slog.String(slog.LevelKey, fromSlogLevel(lv).String())
	default:
		return a
	}
}

func levelColor(l slog.Level) int {
	switch {
	case l <= slog.LevelDebug:
		return colorGray
	case l >= slog.LevelError:
		return colorRed
	case l >= slog.LevelWarn:
		return colorYellow
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
