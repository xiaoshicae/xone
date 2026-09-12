package xlog

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/xiaoshicae/xone/v2/xconfig"
	"github.com/xiaoshicae/xone/v2/xutil"

	"github.com/bytedance/mockey"
	c "github.com/smartystreets/goconvey/convey"
)

func TestXLogConfig(t *testing.T) {
	mockey.PatchConvey("TestXLogConfig-configMergeDefault-Nil", t, func() {
		config := configMergeDefault(nil)
		c.So(config, c.ShouldResemble, &Config{
			Level:              "info",
			EnableFile:         false,
			Name:               "app",
			Path:               "./log",
			EnableConsole:      xutil.ToPtr(true),
			ConsoleFormatIsRaw: false,
			MaxAge:             "7d",
			RotateTime:         "1d",
			Timezone:           "Asia/Shanghai",
		})
	})

	mockey.PatchConvey("TestXLogConfig-configMergeDefault-NotNil", t, func() {
		mockey.Mock(xconfig.GetServerName).Return("a.b.c").Build()
		config := &Config{
			Level:              "1",
			EnableFile:         true,
			Name:               "2",
			Path:               "3",
			EnableConsole:      xutil.ToPtr(true),
			ConsoleFormatIsRaw: true,
			MaxAge:             "4",
			RotateTime:         "5",
			Timezone:           "UTC",
		}
		config = configMergeDefault(config)
		c.So(config, c.ShouldResemble, &Config{
			Level:              "1",
			EnableFile:         true,
			Name:               "2",
			Path:               "3",
			EnableConsole:      xutil.ToPtr(true),
			ConsoleFormatIsRaw: true,
			MaxAge:             "4",
			RotateTime:         "5",
			Timezone:           "UTC",
		})
	})

	mockey.PatchConvey("TestXLogConfig-configMergeDefault-DefaultIsConsoleOnly", t, func() {
		// 默认不写文件，仅打印到控制台
		config := configMergeDefault(&Config{})
		c.So(config.EnableFile, c.ShouldBeFalse)
		c.So(*config.EnableConsole, c.ShouldBeTrue)
	})

	mockey.PatchConvey("TestXLogConfig-configMergeDefault-FileOnly", t, func() {
		// 开启文件写入后，显式关闭的 EnableConsole 不会被强制打开
		config := configMergeDefault(&Config{EnableFile: true, EnableConsole: xutil.ToPtr(false)})
		c.So(config.EnableFile, c.ShouldBeTrue)
		c.So(*config.EnableConsole, c.ShouldBeFalse)
	})

	mockey.PatchConvey("TestXLogConfig-configMergeDefault-BothDisabledForceConsole", t, func() {
		// 文件和控制台都关闭时强制打开控制台，避免日志无处输出
		config := configMergeDefault(&Config{EnableFile: false, EnableConsole: xutil.ToPtr(false)})
		c.So(*config.EnableConsole, c.ShouldBeTrue)
	})

	mockey.PatchConvey("TestXLogConfig-configMergeDefault-Idempotent", t, func() {
		// 重复合并结果一致，initXLogByConfig 的兜底调用依赖该性质
		once := configMergeDefault(&Config{Level: "debug", EnableFile: true, EnableConsole: xutil.ToPtr(false)})
		twice := configMergeDefault(once)
		c.So(twice, c.ShouldResemble, once)
	})

	mockey.PatchConvey("TestXLogConfig-configMergeDefault-ForceConsoleKeepFormat", t, func() {
		// 强制打开 EnableConsole 不影响用户配置的输出格式
		config := configMergeDefault(&Config{EnableConsole: xutil.ToPtr(false), ConsoleFormatIsRaw: true})
		c.So(*config.EnableConsole, c.ShouldBeTrue)
		c.So(config.ConsoleFormatIsRaw, c.ShouldBeTrue)
	})
}

