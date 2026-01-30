package xlog

import (
	"context"
	"errors"
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/xiaoshicae/xone/xconfig"
	"github.com/xiaoshicae/xone/xutil"

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
			Name:               "app",
			Path:               "./log",
			Console:            false,
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
			Name:               "2",
			Path:               "3",
			Console:            true,
			ConsoleFormatIsRaw: true,
			MaxAge:             "4",
			RotateTime:         "5",
			Timezone:           "UTC",
		}
		config = configMergeDefault(config)
		c.So(config, c.ShouldResemble, &Config{
			Level:              "1",
			Name:               "2",
			Path:               "3",
			Console:            true,
			ConsoleFormatIsRaw: true,
			MaxAge:             "4",
			RotateTime:         "5",
			Timezone:           "UTC",
		})
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
			newCtx := CtxWithKV(ctx, map[string]interface{}{"key": "value"})
			c.So(newCtx, c.ShouldNotBeNil)
			kv := newCtx.Value(XLogCtxKVContainerKey).(map[string]interface{})
			c.So(kv["key"], c.ShouldEqual, "value")
		})

		mockey.PatchConvey("TestCtxWithKV-MergeKV", func() {
			ctx := context.Background()
			ctx = CtxWithKV(ctx, map[string]interface{}{"key1": "value1"})
			ctx = CtxWithKV(ctx, map[string]interface{}{"key2": "value2"})
			kv := ctx.Value(XLogCtxKVContainerKey).(map[string]interface{})
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
			RawLog(ctx, logrus.InfoLevel, "test message", "arg1", KVMap(map[string]interface{}{"key": "value"}))
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
			KVMap(map[string]interface{}{"k1": "v1", "k2": "v2"})(opt)
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
			funcVal, fileVal := callerPretty(nil)
			c.So(funcVal, c.ShouldEqual, "???")
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
			ctx = CtxWithKV(ctx, map[string]interface{}{"key": "value"})
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
			ctx := CtxWithKV(context.Background(), map[string]interface{}{"custom": "value"})
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
				IP:         "127.0.0.1",
				ServerName: "test-server",
				PidStr:     "12345",
				Console:    true,
				Writer:     writer,
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
		mockey.PatchConvey("TestConsolePrint-Raw", func() {
			writer := &mockWriter{}
			hook := &xLogHook{
				IP:                 "127.0.0.1",
				ServerName:         "test-server",
				PidStr:             "12345",
				Console:            true,
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
			err := hook.ConsolePrint(entry)
			c.So(err, c.ShouldBeNil)
			c.So(len(writer.written), c.ShouldBeGreaterThan, 0)
		})

		mockey.PatchConvey("TestConsolePrint-Formatted", func() {
			writer := &mockWriter{}
			hook := &xLogHook{
				IP:                 "127.0.0.1",
				ServerName:         "test-server",
				PidStr:             "12345",
				Console:            true,
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
			err := hook.ConsolePrint(entry)
			c.So(err, c.ShouldBeNil)
			c.So(len(writer.written), c.ShouldBeGreaterThan, 0)
		})

		mockey.PatchConvey("TestConsolePrint-WithPanicStack", func() {
			writer := &mockWriter{}
			hook := &xLogHook{
				IP:                 "127.0.0.1",
				ServerName:         "test-server",
				PidStr:             "12345",
				Console:            true,
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
			err := hook.ConsolePrint(entry)
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

		mockey.PatchConvey("TestTimeFormatter-AlreadyFormatted", func() {
			tf := timeFormatter{
				Formatter: &logrus.JSONFormatter{},
				Location:  nil,
			}
			ctx := context.WithValue(context.Background(), timeFormatedCtxKey, true)
			entry := &logrus.Entry{
				Logger:  logrus.New(),
				Data:    logrus.Fields{},
				Context: ctx,
				Time:    time.Now(),
				Level:   logrus.InfoLevel,
				Message: "test",
			}
			bytes, err := tf.Format(entry)
			c.So(err, c.ShouldBeNil)
			c.So(len(bytes), c.ShouldBeGreaterThan, 0)
		})
	})
}

func TestInitXLogByConfig(t *testing.T) {
	mockey.PatchConvey("TestInitXLogByConfig-DirNotExist-MkdirFail", t, func() {
		mockey.Mock(xutil.DirExist).Return(false).Build()
		mockey.Mock(os.MkdirAll).Return(errors.New("mkdir failed")).Build()

		config := &Config{
			Path: "/test/path",
		}
		err := initXLogByConfig(config)
		c.So(err, c.ShouldNotBeNil)
		c.So(err.Error(), c.ShouldContainSubstring, "os.MkdirAll failed")
	})

	mockey.PatchConvey("TestInitXLogByConfig-RotatelogsFail", t, func() {
		mockey.Mock(xutil.DirExist).Return(true).Build()
		mockey.Mock(rotatelogs.New).Return(nil, errors.New("rotatelogs failed")).Build()

		config := &Config{
			Path:       "/test/path",
			Name:       "test",
			MaxAge:     "7d",
			RotateTime: "1d",
		}
		err := initXLogByConfig(config)
		c.So(err, c.ShouldNotBeNil)
		c.So(err.Error(), c.ShouldContainSubstring, "rotatelogs.New failed")
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
