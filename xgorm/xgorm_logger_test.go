package xgorm

import (
	"context"
	"errors"
	"testing"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	. "github.com/bytedance/mockey"
	c "github.com/smartystreets/goconvey/convey"

	"github.com/xiaoshicae/xone/v2/xlog"
)

// captureXLog 拦截 xlog 的四个级别，返回按级别收集到的消息
type capturedLogs struct {
	info  []string
	warn  []string
	err   []string
	debug []string
}

func captureXLog() *capturedLogs {
	got := &capturedLogs{}
	Mock(xlog.Info).To(func(_ context.Context, msg string, _ ...any) { got.info = append(got.info, msg) }).Build()
	Mock(xlog.Warn).To(func(_ context.Context, msg string, _ ...any) { got.warn = append(got.warn, msg) }).Build()
	Mock(xlog.Error).To(func(_ context.Context, msg string, _ ...any) { got.err = append(got.err, msg) }).Build()
	Mock(xlog.Debug).To(func(_ context.Context, msg string, _ ...any) { got.debug = append(got.debug, msg) }).Build()
	return got
}

func TestResolveLoglevel(t *testing.T) {
	PatchConvey("TestResolveLoglevel", t, func() {
		PatchConvey("已知级别，大小写不敏感", func() {
			c.So(resolveLoglevel("info"), c.ShouldEqual, logger.Info)
			c.So(resolveLoglevel("WARN"), c.ShouldEqual, logger.Warn)
			c.So(resolveLoglevel("warning"), c.ShouldEqual, logger.Warn)
			c.So(resolveLoglevel("Error"), c.ShouldEqual, logger.Error)
		})

		PatchConvey("未知级别回退到 Info", func() {
			// debug/trace 在 gorm 侧没有对应级别，回退到最详细的 Info
			c.So(resolveLoglevel("debug"), c.ShouldEqual, logger.Info)
			c.So(resolveLoglevel(""), c.ShouldEqual, logger.Info)
		})
	})
}

func TestFormatRows(t *testing.T) {
	PatchConvey("TestFormatRows", t, func() {
		// gorm 用 -1 表示行数未知，直接打出来会让人以为真的影响了 -1 行
		c.So(formatRows(-1), c.ShouldEqual, "-")
		c.So(formatRows(0), c.ShouldEqual, int64(0))
		c.So(formatRows(42), c.ShouldEqual, int64(42))
	})
}

func TestNewGormLogger(t *testing.T) {
	PatchConvey("TestNewGormLogger", t, func() {
		Mock(xlog.XLogLevel).Return("warn").Build()

		l := newGormLogger(&Config{SlowThreshold: "2s", IgnoreRecordNotFoundErrorLog: true})
		c.So(l.logLevel, c.ShouldEqual, logger.Warn)
		c.So(l.slowThreshold, c.ShouldEqual, 2*time.Second)
		c.So(l.ignoreRecordNotFoundError, c.ShouldBeTrue)
	})
}

func TestGormLoggerLogMode(t *testing.T) {
	PatchConvey("TestGormLoggerLogMode", t, func() {
		// GORM 约定 LogMode 返回新实例，就地改会让共享 logger 被并发改级别
		origin := &gormLogger{logLevel: logger.Info, slowThreshold: time.Second}
		next := origin.LogMode(logger.Error)

		c.So(next, c.ShouldNotEqual, origin)
		c.So(next.(*gormLogger).logLevel, c.ShouldEqual, logger.Error)
		c.So(next.(*gormLogger).slowThreshold, c.ShouldEqual, time.Second)
		c.So(origin.logLevel, c.ShouldEqual, logger.Info) // 原实例不受影响
	})
}

func TestGormLoggerLevels(t *testing.T) {
	PatchConvey("TestGormLoggerLevels", t, func() {
		got := captureXLog()
		l := &gormLogger{}
		ctx := context.Background()

		l.Info(ctx, "info msg")
		l.Warn(ctx, "warn msg")
		l.Error(ctx, "error msg")

		c.So(got.info, c.ShouldResemble, []string{"info msg"})
		c.So(got.warn, c.ShouldResemble, []string{"warn msg"})
		c.So(got.err, c.ShouldResemble, []string{"error msg"})
	})
}

