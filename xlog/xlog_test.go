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
	rotatelogs "github.com/lestrrat-go/file-rotatelogs"
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

func TestResolveLevels(t *testing.T) {
	mockey.PatchConvey("TestResolveLevels", t, func() {
		mockey.PatchConvey("TestResolveLevels-Debug", func() {
			levels := resolveLevels("debug")
			c.So(levels, c.ShouldContain, logrus.DebugLevel)
			c.So(levels, c.ShouldContain, logrus.InfoLevel)
		})

		mockey.PatchConvey("TestResolveLevels-Info", func() {
			levels := resolveLevels("info")
			c.So(levels, c.ShouldContain, logrus.InfoLevel)
			c.So(levels, c.ShouldNotContain, logrus.DebugLevel)
		})

		mockey.PatchConvey("TestResolveLevels-Warn", func() {
			levels := resolveLevels("warn")
			c.So(levels, c.ShouldContain, logrus.WarnLevel)
			c.So(levels, c.ShouldNotContain, logrus.InfoLevel)
		})

		mockey.PatchConvey("TestResolveLevels-Error", func() {
			levels := resolveLevels("error")
			c.So(levels, c.ShouldContain, logrus.ErrorLevel)
			c.So(levels, c.ShouldNotContain, logrus.WarnLevel)
		})

		mockey.PatchConvey("TestResolveLevels-Fatal", func() {
			levels := resolveLevels("fatal")
			c.So(levels, c.ShouldContain, logrus.FatalLevel)
			c.So(len(levels), c.ShouldEqual, 1)
		})

		mockey.PatchConvey("TestResolveLevels-Unknown", func() {
			levels := resolveLevels("unknown")
			c.So(levels, c.ShouldContain, logrus.InfoLevel) // default to info
		})

		mockey.PatchConvey("TestResolveLevels-UpperCase", func() {
			levels := resolveLevels("DEBUG")
			c.So(levels, c.ShouldContain, logrus.DebugLevel)
		})
	})
}