func TestLevel(t *testing.T) {
	mockey.PatchConvey("TestLevel-ParseLevel", t, func() {
		cases := []struct {
			in    string
			want  Level
			valid bool
		}{
			{"debug", DebugLevel, true},
			{"INFO", InfoLevel, true},
			{" warn ", WarnLevel, true},
			{"warning", WarnLevel, true}, // 兼容 logrus 的别名
			{"error", ErrorLevel, true},
			{"fatal", FatalLevel, true},
			{"panic", PanicLevel, true},
			{"trace", TraceLevel, true},
			{"unknown", InfoLevel, false},
			{"", InfoLevel, false},
		}
		for _, tc := range cases {
			got, ok := ParseLevel(tc.in)
			c.So(got, c.ShouldEqual, tc.want)
			c.So(ok, c.ShouldEqual, tc.valid)
		}
	})

	mockey.PatchConvey("TestLevel-String", t, func() {
		c.So(InfoLevel.String(), c.ShouldEqual, "info")
		c.So(PanicLevel.String(), c.ShouldEqual, "panic")
		c.So(TraceLevel.String(), c.ShouldEqual, "trace")
		c.So(Level(99).String(), c.ShouldEqual, "unknown")
		c.So(Level(99).IsValid(), c.ShouldBeFalse)
	})

	mockey.PatchConvey("TestLevel-ToSlog", t, func() {
		// 本模块级别数值越小级别越高，slog 相反，映射必须正确否则过滤行为会反转
		c.So(PanicLevel.toSlog(), c.ShouldEqual, slogLevelPanic)
		c.So(FatalLevel.toSlog(), c.ShouldEqual, slogLevelFatal)
		c.So(ErrorLevel.toSlog(), c.ShouldEqual, slog.LevelError)
		c.So(WarnLevel.toSlog(), c.ShouldEqual, slog.LevelWarn)
		c.So(InfoLevel.toSlog(), c.ShouldEqual, slog.LevelInfo)
		c.So(DebugLevel.toSlog(), c.ShouldEqual, slog.LevelDebug)
		c.So(TraceLevel.toSlog(), c.ShouldEqual, slogLevelTrace)
		c.So(Level(99).toSlog(), c.ShouldEqual, slog.LevelInfo)

		// 往返转换保持一致
		for _, lv := range []Level{PanicLevel, FatalLevel, ErrorLevel, WarnLevel, InfoLevel, DebugLevel, TraceLevel} {
			c.So(fromSlogLevel(lv.toSlog()), c.ShouldEqual, lv)
		}
	})

	mockey.PatchConvey("TestLevel-HandlerCoversAllLevels", t, func() {
		// 回归：此前文件输出使用独立的级别集合，导致 panic 级别日志从不落盘
		h := &xHandler{level: slogLevelTrace}
		for _, lv := range []Level{PanicLevel, FatalLevel, ErrorLevel, WarnLevel, InfoLevel, DebugLevel, TraceLevel} {
			c.So(h.Enabled(context.Background(), lv.toSlog()), c.ShouldBeTrue)
		}
	})
}
func TestCtxWithKV(t *testing.T) {
	mockey.PatchConvey("TestCtxWithKV", t, func() {
		mockey.PatchConvey("TestCtxWithKV-NewCtx", func() {
			ctx := context.Background()
			newCtx := CtxWithKV(ctx, map[string]any{"key": "value"})
			c.So(newCtx, c.ShouldNotBeNil)
			kv := getXLogContainerFromCtx(newCtx)
			c.So(kv["key"], c.ShouldEqual, "value")
		})

		mockey.PatchConvey("TestCtxWithKV-MergeKV", func() {
			ctx := context.Background()
			ctx = CtxWithKV(ctx, map[string]any{"key1": "value1"})
			ctx = CtxWithKV(ctx, map[string]any{"key2": "value2"})
			kv := getXLogContainerFromCtx(ctx)
			c.So(kv["key1"], c.ShouldEqual, "value1")
			c.So(kv["key2"], c.ShouldEqual, "value2")
		})

		mockey.PatchConvey("TestCtxWithKV-NilKV", func() {
			ctx := context.Background()
			newCtx := CtxWithKV(ctx, nil)
			c.So(newCtx, c.ShouldNotBeNil)
		})
	})
}

