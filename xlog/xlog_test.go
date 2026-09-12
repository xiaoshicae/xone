package xlog

import (
	"context"
	"errors"
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/xiaoshicae/xone/v2/xconfig"
	"github.com/xiaoshicae/xone/v2/xutil"

	"github.com/bytedance/mockey"
	"github.com/sirupsen/logrus"
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

	mockey.PatchConvey("TestLevel-ToLogrus", t, func() {
		// 取值需与 logrus 一一对应，否则过滤行为会错位
		c.So(PanicLevel.toLogrus(), c.ShouldEqual, logrus.PanicLevel)
		c.So(FatalLevel.toLogrus(), c.ShouldEqual, logrus.FatalLevel)
		c.So(ErrorLevel.toLogrus(), c.ShouldEqual, logrus.ErrorLevel)
		c.So(WarnLevel.toLogrus(), c.ShouldEqual, logrus.WarnLevel)
		c.So(InfoLevel.toLogrus(), c.ShouldEqual, logrus.InfoLevel)
		c.So(DebugLevel.toLogrus(), c.ShouldEqual, logrus.DebugLevel)
		c.So(TraceLevel.toLogrus(), c.ShouldEqual, logrus.TraceLevel)
		c.So(Level(99).toLogrus(), c.ShouldEqual, logrus.InfoLevel)
	})

	mockey.PatchConvey("TestLevel-HookCoversAllLevels", t, func() {
		// 回归：此前 file hook 使用独立的级别集合，导致 panic 级别日志从不落盘
		hook := &xLogHook{}
		c.So(hook.Levels(), c.ShouldResemble, logrus.AllLevels)
		c.So(hook.Levels(), c.ShouldContain, logrus.PanicLevel)
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
			opt := defaultOptions()
			KV("key", "value")(opt)
			c.So(opt.KV["key"], c.ShouldEqual, "value")
		})

		mockey.PatchConvey("TestKVMap", func() {
			opt := defaultOptions()
			KVMap(map[string]any{"k1": "v1", "k2": "v2"})(opt)
			c.So(opt.KV["k1"], c.ShouldEqual, "v1")
			c.So(opt.KV["k2"], c.ShouldEqual, "v2")
		})

		mockey.PatchConvey("TestDefaultOptions", func() {
			opt := defaultOptions()
			c.So(opt, c.ShouldNotBeNil)
			c.So(opt.KV, c.ShouldNotBeNil)
			c.So(len(opt.KV), c.ShouldEqual, 0)
		})
	})
}

func TestGetLogConsoleLogColor(t *testing.T) {
	mockey.PatchConvey("TestGetLogConsoleLogColor", t, func() {
		mockey.PatchConvey("TestDebugLevel", func() {
			color := getLogConsoleLogColor(logrus.DebugLevel)
			c.So(color, c.ShouldEqual, colorGray)
		})

		mockey.PatchConvey("TestTraceLevel", func() {
			color := getLogConsoleLogColor(logrus.TraceLevel)
			c.So(color, c.ShouldEqual, colorGray)
		})

		mockey.PatchConvey("TestWarnLevel", func() {
			color := getLogConsoleLogColor(logrus.WarnLevel)
			c.So(color, c.ShouldEqual, colorYellow)
		})

		mockey.PatchConvey("TestErrorLevel", func() {
			color := getLogConsoleLogColor(logrus.ErrorLevel)
			c.So(color, c.ShouldEqual, colorRed)
		})

		mockey.PatchConvey("TestFatalLevel", func() {
			color := getLogConsoleLogColor(logrus.FatalLevel)
			c.So(color, c.ShouldEqual, colorRed)
		})

		mockey.PatchConvey("TestPanicLevel", func() {
			color := getLogConsoleLogColor(logrus.PanicLevel)
			c.So(color, c.ShouldEqual, colorRed)
		})

		mockey.PatchConvey("TestInfoLevel", func() {
			color := getLogConsoleLogColor(logrus.InfoLevel)
			c.So(color, c.ShouldEqual, colorBlue)
		})
	})
}