func TestCtxWithKV(t *testing.T) {
	mockey.PatchConvey("TestCtxWithKV", t, func() {
		mockey.PatchConvey("TestCtxWithKV-NewCtx", func() {
			ctx := context.Background()
			newCtx := CtxWithKV(ctx, map[string]any{"key": "value"})
			c.So(newCtx, c.ShouldNotBeNil)
			kv := newCtx.Value(XLogCtxKVContainerKey).(map[string]any)
			c.So(kv["key"], c.ShouldEqual, "value")
		})

		mockey.PatchConvey("TestCtxWithKV-MergeKV", func() {
			ctx := context.Background()
			ctx = CtxWithKV(ctx, map[string]any{"key1": "value1"})
			ctx = CtxWithKV(ctx, map[string]any{"key2": "value2"})
			kv := ctx.Value(XLogCtxKVContainerKey).(map[string]any)
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
		mockey.PatchConvey("TestXLogLevel-Default", func() {
			mockey.Mock(xconfig.GetString).Return("").Build()
			level := XLogLevel()
			c.So(level, c.ShouldEqual, "Info")
		})

		mockey.PatchConvey("TestXLogLevel-Custom", func() {
			mockey.Mock(xconfig.GetString).Return("debug").Build()
			level := XLogLevel()
			c.So(level, c.ShouldEqual, "debug")
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
			RawLog(ctx, logrus.InfoLevel, "test message %s", "arg1", KVMap(map[string]any{"key": "value"}))
		})

		mockey.PatchConvey("TestRawLog-NoArgs", func() {
			ctx := context.Background()
			RawLog(ctx, logrus.InfoLevel, "test message")
		})

		mockey.PatchConvey("TestRawLog-WithKV", func() {
			ctx := context.Background()
			RawLog(ctx, logrus.InfoLevel, "test message", KV("single", "value"))
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
	mockey.PatchConvey("TestXLogHook", t, func() {
		mockey.PatchConvey("TestXLogHook-Levels", func() {
			hook := &xLogHook{}
			levels := hook.Levels()
			c.So(levels, c.ShouldResemble, logrus.AllLevels)
		})

		mockey.PatchConvey("TestXLogHook-Fire", func() {
			hook := &xLogHook{
				IP:         "127.0.0.1",
				ServerName: "test-server",
				PidStr:     "12345",
			}
			entry := &logrus.Entry{
				Logger:  logrus.New(),
				Data:    logrus.Fields{},
				Context: context.Background(),
				Time:    time.Now(),
				Level:   logrus.InfoLevel,
				Message: "test",
			}
			err := hook.Fire(entry)
			c.So(err, c.ShouldBeNil)
			c.So(entry.Data["ip"], c.ShouldEqual, "127.0.0.1")
			c.So(entry.Data["pid"], c.ShouldEqual, "12345")
			c.So(entry.Data["servername"], c.ShouldEqual, "test-server")
		})

		mockey.PatchConvey("TestXLogHook-Fire-WithExistingServername", func() {
			hook := &xLogHook{
				IP:         "127.0.0.1",
				ServerName: "test-server",
				PidStr:     "12345",
			}
			entry := &logrus.Entry{
				Logger:  logrus.New(),
				Data:    logrus.Fields{"servername": "existing-server"},
				Context: context.Background(),
				Time:    time.Now(),
				Level:   logrus.InfoLevel,
				Message: "test",
			}
			err := hook.Fire(entry)
			c.So(err, c.ShouldBeNil)
			c.So(entry.Data["servername"], c.ShouldEqual, "existing-server")
		})

		mockey.PatchConvey("TestXLogHook-Fire-WithCtxKV", func() {
			hook := &xLogHook{
				IP:         "127.0.0.1",
				ServerName: "test-server",
				PidStr:     "12345",
			}
			ctx := CtxWithKV(context.Background(), map[string]any{"custom": "value"})
			entry := &logrus.Entry{
				Logger:  logrus.New(),
				Data:    logrus.Fields{},
				Context: ctx,
				Time:    time.Now(),
				Level:   logrus.InfoLevel,
				Message: "test",
			}
			err := hook.Fire(entry)
			c.So(err, c.ShouldBeNil)
			c.So(entry.Data["custom"], c.ShouldEqual, "value")
		})

		mockey.PatchConvey("TestXLogHook-Fire-WithConsole", func() {
			writer := &mockWriter{}
			hook := &xLogHook{
				IP:            "127.0.0.1",
				ServerName:    "test-server",
				PidStr:        "12345",
				EnableConsole: true,
				Writer:        writer,
			}
			logger := logrus.New()
			logger.SetFormatter(&logrus.JSONFormatter{})
			entry := &logrus.Entry{
				Logger:  logger,
				Data:    logrus.Fields{},
				Context: context.Background(),
				Time:    time.Now(),
				Level:   logrus.InfoLevel,
				Message: "test",
			}
			err := hook.Fire(entry)
			c.So(err, c.ShouldBeNil)
			c.So(len(writer.written), c.ShouldBeGreaterThan, 0)
		})

		mockey.PatchConvey("TestXLogHook-EnsureCaller-WithCaller", func() {
			hook := &xLogHook{}
			frame := &runtime.Frame{
				Function: "test.TestFunc",
				File:     "/test/file.go",
				Line:     100,
			}
			entry := &logrus.Entry{
				Logger: logrus.New(),
				Caller: frame,
			}
			result := hook.ensureCaller(entry)
			c.So(result, c.ShouldEqual, frame)
		})
	})
}

type mockWriter struct {
	written []byte
}

func (m *mockWriter) Write(p []byte) (n int, err error) {
	m.written = append(m.written, p...)
	return len(p), nil
}

func TestXLogHookConsolePrint(t *testing.T) {
	mockey.PatchConvey("TestXLogHookConsolePrint", t, func() {
		testCaller := &runtime.Frame{
			Function: "test.TestFunc",
			File:     "/test/file.go",
			Line:     100,
		}

		mockey.PatchConvey("TestConsolePrint-Raw", func() {
			writer := &mockWriter{}
			hook := &xLogHook{
				IP:                 "127.0.0.1",
				ServerName:         "test-server",
				PidStr:             "12345",
				EnableConsole:      true,
				ConsoleFormatIsRaw: true,
				Writer:             writer,
			}
			logger := logrus.New()
			logger.SetFormatter(&logrus.JSONFormatter{})
			entry := &logrus.Entry{
				Logger:  logger,
				Data:    logrus.Fields{"traceid": "trace-123"},
				Context: context.Background(),
				Time:    time.Now(),
				Level:   logrus.InfoLevel,
				Message: "test message",
			}
			err := hook.consolePrint(entry, testCaller)
			c.So(err, c.ShouldBeNil)
			c.So(len(writer.written), c.ShouldBeGreaterThan, 0)
		})

		mockey.PatchConvey("TestConsolePrint-Formatted", func() {
			writer := &mockWriter{}
			hook := &xLogHook{
				IP:                 "127.0.0.1",
				ServerName:         "test-server",
				PidStr:             "12345",
				EnableConsole:      true,
				ConsoleFormatIsRaw: false,
				Writer:             writer,
			}
			logger := logrus.New()
			logger.SetFormatter(&logrus.JSONFormatter{})
			entry := &logrus.Entry{
				Logger:  logger,
				Data:    logrus.Fields{"traceid": "trace-123"},
				Context: context.Background(),
				Time:    time.Now(),
				Level:   logrus.InfoLevel,
				Message: "test message",
			}
			err := hook.consolePrint(entry, testCaller)
			c.So(err, c.ShouldBeNil)
			c.So(len(writer.written), c.ShouldBeGreaterThan, 0)
		})

		mockey.PatchConvey("TestConsolePrint-WithPanicStack", func() {
			writer := &mockWriter{}
			hook := &xLogHook{
				IP:                 "127.0.0.1",
				ServerName:         "test-server",
				PidStr:             "12345",
				EnableConsole:      true,
				ConsoleFormatIsRaw: false,
				Writer:             writer,
			}
			logger := logrus.New()
			logger.SetFormatter(&logrus.JSONFormatter{})
			entry := &logrus.Entry{
				Logger:  logger,
				Data:    logrus.Fields{"traceid": "trace-123", "panic_stack": "stack trace"},
				Context: context.Background(),
				Time:    time.Now(),
				Level:   logrus.ErrorLevel,
				Message: "panic message",
			}
			err := hook.consolePrint(entry, testCaller)
			c.So(err, c.ShouldBeNil)
			c.So(string(writer.written), c.ShouldContainSubstring, "panic message")
		})
	})
}

func TestTimeFormatter(t *testing.T) {
	mockey.PatchConvey("TestTimeFormatter", t, func() {
		mockey.PatchConvey("TestTimeFormatter-NilContext", func() {
			tf := timeFormatter{
				Formatter: &logrus.JSONFormatter{},
				Location:  nil,
			}
			entry := &logrus.Entry{
				Logger:  logrus.New(),
				Data:    logrus.Fields{},
				Context: nil,
				Time:    time.Now(),
				Level:   logrus.InfoLevel,
				Message: "test",
			}
			bytes, err := tf.Format(entry)
			c.So(err, c.ShouldBeNil)
			c.So(len(bytes), c.ShouldBeGreaterThan, 0)
		})

		mockey.PatchConvey("TestTimeFormatter-WithLocation", func() {
			loc, _ := time.LoadLocation("UTC")
			tf := timeFormatter{
				Formatter: &logrus.JSONFormatter{},
				Location:  loc,
			}
			entry := &logrus.Entry{
				Logger:  logrus.New(),
				Data:    logrus.Fields{},
				Context: context.Background(),
				Time:    time.Now(),
				Level:   logrus.InfoLevel,
				Message: "test",
			}
			bytes, err := tf.Format(entry)
			c.So(err, c.ShouldBeNil)
			c.So(len(bytes), c.ShouldBeGreaterThan, 0)
		})

		mockey.PatchConvey("TestTimeFormatter-MultipleCallsIdempotent", func() {
			loc, _ := time.LoadLocation("UTC")
			tf := timeFormatter{
				Formatter: &logrus.JSONFormatter{},
				Location:  loc,
			}
			entry := &logrus.Entry{
				Logger:  logrus.New(),
				Data:    logrus.Fields{},
				Context: context.Background(),
				Time:    time.Now(),
				Level:   logrus.InfoLevel,
				Message: "test",
			}
			// 多次调用 Format 应产生一致结果（幂等性）
			bytes1, err := tf.Format(entry)
			c.So(err, c.ShouldBeNil)
			bytes2, err := tf.Format(entry)
			c.So(err, c.ShouldBeNil)
			c.So(string(bytes1), c.ShouldEqual, string(bytes2))
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

	mockey.PatchConvey("TestInitXLogByConfig-RotatelogsFail", t, func() {
		mockey.Mock(xutil.DirExist).Return(true).Build()
		mockey.Mock(rotatelogs.New).Return(nil, errors.New("rotatelogs failed")).Build()

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
		c.So(err.Error(), c.ShouldContainSubstring, "rotatelogs.New failed")
	})

	mockey.PatchConvey("TestInitXLogByConfig-DefaultConsoleOnly", t, func() {
		// 默认配置不应创建日志目录、不应创建轮转文件
		mkdirMock := mockey.Mock(os.MkdirAll).Return(nil).Build()
		rotateMock := mockey.Mock(rotatelogs.New).Return(nil, errors.New("should not be called")).Build()

		config := configMergeDefault(nil)
		err := initXLogByConfig(config)
		c.So(err, c.ShouldBeNil)
		c.So(mkdirMock.Times(), c.ShouldEqual, 0)
		c.So(rotateMock.Times(), c.ShouldEqual, 0)
	})

	mockey.PatchConvey("TestInitXLogByConfig-NilConfig", t, func() {
		// 传入 nil 时走默认配置，不应 panic
		rotateMock := mockey.Mock(rotatelogs.New).Return(nil, errors.New("should not be called")).Build()

		err := initXLogByConfig(nil)
		c.So(err, c.ShouldBeNil)
		c.So(rotateMock.Times(), c.ShouldEqual, 0)
	})

	mockey.PatchConvey("TestInitXLogByConfig-NilEnableConsole", t, func() {
		// 未经 configMergeDefault 的 Config（EnableConsole 为 nil）不应 panic
		rotateMock := mockey.Mock(rotatelogs.New).Return(nil, errors.New("should not be called")).Build()

		err := initXLogByConfig(&Config{Level: "debug"})
		c.So(err, c.ShouldBeNil)
		c.So(rotateMock.Times(), c.ShouldEqual, 0)
	})

	mockey.PatchConvey("TestInitXLogByConfig-ConsoleOnly-InvalidTimezone", t, func() {
		// 仅控制台模式下时区加载失败也能正常初始化
		rotateMock := mockey.Mock(rotatelogs.New).Return(nil, errors.New("should not be called")).Build()

		config := configMergeDefault(&Config{Timezone: "Invalid/Zone"})
		err := initXLogByConfig(config)
		c.So(err, c.ShouldBeNil)
		c.So(rotateMock.Times(), c.ShouldEqual, 0)
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