func TestXLogLevel(t *testing.T) {
	mockey.PatchConvey("TestXLogLevel", t, func() {
		old := currentLevel.Load()
		defer currentLevel.Store(old)

		mockey.PatchConvey("TestXLogLevel-Default", func() {
			currentLevel.Store(uint32(InfoLevel))
			c.So(XLogLevel(), c.ShouldEqual, "info")
			c.So(CurrentLevel(), c.ShouldEqual, InfoLevel)
		})

		mockey.PatchConvey("TestXLogLevel-FromConfig", func() {
			// 级别来自初始化，而非运行时回查 xconfig
			c.So(initXLogByConfig(&Config{Level: "debug"}), c.ShouldBeNil)
			c.So(XLogLevel(), c.ShouldEqual, "debug")
		})

		mockey.PatchConvey("TestXLogLevel-UnknownFallbackInfo", func() {
			c.So(initXLogByConfig(&Config{Level: "not-a-level"}), c.ShouldBeNil)
			c.So(XLogLevel(), c.ShouldEqual, "info")
		})
	})
}
func TestLogFunctions(t *testing.T) {
	mockey.PatchConvey("TestLogFunctions", t, func() {
		mockey.PatchConvey("TestInfo", func() {
			// Should not panic
			Info(context.Background(), "test info %s", "arg")
		})

		mockey.PatchConvey("TestWarn", func() {
			Warn(context.Background(), "test warn %s", "arg")
		})

		mockey.PatchConvey("TestError", func() {
			Error(context.Background(), "test error %s", "arg")
		})

		mockey.PatchConvey("TestDebug", func() {
			Debug(context.Background(), "test debug %s", "arg")
		})
	})
}

func TestRawLog(t *testing.T) {
	mockey.PatchConvey("TestRawLog", t, func() {
		mockey.PatchConvey("TestRawLog-WithOptions", func() {
			ctx := context.Background()
			RawLog(ctx, InfoLevel, "test message %s", "arg1", KVMap(map[string]any{"key": "value"}))
		})

		mockey.PatchConvey("TestRawLog-NoArgs", func() {
			ctx := context.Background()
			RawLog(ctx, InfoLevel, "test message")
		})

		mockey.PatchConvey("TestRawLog-WithKV", func() {
			ctx := context.Background()
			RawLog(ctx, InfoLevel, "test message", KV("single", "value"))
		})
	})
}

func TestOptions(t *testing.T) {
	mockey.PatchConvey("TestOptions", t, func() {
		mockey.PatchConvey("TestKV", func() {
			opt := &options{KV: make(map[string]any)}
			KV("key", "value")(opt)
			c.So(opt.KV["key"], c.ShouldEqual, "value")
		})

		mockey.PatchConvey("TestKVMap", func() {
			opt := &options{KV: make(map[string]any)}
			KVMap(map[string]any{"k1": "v1", "k2": "v2"})(opt)
			c.So(opt.KV["k1"], c.ShouldEqual, "v1")
			c.So(opt.KV["k2"], c.ShouldEqual, "v2")
		})

		mockey.PatchConvey("TestDefaultOptions", func() {
			opt := &options{KV: make(map[string]any)}
			c.So(opt, c.ShouldNotBeNil)
			c.So(opt.KV, c.ShouldNotBeNil)
			c.So(len(opt.KV), c.ShouldEqual, 0)
		})
	})
}

func TestLevelColor(t *testing.T) {
	mockey.PatchConvey("TestLevelColor", t, func() {
		c.So(levelColor(slogLevelTrace), c.ShouldEqual, colorGray)
		c.So(levelColor(slog.LevelDebug), c.ShouldEqual, colorGray)
		c.So(levelColor(slog.LevelInfo), c.ShouldEqual, colorBlue)
		c.So(levelColor(slog.LevelWarn), c.ShouldEqual, colorYellow)
		c.So(levelColor(slog.LevelError), c.ShouldEqual, colorRed)
		c.So(levelColor(slogLevelFatal), c.ShouldEqual, colorRed)
		c.So(levelColor(slogLevelPanic), c.ShouldEqual, colorRed)
	})
}

func TestCallerPretty(t *testing.T) {
	mockey.PatchConvey("TestCallerPretty", t, func() {
		c.So(callerPretty(nil), c.ShouldEqual, "???")
		c.So(callerPretty(&runtime.Frame{File: "/a/b/main.go", Line: 42}), c.ShouldEqual, "main.go:42")
	})
}

func TestGetXLogContainerFromCtx(t *testing.T) {
	mockey.PatchConvey("TestGetXLogContainerFromCtx", t, func() {
		mockey.PatchConvey("TestGetXLogContainerFromCtx-Empty", func() {
			c.So(getXLogContainerFromCtx(context.Background()), c.ShouldBeNil)
		})

		mockey.PatchConvey("TestGetXLogContainerFromCtx-NilCtx", func() {
			c.So(getXLogContainerFromCtx(nil), c.ShouldBeNil)
		})

		mockey.PatchConvey("TestGetXLogContainerFromCtx-WithKV", func() {
			ctx := CtxWithKV(context.Background(), map[string]any{"key": "value"})
			result := getXLogContainerFromCtx(ctx)
			c.So(result, c.ShouldNotBeNil)
			c.So(result["key"], c.ShouldEqual, "value")
		})
	})
}