func TestGormLoggerTrace(t *testing.T) {
	PatchConvey("TestGormLoggerTrace", t, func() {
		fc := func() (string, int64) { return "SELECT 1", 1 }
		ctx := context.Background()

		PatchConvey("出错时记 Error", func() {
			got := captureXLog()
			l := &gormLogger{logLevel: logger.Error}
			l.Trace(ctx, time.Now(), fc, errors.New("boom"))
			c.So(len(got.err), c.ShouldEqual, 1)
			c.So(got.err[0], c.ShouldContainSubstring, "rowsAffected")
		})

		PatchConvey("级别低于 Error 时不记错误", func() {
			got := captureXLog()
			l := &gormLogger{logLevel: logger.Silent}
			l.Trace(ctx, time.Now(), fc, errors.New("boom"))
			c.So(got.err, c.ShouldBeEmpty)
		})

		PatchConvey("ErrRecordNotFound 按配置忽略", func() {
			PatchConvey("忽略时不记", func() {
				got := captureXLog()
				l := &gormLogger{logLevel: logger.Error, ignoreRecordNotFoundError: true}
				l.Trace(ctx, time.Now(), fc, gorm.ErrRecordNotFound)
				c.So(got.err, c.ShouldBeEmpty)
			})

			PatchConvey("不忽略时照常记", func() {
				got := captureXLog()
				l := &gormLogger{logLevel: logger.Error, ignoreRecordNotFoundError: false}
				l.Trace(ctx, time.Now(), fc, gorm.ErrRecordNotFound)
				c.So(len(got.err), c.ShouldEqual, 1)
			})
		})

		PatchConvey("慢查询记 Warn", func() {
			got := captureXLog()
			l := &gormLogger{logLevel: logger.Warn, slowThreshold: time.Nanosecond}
			l.Trace(ctx, time.Now().Add(-time.Second), fc, nil)
			c.So(len(got.warn), c.ShouldEqual, 1)
			c.So(got.warn[0], c.ShouldContainSubstring, "SLOW SQL")
		})

		PatchConvey("阈值为 0 表示不判慢查询", func() {
			got := captureXLog()
			l := &gormLogger{logLevel: logger.Warn, slowThreshold: 0}
			l.Trace(ctx, time.Now().Add(-time.Hour), fc, nil)
			c.So(got.warn, c.ShouldBeEmpty)
		})

		PatchConvey("Info 级别记录每条 SQL", func() {
			got := captureXLog()
			l := &gormLogger{logLevel: logger.Info, slowThreshold: time.Hour}
			l.Trace(ctx, time.Now(), fc, nil)
			c.So(len(got.info), c.ShouldEqual, 1)
			c.So(got.info[0], c.ShouldContainSubstring, "sql")
		})

		PatchConvey("Warn 级别且不慢时什么都不记", func() {
			got := captureXLog()
			l := &gormLogger{logLevel: logger.Warn, slowThreshold: time.Hour}
			l.Trace(ctx, time.Now(), fc, nil)
			c.So(got.info, c.ShouldBeEmpty)
			c.So(got.warn, c.ShouldBeEmpty)
			c.So(got.err, c.ShouldBeEmpty)
		})

		PatchConvey("行数未知时显示 -", func() {
			// gorm 用 -1 表示行数未知，直接打出来会让人以为真的影响了 -1 行
			var captured []any
			Mock(xlog.Info).To(func(_ context.Context, _ string, args ...any) { captured = args }).Build()

			l := &gormLogger{logLevel: logger.Info}
			l.Trace(ctx, time.Now(), func() (string, int64) { return "SELECT 1", -1 }, nil)
			c.So(captured, c.ShouldContain, "-")
		})
	})
}