func TestCallerPretty(t *testing.T) {
	mockey.PatchConvey("TestCallerPretty", t, func() {
		mockey.PatchConvey("TestCallerPretty-Nil", func() {
			fileVal := callerPretty(nil)
			c.So(fileVal, c.ShouldEqual, "???")
		})
	})
}

func TestGetXLogContainerFromCtx(t *testing.T) {
	mockey.PatchConvey("TestGetXLogContainerFromCtx", t, func() {
		mockey.PatchConvey("TestGetXLogContainerFromCtx-Empty", func() {
			ctx := context.Background()
			result := getXLogContainerFromCtx(ctx)
			c.So(result, c.ShouldBeNil)
		})

		mockey.PatchConvey("TestGetXLogContainerFromCtx-WithKV", func() {
			ctx := context.Background()
			ctx = CtxWithKV(ctx, map[string]any{"key": "value"})
			result := getXLogContainerFromCtx(ctx)
			c.So(result, c.ShouldNotBeNil)
			c.So(result["key"], c.ShouldEqual, "value")
		})
	})
}

func TestXLogHook(t *testing.T) {
	newEntry := func(ctx context.Context, data logrus.Fields) *logrus.Entry {
		return &logrus.Entry{
			Logger:  logrus.New(),
			Data:    data,
			Context: ctx,
			Time:    time.Now(),
			Level:   logrus.InfoLevel,
			Message: "test",
		}
	}

	mockey.PatchConvey("TestXLogHook", t, func() {
		mockey.PatchConvey("TestXLogHook-Levels", func() {
			c.So((&xLogHook{}).Levels(), c.ShouldResemble, logrus.AllLevels)
		})

		mockey.PatchConvey("TestXLogHook-Fire", func() {
			hook := &xLogHook{IP: "127.0.0.1", ServerName: "test-server", PidStr: "12345"}
			entry := newEntry(context.Background(), logrus.Fields{})
			c.So(hook.Fire(entry), c.ShouldBeNil)
			c.So(entry.Data["ip"], c.ShouldEqual, "127.0.0.1")
			c.So(entry.Data["pid"], c.ShouldEqual, "12345")
			c.So(entry.Data["servername"], c.ShouldEqual, "test-server")
		})

		mockey.PatchConvey("TestXLogHook-Fire-WithExistingServername", func() {
			hook := &xLogHook{IP: "127.0.0.1", ServerName: "test-server", PidStr: "12345"}
			entry := newEntry(context.Background(), logrus.Fields{"servername": "existing-server"})
			c.So(hook.Fire(entry), c.ShouldBeNil)
			c.So(entry.Data["servername"], c.ShouldEqual, "existing-server")
		})

		mockey.PatchConvey("TestXLogHook-Fire-WithCtxKV", func() {
			hook := &xLogHook{IP: "127.0.0.1", ServerName: "test-server", PidStr: "12345"}
			ctx := CtxWithKV(context.Background(), map[string]any{"custom": "value"})
			entry := newEntry(ctx, logrus.Fields{})
			c.So(hook.Fire(entry), c.ShouldBeNil)
			c.So(entry.Data["custom"], c.ShouldEqual, "value")
		})

		mockey.PatchConvey("TestXLogHook-Fire-WithConsole", func() {
			writer := &mockWriter{}
			hook := &xLogHook{IP: "127.0.0.1", ServerName: "test-server", PidStr: "12345", consoleWriter: writer}
			c.So(hook.Fire(newEntry(context.Background(), logrus.Fields{})), c.ShouldBeNil)
			c.So(len(writer.written), c.ShouldBeGreaterThan, 0)
		})

		mockey.PatchConvey("TestXLogHook-Fire-WithFileAndConsole", func() {
			// 文件与控制台同时开启时，两路都应收到内容
			fileW, consoleW := &mockWriter{}, &mockWriter{}
			hook := &xLogHook{
				ServerName:    "test-server",
				jsonFormatter: &logrus.JSONFormatter{},
				consoleWriter: consoleW,
				fileWriter:    fileW,
			}
			c.So(hook.Fire(newEntry(context.Background(), logrus.Fields{})), c.ShouldBeNil)
			c.So(len(fileW.written), c.ShouldBeGreaterThan, 0)
			c.So(len(consoleW.written), c.ShouldBeGreaterThan, 0)
			c.So(string(fileW.written), c.ShouldContainSubstring, "test-server")
		})

		mockey.PatchConvey("TestXLogHook-Fire-NoSinkSkipsSerialization", func() {
			// 两路输出都关闭时不应调用 JSON 序列化
			formatter := &countingFormatter{}
			hook := &xLogHook{jsonFormatter: formatter}
			c.So(hook.Fire(newEntry(context.Background(), logrus.Fields{})), c.ShouldBeNil)
			c.So(formatter.calls, c.ShouldEqual, 0)
		})

		mockey.PatchConvey("TestXLogHook-Fire-ConsolePrettySkipsSerialization", func() {
			// 控制台使用可读格式时无需 JSON，不应产生被丢弃的序列化
			formatter := &countingFormatter{}
			hook := &xLogHook{jsonFormatter: formatter, consoleWriter: &mockWriter{}}
			c.So(hook.Fire(newEntry(context.Background(), logrus.Fields{})), c.ShouldBeNil)
			c.So(formatter.calls, c.ShouldEqual, 0)
		})

		mockey.PatchConvey("TestXLogHook-Fire-SerializesOnceForBothSinks", func() {
			// raw 控制台 + 文件：共用同一次序列化结果
			formatter := &countingFormatter{}
			hook := &xLogHook{
				jsonFormatter: formatter,
				consoleWriter: &mockWriter{},
				consoleRaw:    true,
				fileWriter:    &mockWriter{},
			}
			c.So(hook.Fire(newEntry(context.Background(), logrus.Fields{})), c.ShouldBeNil)
			c.So(formatter.calls, c.ShouldEqual, 1)
		})

		mockey.PatchConvey("TestXLogHook-Fire-AppliesTimezone", func() {
			// 时区在 Fire 中统一应用，保证控制台与 JSON 时间一致
			loc, err := time.LoadLocation("UTC")
			c.So(err, c.ShouldBeNil)
			consoleW := &mockWriter{}
			hook := &xLogHook{location: loc, consoleWriter: consoleW}
			entry := newEntry(context.Background(), logrus.Fields{})
			entry.Time = time.Date(2026, 1, 2, 3, 4, 5, 0, time.FixedZone("X", 8*3600))
			c.So(hook.Fire(entry), c.ShouldBeNil)
			c.So(entry.Time.Location(), c.ShouldEqual, loc)
			// 08:00 时区的 03:04:05 对应 UTC 的前一日 19:04:05
			c.So(string(consoleW.written), c.ShouldContainSubstring, "2026-01-01 19:04:05")
		})

		mockey.PatchConvey("TestXLogHook-Fire-FileWriteError", func() {
			hook := &xLogHook{
				jsonFormatter: &logrus.JSONFormatter{},
				fileWriter:    &errWriter{err: errors.New("disk full")},
			}
			c.So(hook.Fire(newEntry(context.Background(), logrus.Fields{})), c.ShouldNotBeNil)
		})

		mockey.PatchConvey("TestXLogHook-EnsureCaller-WithCaller", func() {
			hook := &xLogHook{}
			frame := &runtime.Frame{Function: "test.TestFunc", File: "/test/file.go", Line: 100}
			result := hook.ensureCaller(&logrus.Entry{Logger: logrus.New(), Caller: frame})
			c.So(result, c.ShouldEqual, frame)
		})

		mockey.PatchConvey("TestXLogHook-EnsureCaller-WithoutCaller", func() {
			hook := &xLogHook{SuffixToIgnore: findFrameIgnoreFileNames}
			result := hook.ensureCaller(&logrus.Entry{Logger: logrus.New()})
			c.So(result, c.ShouldNotBeNil)
		})
	})
}