// mockWriter 记录写入内容
type mockWriter struct {
	written []byte
}

func (m *mockWriter) Write(p []byte) (int, error) {
	m.written = append(m.written, p...)
	return len(p), nil
}

// errWriter 总是写入失败
type errWriter struct {
	err error
}

func (w *errWriter) Write([]byte) (int, error) { return 0, w.err }

// newTestRecord 构造一条测试日志记录
func newTestRecord(level slog.Level, msg string) slog.Record {
	return slog.NewRecord(time.Now(), level, msg, 0)
}

// decodeJSON 解析 handler 写出的 JSON 日志行
func decodeJSON(t *testing.T, b []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("日志不是合法 JSON: %v, 内容=%s", err, b)
	}
	return m
}

func TestHandler(t *testing.T) {
	mockey.PatchConvey("TestHandler", t, func() {
		mockey.PatchConvey("TestHandler-Enabled按级别过滤", func() {
			h := &xHandler{level: slog.LevelInfo}
			c.So(h.Enabled(context.Background(), slog.LevelInfo), c.ShouldBeTrue)
			c.So(h.Enabled(context.Background(), slog.LevelError), c.ShouldBeTrue)
			c.So(h.Enabled(context.Background(), slog.LevelDebug), c.ShouldBeFalse)
		})

		mockey.PatchConvey("TestHandler-JSON字段与迁移前一致", func() {
			fileW := &mockWriter{}
			h := &xHandler{
				serverName: "test-server",
				ip:         "127.0.0.1",
				pidStr:     "12345",
				fileWriter: fileW,
				level:      slogLevelTrace,
			}
			c.So(h.Handle(context.Background(), newTestRecord(slog.LevelInfo, "hello")), c.ShouldBeNil)

			m := decodeJSON(t, fileW.written)
			c.So(m["msg"], c.ShouldEqual, "hello")
			c.So(m["level"], c.ShouldEqual, "info")
			c.So(m["servername"], c.ShouldEqual, "test-server")
			c.So(m["ip"], c.ShouldEqual, "127.0.0.1")
			c.So(m["pid"], c.ShouldEqual, "12345")
			// time 为 "2006-01-02 15:04:05.999" 而非 slog 默认的 RFC3339
			ts, ok := m["time"].(string)
			c.So(ok, c.ShouldBeTrue)
			_, err := time.Parse(consoleTimeLayout, ts)
			c.So(err, c.ShouldBeNil)
		})

		mockey.PatchConvey("TestHandler-自定义级别名称", func() {
			for _, tc := range []struct {
				lv   slog.Level
				want string
			}{
				{slogLevelTrace, "trace"},
				{slog.LevelDebug, "debug"},
				{slog.LevelWarn, "warn"},
				{slog.LevelError, "error"},
				{slogLevelFatal, "fatal"},
				{slogLevelPanic, "panic"},
			} {
				fileW := &mockWriter{}
				h := &xHandler{fileWriter: fileW, level: slogLevelTrace}
				c.So(h.Handle(context.Background(), newTestRecord(tc.lv, "m")), c.ShouldBeNil)
				c.So(decodeJSON(t, fileW.written)["level"], c.ShouldEqual, tc.want)
			}
		})

		mockey.PatchConvey("TestHandler-注入ctx中的KV", func() {
			fileW := &mockWriter{}
			h := &xHandler{fileWriter: fileW, level: slogLevelTrace}
			ctx := CtxWithKV(context.Background(), map[string]any{"custom": "value"})
			c.So(h.Handle(ctx, newTestRecord(slog.LevelInfo, "m")), c.ShouldBeNil)
			c.So(decodeJSON(t, fileW.written)["custom"], c.ShouldEqual, "value")
		})

		mockey.PatchConvey("TestHandler-文件与控制台同时输出", func() {
			fileW, consoleW := &mockWriter{}, &mockWriter{}
			h := &xHandler{
				serverName:    "test-server",
				fileWriter:    fileW,
				consoleWriter: consoleW,
				level:         slogLevelTrace,
			}
			c.So(h.Handle(context.Background(), newTestRecord(slog.LevelInfo, "m")), c.ShouldBeNil)
			c.So(len(fileW.written), c.ShouldBeGreaterThan, 0)
			c.So(len(consoleW.written), c.ShouldBeGreaterThan, 0)
			c.So(string(fileW.written), c.ShouldContainSubstring, "test-server")
		})

		mockey.PatchConvey("TestHandler-无输出目标时不序列化", func() {
			// 两路输出都关闭，不应产生任何写入
			h := &xHandler{level: slogLevelTrace}
			c.So(h.Handle(context.Background(), newTestRecord(slog.LevelInfo, "m")), c.ShouldBeNil)
		})

		mockey.PatchConvey("TestHandler-控制台可读格式", func() {
			mockey.Mock(xutil.GetTraceIDFromCtx).Return("trace-123").Build()
			consoleW := &mockWriter{}
			h := &xHandler{consoleWriter: consoleW, level: slogLevelTrace}
			c.So(h.Handle(context.Background(), newTestRecord(slog.LevelInfo, "test message")), c.ShouldBeNil)

			out := string(consoleW.written)
			c.So(out, c.ShouldContainSubstring, "INFO")
			c.So(out, c.ShouldContainSubstring, "trace-123")
			c.So(out, c.ShouldContainSubstring, "test message")
		})

		mockey.PatchConvey("TestHandler-自定义字段与框架字段同名时不产生重复key", func() {
			// slog 的 attrs 是列表而非 map，若不去重会在 JSON 中出现两个同名字段
			mockey.Mock(xutil.GetTraceIDFromCtx).Return("from-ctx").Build()
			fileW := &mockWriter{}
			h := &xHandler{fileWriter: fileW, level: slogLevelTrace}
			r := newTestRecord(slog.LevelInfo, "m")
			r.AddAttrs(slog.String(fieldTraceID, "from-user"))
			c.So(h.Handle(context.Background(), r), c.ShouldBeNil)

			line := string(fileW.written)
			c.So(strings.Count(line, `"traceid"`), c.ShouldEqual, 1)
			c.So(decodeJSON(t, fileW.written)["traceid"], c.ShouldEqual, "from-ctx")
		})

		mockey.PatchConvey("TestHandler-调用方可覆盖servername", func() {
			fileW := &mockWriter{}
			h := &xHandler{serverName: "default-server", fileWriter: fileW, level: slogLevelTrace}
			r := newTestRecord(slog.LevelInfo, "m")
			r.AddAttrs(slog.String(fieldServerName, "custom-server"))
			c.So(h.Handle(context.Background(), r), c.ShouldBeNil)

			line := string(fileW.written)
			c.So(strings.Count(line, `"servername"`), c.ShouldEqual, 1)
			c.So(decodeJSON(t, fileW.written)["servername"], c.ShouldEqual, "custom-server")
		})

		mockey.PatchConvey("TestHandler-控制台raw模式复用JSON", func() {
			consoleW := &mockWriter{}
			h := &xHandler{consoleWriter: consoleW, consoleRaw: true, level: slogLevelTrace}
			c.So(h.Handle(context.Background(), newTestRecord(slog.LevelInfo, "m")), c.ShouldBeNil)
			c.So(decodeJSON(t, consoleW.written)["msg"], c.ShouldEqual, "m")
		})

		mockey.PatchConvey("TestHandler-panic栈附加到控制台", func() {
			consoleW := &mockWriter{}
			h := &xHandler{consoleWriter: consoleW, level: slogLevelTrace}
			r := newTestRecord(slog.LevelError, "panic recover")
			r.AddAttrs(slog.String(fieldPanicStack, "goroutine 1 [running]"))
			c.So(h.Handle(context.Background(), r), c.ShouldBeNil)
			c.So(string(consoleW.written), c.ShouldContainSubstring, "goroutine 1 [running]")
		})

		mockey.PatchConvey("TestHandler-应用时区", func() {
			loc, err := time.LoadLocation("UTC")
			c.So(err, c.ShouldBeNil)
			consoleW := &mockWriter{}
			h := &xHandler{location: loc, consoleWriter: consoleW, level: slogLevelTrace}
			r := slog.NewRecord(
				time.Date(2026, 1, 2, 3, 4, 5, 0, time.FixedZone("X", 8*3600)),
				slog.LevelInfo, "m", 0)
			c.So(h.Handle(context.Background(), r), c.ShouldBeNil)
			// 08:00 时区的 03:04:05 对应 UTC 的前一日 19:04:05
			c.So(string(consoleW.written), c.ShouldContainSubstring, "2026-01-01 19:04:05")
		})

		mockey.PatchConvey("TestHandler-文件写入失败返回错误", func() {
			h := &xHandler{
				fileWriter: &errWriter{err: errors.New("disk full")},
				level:      slogLevelTrace,
			}
			c.So(h.Handle(context.Background(), newTestRecord(slog.LevelInfo, "m")), c.ShouldNotBeNil)
		})

		mockey.PatchConvey("TestHandler-WithAttrs附加字段", func() {
			fileW := &mockWriter{}
			base := &xHandler{fileWriter: fileW, level: slogLevelTrace}
			h := base.WithAttrs([]slog.Attr{slog.String("app", "demo")})
			c.So(h.Handle(context.Background(), newTestRecord(slog.LevelInfo, "m")), c.ShouldBeNil)
			c.So(decodeJSON(t, fileW.written)["app"], c.ShouldEqual, "demo")

			// 空 attrs 返回自身，避免无谓复制
			c.So(base.WithAttrs(nil), c.ShouldEqual, base)
			// 日志为扁平结构，不支持分组
			c.So(base.WithGroup("g"), c.ShouldEqual, base)
		})

		mockey.PatchConvey("TestHandler-解析到业务调用位置", func() {
			fileW := &mockWriter{}
			h := &xHandler{
				fileWriter:     fileW,
				callerResolver: defaultCallerResolver,
				level:          slogLevelTrace,
			}
			c.So(h.Handle(context.Background(), newTestRecord(slog.LevelInfo, "m")), c.ShouldBeNil)
			m := decodeJSON(t, fileW.written)
			c.So(m["filename"], c.ShouldEqual, "xlog_test.go")
			c.So(m["lineid"], c.ShouldNotBeEmpty)
		})
	})
}

