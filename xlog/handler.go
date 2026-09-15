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

// recordAttrStackBuf 组装日志记录时栈上缓冲的字段数
//
// 比 RawLog 的 attrStackBuf 大：那里装的只是调用方的 KV，这里还要先放下
// 7 个固定字段（servername/ip/pid/filename/lineid/traceid/spanid）
const recordAttrStackBuf = 24

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
	serverName string
	ip         string
	pidStr     string // 缓存 Pid 字符串，避免每条日志重复转换

	// callerResolver 调用方解析器，按 PC 缓存解析结果
	callerResolver *xutil.CallerResolver

	// location 日志时间所用时区，控制台与 JSON 共用，保证两者时间一致
	location *time.Location

	// consoleWriter 为 nil 表示不输出到控制台
	consoleWriter io.Writer

	// consoleJSON 控制台是否输出 JSON 格式
	consoleJSON bool

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

	caller := h.caller()
	var fileName string
	var lineNo int
	if caller != nil {
		fileName = path.Base(caller.File)
		lineNo = caller.Line
	}

	traceID, spanID := xutil.GetTraceAndSpanIDFromCtx(ctx)

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
	if r.NumAttrs() > 0 {
		r.Attrs(func(a slog.Attr) bool {
			if a.Key == fieldServerName {
				serverName = a.Value.String() // 调用方显式指定的服务名优先
				return false
			}
			return true
		})
	}

	out := slog.NewRecord(r.Time, r.Level, r.Message, r.PC)

	// 先把全部字段收集到一个切片里，最后一次性 AddAttrs
	//
	// slog.Record 只内联前 5 个 attr，之后每调一次 AddAttrs 都要 slices.Grow
	// 一次底层切片。这里光固定字段就有 7 个，再逐个追加 ctx KV 与调用方 attr，
	// 一条请求日志能连续扩容十几次。RawLog 里已经是这么收集的，Handle 漏了。
	//
	// 起手用栈上定长数组，装得下就不碰堆；装不下由 append 接管，也只在
	// 越界那一次扩容。不预先 make：绝大多数日志只有固定字段，
	// 那会给它们平白加一次堆分配。
	var buf [recordAttrStackBuf]slog.Attr
	attrs := buf[:0]

	attrs = append(attrs,
		slog.String(fieldServerName, serverName),
		slog.String(fieldIP, h.ip),
		slog.String(fieldPid, h.pidStr),
	)
	if caller != nil {
		attrs = append(attrs,
			slog.String(fieldFilename, fileName),
			slog.String(fieldLineID, strconv.Itoa(lineNo)),
		)
	}
	attrs = append(attrs,
		slog.String(fieldTraceID, traceID),
		slog.String(fieldSpanID, spanID),
	)
	attrs = append(attrs, h.attrs...)

	rangeCtxKV(ctx, func(k string, v any) {
		if !isReservedField(k) {
			attrs = append(attrs, slog.Any(k, v))
		}
	})
	// 顺带取出 panic 栈，避免控制台输出时再遍历一次
	var panicStack string
	r.Attrs(func(a slog.Attr) bool {
		if a.Key == fieldPanicStack {
			panicStack = a.Value.String()
		}
		if !isReservedField(a.Key) {
			attrs = append(attrs, a)
		}
		return true
	})

	out.AddAttrs(attrs...)

	// 仅在确有 JSON 输出目标时才序列化，两个输出目标共用同一份结果
	var jsonLine []byte
	var jsonErr error
	if h.needJSON() {
		enc := acquireEncoder()
		defer releaseEncoder(enc)
		jsonLine, jsonErr = enc.encode(ctx, out)
	}

	return h.writeOutputs(out, caller, jsonLine, jsonErr, traceID, panicStack)
}

// needJSON 是否有输出目标需要 JSON 序列化
func (h *xHandler) needJSON() bool {
	return h.fileWriter != nil || (h.consoleWriter != nil && h.consoleJSON)
}

// writeOutputs 把日志写往文件与控制台，返回最先发生的错误
//
// jsonErr 是序列化阶段的错误：它只影响 JSON 输出，
// 控制台的可读格式仍应照常写出，所以不在这里提前返回
func (h *xHandler) writeOutputs(r slog.Record, caller *runtime.Frame, jsonLine []byte, jsonErr error, traceID, panicStack string) error {
	firstErr := jsonErr

	if h.fileWriter != nil && jsonLine != nil {
		if _, err := h.fileWriter.Write(jsonLine); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if h.consoleWriter != nil {
		// 控制台写入失败不应掩盖文件写入的错误，保留先发生的错误
		if err := h.writeConsole(r, caller, jsonLine, traceID, panicStack); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// writeConsole 输出到控制台
func (h *xHandler) writeConsole(r slog.Record, caller *runtime.Frame, jsonLine []byte, traceID, panicStack string) error {
	if h.consoleJSON {
		// JSON 格式直接复用已序列化的结果
		_, err := h.consoleWriter.Write(jsonLine)
		return err
	}

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

// caller 解析日志调用方
//
// 不使用 slog.Record 自带的 PC：它指向 xlog 内部的调用点，
// 需按忽略规则回溯才能定位到业务代码
func (h *xHandler) caller() *runtime.Frame {
	if h.callerResolver == nil {
		return nil
	}
	return h.callerResolver.Caller(0)
}

// lockedWriter 串行化写入
//
// 控制台多路输出共用一个 fd，单次 write 仅在小于管道缓冲区时才保证原子；
// panic 栈这类长内容会被拆成多次写入，并发下相互穿插。文件侧由 asyncWriter
// 的单消费协程天然串行，只有控制台需要显式加锁。
type lockedWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func newLockedWriter(w io.Writer) io.Writer {
	if w == nil {
		return nil
	}
	return &lockedWriter{w: w}
}

func (l *lockedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(p)
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

// encode 序列化一条记录，失败时返回 nil 行与错误
func (enc *jsonEncoder) encode(ctx context.Context, r slog.Record) ([]byte, error) {
	if err := enc.handler.Handle(ctx, r); err != nil {
		return nil, err
	}
	return enc.buf.Bytes(), nil
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
func replaceAttr(groups []string, a slog.Attr) slog.Attr {
	if len(groups) > 0 {
		return a // 仅改写顶层的 time/level，分组内的同名字段保持原样
	}
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