// mockWriter 记录写入内容
type mockWriter struct {
	written []byte
}

// countingFormatter 统计序列化次数，用于验证不产生多余的序列化
type countingFormatter struct {
	calls int
}

func (f *countingFormatter) Format(*logrus.Entry) ([]byte, error) {
	f.calls++
	return []byte("{}\n"), nil
}

// errWriter 总是写入失败
type errWriter struct {
	err error
}

func (w *errWriter) Write([]byte) (int, error) { return 0, w.err }

func (m *mockWriter) Write(p []byte) (n int, err error) {
	m.written = append(m.written, p...)
	return len(p), nil
}

func TestXLogHookWriteConsole(t *testing.T) {
	testCaller := &runtime.Frame{Function: "test.TestFunc", File: "/test/file.go", Line: 100}
	newEntry := func(data logrus.Fields) *logrus.Entry {
		return &logrus.Entry{
			Logger:  logrus.New(),
			Data:    data,
			Context: context.Background(),
			Time:    time.Now(),
			Level:   logrus.InfoLevel,
			Message: "test message",
		}
	}

	mockey.PatchConvey("TestXLogHookWriteConsole", t, func() {
		mockey.PatchConvey("TestWriteConsole-Raw", func() {
			writer := &mockWriter{}
			hook := &xLogHook{consoleWriter: writer, consoleRaw: true}
			jsonLine := []byte(`{"msg":"test message"}` + "\n")
			c.So(hook.writeConsole(newEntry(logrus.Fields{"traceid": "trace-123"}), testCaller, jsonLine), c.ShouldBeNil)
			// raw 模式直接复用已序列化结果，不做二次加工
			c.So(string(writer.written), c.ShouldEqual, string(jsonLine))
		})

		mockey.PatchConvey("TestWriteConsole-Formatted", func() {
			writer := &mockWriter{}
			hook := &xLogHook{consoleWriter: writer}
			c.So(hook.writeConsole(newEntry(logrus.Fields{"traceid": "trace-123"}), testCaller, nil), c.ShouldBeNil)
			out := string(writer.written)
			c.So(out, c.ShouldContainSubstring, "INFO")
			c.So(out, c.ShouldContainSubstring, "file.go:100")
			c.So(out, c.ShouldContainSubstring, "trace-123")
			c.So(out, c.ShouldContainSubstring, "test message")
		})

		mockey.PatchConvey("TestWriteConsole-WithPanicStack", func() {
			writer := &mockWriter{}
			hook := &xLogHook{consoleWriter: writer}
			entry := newEntry(logrus.Fields{"panic_stack": "goroutine 1 [running]"})
			c.So(hook.writeConsole(entry, testCaller, nil), c.ShouldBeNil)
			c.So(string(writer.written), c.ShouldContainSubstring, "goroutine 1 [running]")
		})

		mockey.PatchConvey("TestWriteConsole-NilCaller", func() {
			writer := &mockWriter{}
			hook := &xLogHook{consoleWriter: writer}
			c.So(hook.writeConsole(newEntry(logrus.Fields{}), nil, nil), c.ShouldBeNil)
			c.So(string(writer.written), c.ShouldContainSubstring, "???")
		})

		mockey.PatchConvey("TestWriteConsole-WriteError", func() {
			hook := &xLogHook{consoleWriter: &errWriter{err: errors.New("broken pipe")}}
			c.So(hook.writeConsole(newEntry(logrus.Fields{}), testCaller, nil), c.ShouldNotBeNil)
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