func TestInitXLogByConfig(t *testing.T) {
	mockey.PatchConvey("TestInitXLogByConfig-DirNotExist-MkdirFail", t, func() {
		mockey.Mock(xutil.DirExist).Return(false).Build()
		mockey.Mock(os.MkdirAll).Return(errors.New("mkdir failed")).Build()

		config := &Config{
			Path:          "/test/path",
			EnableFile:    true,
			EnableConsole: xutil.ToPtr(false),
		}
		err := initXLogByConfig(config)
		c.So(err, c.ShouldNotBeNil)
		c.So(err.Error(), c.ShouldContainSubstring, "os.MkdirAll failed")
	})

	mockey.PatchConvey("TestInitXLogByConfig-OpenFileFail", t, func() {
		mockey.Mock(xutil.DirExist).Return(true).Build()
		mockey.Mock(newRotateWriter).Return(nil, errors.New("open log file failed")).Build()

		config := &Config{
			Path:          "/test/path",
			Name:          "test",
			MaxAge:        "7d",
			RotateTime:    "1d",
			EnableFile:    true,
			EnableConsole: xutil.ToPtr(false),
		}
		err := initXLogByConfig(config)
		c.So(err, c.ShouldNotBeNil)
		c.So(err.Error(), c.ShouldContainSubstring, "open log file failed")
	})

	mockey.PatchConvey("TestInitXLogByConfig-DefaultConsoleOnly", t, func() {
		// 默认配置不应创建日志目录、不应创建轮转文件
		mkdirMock := mockey.Mock(os.MkdirAll).Return(nil).Build()
		rotateMock := mockey.Mock(newRotateWriter).Return(nil, errors.New("should not be called")).Build()

		config := configMergeDefault(nil)
		err := initXLogByConfig(config)
		c.So(err, c.ShouldBeNil)
		c.So(mkdirMock.Times(), c.ShouldEqual, 0)
		c.So(rotateMock.Times(), c.ShouldEqual, 0)
	})

	mockey.PatchConvey("TestInitXLogByConfig-NilConfig", t, func() {
		// 传入 nil 时走默认配置，不应 panic
		rotateMock := mockey.Mock(newRotateWriter).Return(nil, errors.New("should not be called")).Build()

		err := initXLogByConfig(nil)
		c.So(err, c.ShouldBeNil)
		c.So(rotateMock.Times(), c.ShouldEqual, 0)
	})

	mockey.PatchConvey("TestInitXLogByConfig-NilEnableConsole", t, func() {
		// 未经 configMergeDefault 的 Config（EnableConsole 为 nil）不应 panic
		rotateMock := mockey.Mock(newRotateWriter).Return(nil, errors.New("should not be called")).Build()

		err := initXLogByConfig(&Config{Level: "debug"})
		c.So(err, c.ShouldBeNil)
		c.So(rotateMock.Times(), c.ShouldEqual, 0)
	})

	mockey.PatchConvey("TestInitXLogByConfig-ConsoleOnly-InvalidTimezone", t, func() {
		// 仅控制台模式下时区加载失败也能正常初始化
		rotateMock := mockey.Mock(newRotateWriter).Return(nil, errors.New("should not be called")).Build()

		config := configMergeDefault(&Config{Timezone: "Invalid/Zone"})
		err := initXLogByConfig(config)
		c.So(err, c.ShouldBeNil)
		c.So(rotateMock.Times(), c.ShouldEqual, 0)
	})
}

