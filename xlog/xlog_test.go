package xlog

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"runtime"
	"strconv"
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
			Level:    "info",
			Timezone: "Asia/Shanghai",
			Console: ConsoleConfig{
				Enable: xutil.ToPtr(true),
				Format: FormatText,
			},
			File: FileConfig{
				Enable:     false,
				Path:       "./log",
				Name:       "app",
				MaxAge:     "7d",
				RotateTime: "1d",
				Perm:       defaultFilePerm,
			},
		})
	})

	mockey.PatchConvey("TestXLogConfig-configMergeDefault-NotNil", t, func() {
		mockey.Mock(xconfig.GetServerName).Return("a.b.c").Build()
		want := &Config{
			Level:    "1",
			Timezone: "UTC",
			Console: ConsoleConfig{
				Enable: xutil.ToPtr(true),
				Format: FormatJSON,
			},
			File: FileConfig{
				Enable:     true,
				Path:       "3",
				Name:       "2",
				MaxAge:     "4",
				RotateTime: "5",
				Perm:       defaultFilePerm,
			},
		}
		// RotateTime "5" 会被解析为 5 纳秒，低于下限，回退到默认值
		want.File.RotateTime = defaultRotateTime
		config := configMergeDefault(&Config{
			Level:    "1",
			Timezone: "UTC",
			Console:  ConsoleConfig{Enable: xutil.ToPtr(true), Format: FormatJSON},
			File:     FileConfig{Enable: true, Path: "3", Name: "2", MaxAge: "4", RotateTime: "5"},
		})
		c.So(config, c.ShouldResemble, want)
	})

	mockey.PatchConvey("TestXLogConfig-configMergeDefault-DefaultIsConsoleOnly", t, func() {
		// 默认不写文件，仅打印到控制台
		config := configMergeDefault(&Config{})
		c.So(config.File.Enable, c.ShouldBeFalse)
		c.So(*config.Console.Enable, c.ShouldBeTrue)
	})

	mockey.PatchConvey("TestXLogConfig-configMergeDefault-FileOnly", t, func() {
		// 开启文件写入后，显式关闭的 EnableConsole 不会被强制打开
		config := configMergeDefault(&Config{File: FileConfig{Enable: true}, Console: ConsoleConfig{Enable: xutil.ToPtr(false)}})
		c.So(config.File.Enable, c.ShouldBeTrue)
		c.So(*config.Console.Enable, c.ShouldBeFalse)
	})

	mockey.PatchConvey("TestXLogConfig-configMergeDefault-BothDisabledForceConsole", t, func() {
		// 文件和控制台都关闭时强制打开控制台，避免日志无处输出
		config := configMergeDefault(&Config{Console: ConsoleConfig{Enable: xutil.ToPtr(false)}})
		c.So(*config.Console.Enable, c.ShouldBeTrue)
	})

	mockey.PatchConvey("TestXLogConfig-configMergeDefault-InvalidRotateTime", t, func() {
		// 回归：无法解析的轮转周期会让 ToDuration 返回 0，
		// 进而退化为按分钟切割（一天上千个文件），必须回退到默认值
		// "5" 无单位会被解析为 5 纳秒，"30s" 低于一分钟下限，均无法兑现
		for _, bad := range []string{"abc", "-1d", "0", "5", "30s"} {
			config := configMergeDefault(&Config{File: FileConfig{RotateTime: bad}})
			c.So(config.File.RotateTime, c.ShouldEqual, defaultRotateTime)
			c.So(xutil.ToDuration(config.File.RotateTime), c.ShouldBeGreaterThan, 0)
		}
	})

	mockey.PatchConvey("TestXLogConfig-configMergeDefault-ValidRotateTimeKept", t, func() {
		for _, ok := range []string{"1d", "6h", "30m", "1m", "2d12h"} {
			c.So(configMergeDefault(&Config{File: FileConfig{RotateTime: ok}}).File.RotateTime, c.ShouldEqual, ok)
		}
	})

	mockey.PatchConvey("TestXLogConfig-configMergeDefault-NegativeMaxAge", t, func() {
		// 负数保留时长会让历史文件立即过期
		c.So(configMergeDefault(&Config{File: FileConfig{MaxAge: "-1d"}}).File.MaxAge, c.ShouldEqual, defaultMaxAge)
		// 0 表示不清理，是合法配置，应保留
		c.So(configMergeDefault(&Config{File: FileConfig{MaxAge: "0"}}).File.MaxAge, c.ShouldEqual, "0")
	})

	mockey.PatchConvey("TestXLogConfig-configMergeDefault-Idempotent", t, func() {
		// 重复合并结果一致，initXLogByConfig 的兜底调用依赖该性质
		once := configMergeDefault(&Config{Level: "debug", File: FileConfig{Enable: true}, Console: ConsoleConfig{Enable: xutil.ToPtr(false)}})
		twice := configMergeDefault(once)
		c.So(twice, c.ShouldResemble, once)
	})

	mockey.PatchConvey("TestXLogConfig-configMergeDefault-ForceConsoleKeepFormat", t, func() {
		// 强制打开 EnableConsole 不影响用户配置的输出格式
		config := configMergeDefault(&Config{Console: ConsoleConfig{Enable: xutil.ToPtr(false), Format: FormatJSON}})
		c.So(*config.Console.Enable, c.ShouldBeTrue)
		c.So(config.Console.IsJSON(), c.ShouldBeTrue)
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
			kv := KVFromCtx(newCtx)
			c.So(kv["key"], c.ShouldEqual, "value")
		})

		mockey.PatchConvey("TestCtxWithKV-MergeKV", func() {
			ctx := context.Background()
			ctx = CtxWithKV(ctx, map[string]any{"key1": "value1"})
			ctx = CtxWithKV(ctx, map[string]any{"key2": "value2"})
			kv := KVFromCtx(ctx)
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

func TestCurrentLevel(t *testing.T) {
	mockey.PatchConvey("TestCurrentLevel", t, func() {
		old := currentLevel.Load()
		defer currentLevel.Store(old)

		mockey.PatchConvey("TestCurrentLevel-Default", func() {
			currentLevel.Store(uint32(InfoLevel))
			c.So(CurrentLevel().String(), c.ShouldEqual, "info")
			c.So(CurrentLevel(), c.ShouldEqual, InfoLevel)
		})

		mockey.PatchConvey("TestCurrentLevel-FromConfig", func() {
			// 级别来自初始化，而非运行时回查 xconfig
			c.So(initXLogByConfig(&Config{Level: "debug"}), c.ShouldBeNil)
			c.So(CurrentLevel().String(), c.ShouldEqual, "debug")
		})

		mockey.PatchConvey("TestCurrentLevel-UnknownFallbackInfo", func() {
			c.So(initXLogByConfig(&Config{Level: "not-a-level"}), c.ShouldBeNil)
			c.So(CurrentLevel().String(), c.ShouldEqual, "info")
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

func TestScopeFromCtx(t *testing.T) {
	mockey.PatchConvey("TestScopeFromCtx", t, func() {
		mockey.PatchConvey("TestScopeFromCtx-Empty", func() {
			c.So(scopeFromCtx(context.Background()), c.ShouldBeNil)
		})

		mockey.PatchConvey("TestScopeFromCtx-NilCtx", func() {
			c.So(scopeFromCtx(nil), c.ShouldBeNil)
		})

		mockey.PatchConvey("TestScopeFromCtx-WithKV", func() {
			ctx := CtxWithKV(context.Background(), map[string]any{"key": "value"})
			s := scopeFromCtx(ctx)
			c.So(s, c.ShouldNotBeNil)
			c.So(s.snapshot()["key"], c.ShouldEqual, "value")
		})
	})
}

func TestRangeCtxKV(t *testing.T) {
	mockey.PatchConvey("TestRangeCtxKV", t, func() {
		// 日志热路径走它而不是 KVFromCtx，避免每行日志复制一次 map
		mockey.PatchConvey("TestRangeCtxKV-无作用域时不回调", func() {
			called := 0
			rangeCtxKV(context.Background(), func(string, any) { called++ })
			c.So(called, c.ShouldEqual, 0)
		})

		mockey.PatchConvey("TestRangeCtxKV-遍历全部", func() {
			ctx := CtxWithKV(context.Background(), map[string]any{"a": 1, "b": 2})
			got := map[string]any{}
			rangeCtxKV(ctx, func(k string, v any) { got[k] = v })
			c.So(got, c.ShouldResemble, map[string]any{"a": 1, "b": 2})
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
			mockey.Mock(xutil.GetTraceAndSpanIDFromCtx).Return("trace-123", "").Build()
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
			mockey.Mock(xutil.GetTraceAndSpanIDFromCtx).Return("from-ctx", "").Build()
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
			h := &xHandler{consoleWriter: consoleW, consoleJSON: true, level: slogLevelTrace}
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
			File:    FileConfig{Enable: true, Path: "/test/path"},
			Console: ConsoleConfig{Enable: xutil.ToPtr(false)},
		}
		err := initXLogByConfig(config)
		c.So(err, c.ShouldNotBeNil)
		c.So(err.Error(), c.ShouldContainSubstring, "os.MkdirAll failed")
	})

	mockey.PatchConvey("TestInitXLogByConfig-OpenFileFail", t, func() {
		mockey.Mock(xutil.DirExist).Return(true).Build()
		mockey.Mock(newRotateWriter).Return(nil, errors.New("open log file failed")).Build()

		config := &Config{
			File:    FileConfig{Enable: true, Path: "/test/path", Name: "test", MaxAge: "7d", RotateTime: "1d"},
			Console: ConsoleConfig{Enable: xutil.ToPtr(false)},
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
			swapFileWriter(newAsyncWriter(mw, 8))
			c.So(closeFileWriter(), c.ShouldBeNil)
			c.So(mw.closed, c.ShouldBeTrue)
			// 重复关闭安全
			c.So(closeFileWriter(), c.ShouldBeNil)
		})

		mockey.PatchConvey("TestSwapFileWriter-ReplacesAndClosesPrevious", func() {
			// 重复初始化不应泄漏上一个写入器的 goroutine
			first, second := &mockWriteCloser{}, &mockWriteCloser{}
			swapFileWriter(newAsyncWriter(first, 8))
			swapFileWriter(newAsyncWriter(second, 8))
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
	mu       sync.Mutex
	written  []byte
	closed   bool
	writeErr error
	closeErr error
}

func (m *mockWriteCloser) Write(p []byte) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.writeErr != nil {
		return 0, m.writeErr
	}
	m.written = append(m.written, p...)
	return len(p), nil
}

func (m *mockWriteCloser) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	return m.closeErr
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
		c.So(config.File.Name, c.ShouldEqual, "app")
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

// TestReinitKeepsWritesAlive 回归防护：
// 重复初始化时若先关闭旧写入器再切换 handler，切换窗口内的日志会写入已关闭的
// 写入器而丢失。此处断言新 handler 生效后旧写入器才被关闭。
func TestReinitKeepsWritesAlive(t *testing.T) {
	mockey.PatchConvey("TestReinitKeepsWritesAlive", t, func() {
		dir := t.TempDir()
		cfg := &Config{File: FileConfig{Enable: true, Path: dir, Name: "app"}, Console: ConsoleConfig{Enable: xutil.ToPtr(false)}}

		c.So(initXLogByConfig(cfg), c.ShouldBeNil)
		firstHandler := handler.Load()
		c.So(firstHandler.fileWriter, c.ShouldNotBeNil)

		// 再次初始化
		c.So(initXLogByConfig(cfg), c.ShouldBeNil)
		secondHandler := handler.Load()

		// handler 已替换，且新 handler 的写入器可用
		c.So(secondHandler, c.ShouldNotEqual, firstHandler)
		n, err := secondHandler.fileWriter.Write([]byte("{}\n"))
		c.So(err, c.ShouldBeNil)
		c.So(n, c.ShouldBeGreaterThan, 0)

		c.So(closeFileWriter(), c.ShouldBeNil)
	})
}

func TestRawLogNilCtx(t *testing.T) {
	mockey.PatchConvey("TestRawLogNilCtx", t, func() {
		// ctx 为 nil 是调用方的疏忽，但为此丢掉一整条（可能是 Error 级的）日志
		// 代价太大，用 Background 兜底，日志照常输出
		fileW := &mockWriter{}
		handler.Store(&xHandler{fileWriter: fileW, level: slogLevelTrace})

		//nolint:staticcheck // 有意传入 nil 验证兜底行为
		RawLog(nil, InfoLevel, "仍应输出")
		c.So(string(fileW.written), c.ShouldContainSubstring, "仍应输出")
	})
}

func TestHandlerAndLoggerAccessors(t *testing.T) {
	mockey.PatchConvey("TestHandlerAndLoggerAccessors", t, func() {
		fileW := &mockWriter{}
		h := &xHandler{fileWriter: fileW, level: slogLevelTrace}
		handler.Store(h)

		mockey.PatchConvey("Handler 返回当前处理器", func() {
			c.So(Handler(), c.ShouldEqual, h)
		})

		mockey.PatchConvey("Logger 可直接交给 slog 使用", func() {
			l := Logger()
			c.So(l, c.ShouldNotBeNil)
			l.Info("来自 slog.Logger")
			c.So(string(fileW.written), c.ShouldContainSubstring, "来自 slog.Logger")
		})
	})
}

func TestReplaceAttr(t *testing.T) {
	mockey.PatchConvey("TestReplaceAttr", t, func() {
		mockey.PatchConvey("分组内字段保持原样", func() {
			a := slog.String(slog.TimeKey, "原值")
			c.So(replaceAttr([]string{"g"}, a), c.ShouldResemble, a)
		})

		mockey.PatchConvey("level 值类型异常时原样返回", func() {
			a := slog.String(slog.LevelKey, "不是 slog.Level")
			c.So(replaceAttr(nil, a), c.ShouldResemble, a)
		})

		mockey.PatchConvey("其他字段不受影响", func() {
			a := slog.String("custom", "v")
			c.So(replaceAttr(nil, a), c.ShouldResemble, a)
		})
	})
}

func TestHandlerErrorPaths(t *testing.T) {
	newRec := func() slog.Record { return slog.NewRecord(time.Now(), slog.LevelInfo, "m", 0) }

	mockey.PatchConvey("TestHandlerErrorPaths", t, func() {
		mockey.PatchConvey("JSON 序列化失败时控制台仍照常输出", func() {
			// slog 的 JSONHandler 对任何值都不返回错误（内部有兜底），
			// 该分支只能通过 mock 触发，但仍需保证失败时不连累控制台输出
			mockey.Mock((*slog.JSONHandler).Handle).Return(errors.New("encode failed")).Build()

			fileW, consoleW := &mockWriter{}, &mockWriter{}
			h := &xHandler{fileWriter: fileW, consoleWriter: consoleW, level: slogLevelTrace}

			err := h.Handle(context.Background(), newRec())
			c.So(err, c.ShouldNotBeNil)
			c.So(err.Error(), c.ShouldContainSubstring, "encode failed")
			// JSON 未产出，文件侧不应写入半成品
			c.So(fileW.written, c.ShouldBeEmpty)
			// 控制台的可读格式不依赖 JSON，应仍然写出
			c.So(string(consoleW.written), c.ShouldContainSubstring, "m")
		})

		mockey.PatchConvey("文件与控制台同时失败时保留先发生的错误", func() {
			fileErr := errors.New("disk full")
			h := &xHandler{
				fileWriter:    &errWriter{err: fileErr},
				consoleWriter: &errWriter{err: errors.New("broken pipe")},
				level:         slogLevelTrace,
			}
			c.So(h.Handle(context.Background(), newRec()), c.ShouldEqual, fileErr)
		})

		mockey.PatchConvey("仅控制台失败时返回控制台错误", func() {
			consoleErr := errors.New("broken pipe")
			h := &xHandler{consoleWriter: &errWriter{err: consoleErr}, level: slogLevelTrace}
			c.So(h.Handle(context.Background(), newRec()), c.ShouldEqual, consoleErr)
		})

		mockey.PatchConvey("超大日志行的编码器不归还池", func() {
			fileW := &mockWriter{}
			h := &xHandler{fileWriter: fileW, level: slogLevelTrace}
			r := slog.NewRecord(time.Now(), slog.LevelInfo, strings.Repeat("x", maxPoolBufSize+1), 0)
			c.So(h.Handle(context.Background(), r), c.ShouldBeNil)
			c.So(len(fileW.written), c.ShouldBeGreaterThan, maxPoolBufSize)
		})
	})
}

func TestInitXLog(t *testing.T) {
	mockey.PatchConvey("TestInitXLog", t, func() {
		mockey.PatchConvey("配置读取失败时返回错误", func() {
			mockey.Mock(xconfig.UnmarshalConfig).Return(errors.New("unmarshal failed")).Build()
			err := initXLog()
			c.So(err, c.ShouldNotBeNil)
			c.So(err.Error(), c.ShouldContainSubstring, "getConfig failed")
		})

		mockey.PatchConvey("配置正常时完成初始化", func() {
			mockey.Mock(xconfig.UnmarshalConfig).Return(nil).Build()
			c.So(initXLog(), c.ShouldBeNil)
			c.So(handler.Load(), c.ShouldNotBeNil)
		})
	})
}

func TestSwapFileWriterCloseError(t *testing.T) {
	mockey.PatchConvey("TestSwapFileWriterCloseError", t, func() {
		// 旧写入器关闭失败不应影响新写入器生效
		failing := &mockWriteCloser{closeErr: errors.New("close failed")}
		swapFileWriter(newAsyncWriter(failing, 8))

		next := &mockWriteCloser{}
		swapFileWriter(newAsyncWriter(next, 8))
		c.So(failing.closed, c.ShouldBeTrue)

		c.So(closeFileWriter(), c.ShouldBeNil)
	})
}

func TestAsyncWriterEdgeCases(t *testing.T) {
	mockey.PatchConvey("TestAsyncWriterEdgeCases", t, func() {
		mockey.PatchConvey("关闭后写入返回错误", func() {
			aw := newAsyncWriter(&mockWriteCloser{}, 8)
			c.So(aw.Close(), c.ShouldBeNil)

			n, err := aw.Write([]byte("late"))
			c.So(n, c.ShouldEqual, 0)
			c.So(err, c.ShouldEqual, errAsyncWriterClosed)
		})

		mockey.PatchConvey("缓冲区满时被关闭则放弃该条日志", func() {
			// 底层写入阻塞，缓冲区填满后 Write 会阻塞在发送上
			release := make(chan struct{})
			mw := &blockingWriteCloser{release: release}
			aw := newAsyncWriter(mw, 1)

			_, _ = aw.Write([]byte("first"))  // 被消费协程取走后阻塞在底层写入
			_, _ = aw.Write([]byte("second")) // 占满缓冲区

			errCh := make(chan error, 1)
			go func() {
				_, err := aw.Write([]byte("third")) // 阻塞在发送
				errCh <- err
			}()

			// 关闭应让阻塞中的 Write 立即返回
			go func() {
				time.Sleep(20 * time.Millisecond)
				close(release)
				_ = aw.Close()
			}()

			select {
			case err := <-errCh:
				// 要么在关闭前成功入队，要么因关闭被放弃，两者都可接受
				if err != nil {
					c.So(err, c.ShouldEqual, errAsyncWriterClosed)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("关闭后 Write 仍未返回")
			}
		})

		mockey.PatchConvey("底层关闭失败时返回该错误", func() {
			closeErr := errors.New("close failed")
			aw := newAsyncWriter(&mockWriteCloser{closeErr: closeErr}, 8)
			c.So(aw.Close(), c.ShouldEqual, closeErr)
			// 重复关闭返回同一错误
			c.So(aw.Close(), c.ShouldEqual, closeErr)
		})

		mockey.PatchConvey("底层写入失败由 Close 返回", func() {
			writeErr := errors.New("disk full")
			aw := newAsyncWriter(&mockWriteCloser{writeErr: writeErr}, 8)
			_, _ = aw.Write([]byte("a"))
			_, _ = aw.Write([]byte("b"))
			c.So(aw.Close(), c.ShouldEqual, writeErr)
		})

		mockey.PatchConvey("超大数据不复用池中 buffer", func() {
			mw := &mockWriteCloser{}
			aw := newAsyncWriter(mw, 8)
			big := bytes.Repeat([]byte("x"), maxPoolBufSize+1)

			n, err := aw.Write(big)
			c.So(err, c.ShouldBeNil)
			c.So(n, c.ShouldEqual, len(big))
			c.So(aw.Close(), c.ShouldBeNil)
			c.So(len(mw.written), c.ShouldEqual, len(big))
		})
	})
}

// blockingWriteCloser 首次写入阻塞到 release 关闭，用于填满异步缓冲区
type blockingWriteCloser struct {
	release chan struct{}
	once    sync.Once
}

func (w *blockingWriteCloser) Write(p []byte) (int, error) {
	w.once.Do(func() { <-w.release })
	return len(p), nil
}

func (w *blockingWriteCloser) Close() error { return nil }

func TestConsoleConfigIsJSON(t *testing.T) {
	mockey.PatchConvey("TestConsoleConfigIsJSON", t, func() {
		c.So(ConsoleConfig{Format: FormatJSON}.IsJSON(), c.ShouldBeTrue)
		c.So(ConsoleConfig{Format: "JSON"}.IsJSON(), c.ShouldBeTrue)
		c.So(ConsoleConfig{Format: FormatText}.IsJSON(), c.ShouldBeFalse)
		c.So(ConsoleConfig{}.IsJSON(), c.ShouldBeFalse)
	})
}

func TestConsoleFormatFallback(t *testing.T) {
	mockey.PatchConvey("TestConsoleFormatFallback", t, func() {
		mockey.PatchConvey("未知格式回退为 text", func() {
			// 笔误不应让控制台意外变成 JSON 或反之
			config := configMergeDefault(&Config{Console: ConsoleConfig{Format: "yaml"}})
			c.So(config.Console.Format, c.ShouldEqual, FormatText)
		})

		mockey.PatchConvey("大小写不敏感且归一化", func() {
			c.So(configMergeDefault(&Config{Console: ConsoleConfig{Format: "JSON"}}).Console.Format, c.ShouldEqual, FormatJSON)
			c.So(configMergeDefault(&Config{Console: ConsoleConfig{Format: "TEXT"}}).Console.Format, c.ShouldEqual, FormatText)
		})
	})
}

// stuckWriteCloser 每次写入都阻塞，模拟磁盘满 / NFS 不响应
type stuckWriteCloser struct {
	release chan struct{}
}

func (s *stuckWriteCloser) Write(p []byte) (int, error) {
	<-s.release
	return len(p), nil
}

func (s *stuckWriteCloser) Close() error { return nil }

func TestAsyncWriterCloseTimeout(t *testing.T) {
	mockey.PatchConvey("TestAsyncWriterCloseTimeout", t, func() {
		mockey.PatchConvey("底层写入卡住时 Close 不会永久阻塞", func() {
			// xlog 是最后关闭的模块，卡在这里等于整个进程退不出去
			bw := &stuckWriteCloser{release: make(chan struct{})}
			defer close(bw.release)

			aw := newAsyncWriter(bw, 8)
			aw.drainTimeout = 50 * time.Millisecond
			_, _ = aw.Write([]byte("stuck\n"))

			done := make(chan error, 1)
			go func() { done <- aw.Close() }()

			select {
			case err := <-done:
				c.So(err, c.ShouldEqual, errAsyncWriterDrainTimeout)
			case <-time.After(3 * time.Second):
				t.Fatal("Close 没有在超时后返回")
			}
		})

		mockey.PatchConvey("正常写完时不报超时", func() {
			aw := newAsyncWriter(&mockWriteCloser{}, 8)
			aw.drainTimeout = 2 * time.Second
			_, _ = aw.Write([]byte("ok\n"))
			c.So(aw.Close(), c.ShouldBeNil)
		})
	})
}

func TestCloseFileWriterSwapsHandler(t *testing.T) {
	mockey.PatchConvey("TestCloseFileWriterSwapsHandler", t, func() {
		// xlog 是最后关闭的模块，但 xconfig 的保留层级更低，其关闭日志在此之后产生。
		// 不摘掉 fileWriter，这些日志会写进已关闭的写入器并静默失败
		mockey.PatchConvey("摘掉文件写入器并保留控制台", func() {
			consoleW := &mockWriter{}
			aw := newAsyncWriter(&mockWriteCloser{}, 8)
			handler.Store(&xHandler{
				consoleWriter: newLockedWriter(consoleW),
				fileWriter:    aw,
				level:         slogLevelTrace,
			})
			fileWriterMu.Lock()
			fileWriter = aw
			fileWriterMu.Unlock()

			c.So(closeFileWriter(), c.ShouldBeNil)
			c.So(handler.Load().fileWriter, c.ShouldBeNil)
			c.So(handler.Load().consoleWriter, c.ShouldNotBeNil)

			// 关闭之后的日志仍能落到标准输出
			RawLog(context.Background(), InfoLevel, "关闭阶段的日志")
			c.So(string(consoleW.written), c.ShouldContainSubstring, "关闭阶段的日志")
		})

		mockey.PatchConvey("原本只写文件时补上标准输出", func() {
			aw := newAsyncWriter(&mockWriteCloser{}, 8)
			handler.Store(&xHandler{fileWriter: aw, level: slogLevelTrace})
			fileWriterMu.Lock()
			fileWriter = aw
			fileWriterMu.Unlock()

			c.So(closeFileWriter(), c.ShouldBeNil)
			c.So(handler.Load().fileWriter, c.ShouldBeNil)
			// 否则关闭阶段的日志彻底无处可去
			c.So(handler.Load().consoleWriter, c.ShouldNotBeNil)
		})

		mockey.PatchConvey("没有文件写入器时是空操作", func() {
			handler.Store(&xHandler{level: slogLevelTrace})
			fileWriterMu.Lock()
			fileWriter = nil
			fileWriterMu.Unlock()
			c.So(closeFileWriter(), c.ShouldBeNil)
		})
	})
}

func TestFileConfigFileMode(t *testing.T) {
	mockey.PatchConvey("TestFileConfigFileMode", t, func() {
		c.So(FileConfig{Perm: "0644"}.FileMode(), c.ShouldEqual, os.FileMode(0o644))
		c.So(FileConfig{Perm: "0600"}.FileMode(), c.ShouldEqual, os.FileMode(0o600))
		c.So(FileConfig{Perm: "600"}.FileMode(), c.ShouldEqual, os.FileMode(0o600))
		// 笔误不应让日志文件变成不可读或全局可写
		c.So(FileConfig{Perm: "abc"}.FileMode(), c.ShouldEqual, os.FileMode(defaultLogFilePerm))
		c.So(FileConfig{Perm: ""}.FileMode(), c.ShouldEqual, os.FileMode(defaultLogFilePerm))
		c.So(FileConfig{Perm: "0"}.FileMode(), c.ShouldEqual, os.FileMode(defaultLogFilePerm))
	})
}

func TestAsyncWriterWaitDrainNoTimeout(t *testing.T) {
	mockey.PatchConvey("TestAsyncWriterWaitDrainNoTimeout", t, func() {
		// drainTimeout <= 0 表示不限时，退回无限等待
		aw := newAsyncWriter(&mockWriteCloser{}, 4)
		aw.drainTimeout = 0
		_, _ = aw.Write([]byte("x\n"))
		c.So(aw.Close(), c.ShouldBeNil)
	})
}

func TestRotateWriterWriteAfterFileGone(t *testing.T) {
	mockey.PatchConvey("TestRotateWriterWriteAfterFileGone", t, func() {
		// 轮转持续失败且此前没有可用句柄时，写入要报错而不是对 nil 解引用
		w := &rotateWriter{
			base:   "/nonexistent-dir/app.log",
			layout: rotateLayoutDay,
			rotate: 24 * time.Hour,
			clock:  time.Now,
			perm:   defaultLogFilePerm,
		}
		mockey.Mock(xutil.WarnIfEnableDebug).Return().Build()

		n, err := w.Write([]byte("x"))
		c.So(n, c.ShouldEqual, 0)
		c.So(err, c.ShouldEqual, os.ErrClosed)
	})
}

func TestCtxWithKVScope(t *testing.T) {
	mockey.PatchConvey("TestCtxWithKVScope", t, func() {
		mockey.PatchConvey("开启后可写入", func() {
			ctx := CtxWithKVScope(context.Background())
			AddKV(ctx, "userID", "u-1")
			c.So(KVFromCtx(ctx)["userID"], c.ShouldEqual, "u-1")
		})

		mockey.PatchConvey("幂等-重复开启不丢已写入的字段", func() {
			// 中间件可能被注册多次，重装一个空作用域会把前面写的全清掉
			ctx := CtxWithKVScope(context.Background())
			AddKV(ctx, "a", 1)

			ctx2 := CtxWithKVScope(ctx)
			c.So(ctx2, c.ShouldEqual, ctx) // 原样返回
			c.So(KVFromCtx(ctx2)["a"], c.ShouldEqual, 1)
		})

		mockey.PatchConvey("写入对同一 ctx 的所有持有方可见", func() {
			// 这正是 CtxWithKV 做不到、而访问日志需要的：
			// 业务函数在调用栈深处拿不到 *gin.Context，没机会回传新 ctx
			ctx := CtxWithKVScope(context.Background())

			deepInBusinessCode := func(ctx context.Context) { AddKV(ctx, "orderID", "o-9") }
			deepInBusinessCode(ctx)

			c.So(KVFromCtx(ctx)["orderID"], c.ShouldEqual, "o-9") // 入口处的 ctx 看得到
		})
	})
}

func TestAddKV(t *testing.T) {
	mockey.PatchConvey("TestAddKV", t, func() {
		mockey.PatchConvey("同名 key 覆盖", func() {
			ctx := CtxWithKVScope(context.Background())
			AddKV(ctx, "k", "old")
			AddKV(ctx, "k", "new")
			c.So(KVFromCtx(ctx)["k"], c.ShouldEqual, "new")
		})

		mockey.PatchConvey("无作用域时丢弃并打日志", func() {
			// 否则"日志里就是没有这个字段"没有任何线索
			warned := 0
			mockey.Mock(xutil.WarnIfEnableDebug).To(func(string, ...any) { warned++ }).Build()

			AddKV(context.Background(), "k", "v")
			c.So(warned, c.ShouldEqual, 1)
		})

		mockey.PatchConvey("AddKVs 批量写入", func() {
			ctx := CtxWithKVScope(context.Background())
			AddKVs(ctx, map[string]any{"a": 1, "b": 2})
			c.So(KVFromCtx(ctx), c.ShouldResemble, map[string]any{"a": 1, "b": 2})
		})

		mockey.PatchConvey("AddKVs 空入参直接返回，不触发告警", func() {
			warned := 0
			mockey.Mock(xutil.WarnIfEnableDebug).To(func(string, ...any) { warned++ }).Build()

			AddKVs(context.Background(), nil)
			AddKVs(context.Background(), map[string]any{})
			c.So(warned, c.ShouldEqual, 0)
		})

		mockey.PatchConvey("AddKVs 无作用域时丢弃并打日志", func() {
			warned := 0
			mockey.Mock(xutil.WarnIfEnableDebug).To(func(string, ...any) { warned++ }).Build()

			AddKVs(context.Background(), map[string]any{"a": 1})
			c.So(warned, c.ShouldEqual, 1)
		})
	})
}

func TestCtxWithKVIsDerived(t *testing.T) {
	mockey.PatchConvey("TestCtxWithKVIsDerived", t, func() {
		// CtxWithKV 与 CtxWithKVScope 的分工：前者派生快照，写入不影响传入的 ctx
		mockey.PatchConvey("派生后原 ctx 不受影响", func() {
			parent := CtxWithKVScope(context.Background())
			AddKV(parent, "a", 1)

			child := CtxWithKV(parent, map[string]any{"b": 2})

			c.So(KVFromCtx(child), c.ShouldResemble, map[string]any{"a": 1, "b": 2}) // 继承 + 新增
			c.So(KVFromCtx(parent), c.ShouldResemble, map[string]any{"a": 1})        // 原 ctx 不变
		})

		mockey.PatchConvey("向派生作用域写入不回流到原 ctx", func() {
			parent := CtxWithKVScope(context.Background())
			child := CtxWithKV(parent, nil)

			AddKV(child, "onlyChild", true)
			c.So(KVFromCtx(child)["onlyChild"], c.ShouldEqual, true)
			c.So(KVFromCtx(parent), c.ShouldBeEmpty)
		})

		mockey.PatchConvey("同名 key 以传入的为准", func() {
			parent := CtxWithKV(context.Background(), map[string]any{"k": "old"})
			child := CtxWithKV(parent, map[string]any{"k": "new"})
			c.So(KVFromCtx(child)["k"], c.ShouldEqual, "new")
		})
	})
}

func TestKVScopeConcurrent(t *testing.T) {
	mockey.PatchConvey("TestKVScopeConcurrent", t, func() {
		// handler 起协程打日志是常态：写入与日志读取会真并发
		ctx := CtxWithKVScope(context.Background())

		var wg sync.WaitGroup
		for i := range 50 {
			wg.Add(1)
			go func() { defer wg.Done(); AddKV(ctx, "k"+strconv.Itoa(i), i) }()
			wg.Add(1)
			go func() { defer wg.Done(); rangeCtxKV(ctx, func(string, any) {}) }()
		}
		wg.Wait()

		c.So(len(KVFromCtx(ctx)), c.ShouldEqual, 50)
	})
}