func TestKVFromCtx(t *testing.T) {
	mockey.PatchConvey("TestKVFromCtx", t, func() {
		mockey.PatchConvey("TestKVFromCtx-NotInjected", func() {
			c.So(KVFromCtx(context.Background()), c.ShouldBeNil)
		})

		mockey.PatchConvey("TestKVFromCtx-Injected", func() {
			ctx := CtxWithKV(context.Background(), map[string]any{"k": "v"})
			c.So(KVFromCtx(ctx), c.ShouldResemble, map[string]any{"k": "v"})
		})

		mockey.PatchConvey("TestKVFromCtx-EmptyContainer", func() {
			// 注入空容器与从未注入需可区分
			ctx := CtxWithKV(context.Background(), nil)
			got := KVFromCtx(ctx)
			c.So(got, c.ShouldNotBeNil)
			c.So(got, c.ShouldBeEmpty)
		})

		mockey.PatchConvey("TestKVFromCtx-ReturnsCopy", func() {
			// 返回副本，调用方修改不应影响后续日志
			ctx := CtxWithKV(context.Background(), map[string]any{"k": "v"})
			got := KVFromCtx(ctx)
			got["k"] = "changed"
			c.So(KVFromCtx(ctx)["k"], c.ShouldEqual, "v")
		})
	})
}

func TestFileWriterLifecycle(t *testing.T) {
	mockey.PatchConvey("TestFileWriterLifecycle", t, func() {
		mockey.PatchConvey("TestCloseFileWriter-NoWriter", func() {
			c.So(closeFileWriter(), c.ShouldBeNil)
		})

		mockey.PatchConvey("TestCloseFileWriter-ClosesUnderlying", func() {
			mw := &mockWriteCloser{}
			setFileWriter(newAsyncWriter(mw, 8))
			c.So(closeFileWriter(), c.ShouldBeNil)
			c.So(mw.closed, c.ShouldBeTrue)
			// 重复关闭安全
			c.So(closeFileWriter(), c.ShouldBeNil)
		})

		mockey.PatchConvey("TestSetFileWriter-ReplacesAndClosesPrevious", func() {
			// 重复初始化不应泄漏上一个写入器的 goroutine
			first, second := &mockWriteCloser{}, &mockWriteCloser{}
			setFileWriter(newAsyncWriter(first, 8))
			setFileWriter(newAsyncWriter(second, 8))
			c.So(first.closed, c.ShouldBeTrue)
			c.So(second.closed, c.ShouldBeFalse)

			c.So(closeFileWriter(), c.ShouldBeNil)
			c.So(second.closed, c.ShouldBeTrue)
		})
	})
}

func TestAsyncWriter(t *testing.T) {
	mockey.PatchConvey("TestAsyncWriter", t, func() {
		mockey.PatchConvey("TestAsyncWriter-WriteAndClose", func() {
			mw := &mockWriteCloser{}
			aw := newAsyncWriter(mw, 16)

			// 写入多条数据
			n, err := aw.Write([]byte("hello"))
			c.So(err, c.ShouldBeNil)
			c.So(n, c.ShouldEqual, 5)

			n, err = aw.Write([]byte(" world"))
			c.So(err, c.ShouldBeNil)
			c.So(n, c.ShouldEqual, 6)

			// 关闭后验证数据完整写入
			err = aw.Close()
			c.So(err, c.ShouldBeNil)
			c.So(string(mw.written), c.ShouldEqual, "hello world")
			c.So(mw.closed, c.ShouldBeTrue)
		})

		mockey.PatchConvey("TestAsyncWriter-DataIsolation", func() {
			// 验证 Write 会拷贝数据，调用方修改原 buffer 不影响已写入的内容
			mw := &mockWriteCloser{}
			aw := newAsyncWriter(mw, 16)

			buf := []byte("original")
			_, _ = aw.Write(buf)

			// 修改原 buffer
			copy(buf, "modified")

			_ = aw.Close()
			c.So(string(mw.written), c.ShouldEqual, "original")
		})

		mockey.PatchConvey("TestAsyncWriter-CloseIdempotent", func() {
			// 多次 Close 不应 panic
			mw := &mockWriteCloser{}
			aw := newAsyncWriter(mw, 16)

			err := aw.Close()
			c.So(err, c.ShouldBeNil)

			// 第二次 Close 不 panic，底层 writer 只关闭一次
			err = aw.Close()
			c.So(err, c.ShouldBeNil)
		})

		mockey.PatchConvey("TestAsyncWriter-DefaultBufferSize", func() {
			mw := &mockWriteCloser{}
			aw := newAsyncWriter(mw, 0)
			c.So(cap(aw.ch), c.ShouldEqual, defaultAsyncBufferSize)
			_ = aw.Close()
		})

		mockey.PatchConvey("TestAsyncWriter-LargeVolume", func() {
			// 验证大量写入不丢数据
			mw := &mockWriteCloser{}
			aw := newAsyncWriter(mw, 64)

			total := 1000
			msgLen := 0
			for i := 0; i < total; i++ {
				msg := []byte("log line\n")
				msgLen += len(msg)
				_, _ = aw.Write(msg)
			}

			_ = aw.Close()
			c.So(len(mw.written), c.ShouldEqual, msgLen)
		})
	})
}

type mockWriteCloser struct {
	written []byte
	closed  bool
}

func (m *mockWriteCloser) Write(p []byte) (int, error) {
	m.written = append(m.written, p...)
	return len(p), nil
}

func (m *mockWriteCloser) Close() error {
	m.closed = true
	return nil
}

func TestGetConfig(t *testing.T) {
	mockey.PatchConvey("TestGetConfig-UnmarshalFail", t, func() {
		mockey.Mock(xconfig.UnmarshalConfig).Return(errors.New("unmarshal failed")).Build()

		config, err := getConfig()
		c.So(err, c.ShouldNotBeNil)
		c.So(config, c.ShouldBeNil)
	})

	mockey.PatchConvey("TestGetConfig-Success", t, func() {
		mockey.Mock(xconfig.UnmarshalConfig).Return(nil).Build()

		config, err := getConfig()
		c.So(err, c.ShouldBeNil)
		c.So(config, c.ShouldNotBeNil)
		// 验证默认值已合并
		c.So(config.Level, c.ShouldEqual, "info")
		c.So(config.Name, c.ShouldEqual, "app")
	})
}

func TestLockedWriter(t *testing.T) {
	mockey.PatchConvey("TestLockedWriter", t, func() {
		mockey.PatchConvey("nil 写入器返回 nil", func() {
			c.So(newLockedWriter(nil), c.ShouldBeNil)
		})

		mockey.PatchConvey("并发长行不被穿插", func() {
			// 超过管道缓冲区的日志行（如 panic 栈）会被拆成多次写入，
			// 不加锁时并发下会相互穿插
			buf := &mockWriter{}
			w := newLockedWriter(&chunkedWriter{dst: buf})

			var wg sync.WaitGroup
			for i := 0; i < 20; i++ {
				wg.Add(1)
				go func(n int) {
					defer wg.Done()
					_, _ = w.Write([]byte(strings.Repeat(string(rune('A'+n)), 4096) + "\n"))
				}(i)
			}
			wg.Wait()

			for _, line := range strings.Split(strings.TrimSpace(string(buf.written)), "\n") {
				c.So(len(line), c.ShouldEqual, 4096)
				c.So(strings.Trim(line, string(line[0])), c.ShouldBeEmpty)
			}
		})
	})
}

// chunkedWriter 分多次写入，放大并发穿插的概率
type chunkedWriter struct {
	dst *mockWriter
}

func (w *chunkedWriter) Write(p []byte) (int, error) {
	for i := 0; i < len(p); i += 512 {
		end := min(i+512, len(p))
		_, _ = w.dst.Write(p[i:end])
	}
	return len(p), nil
}
