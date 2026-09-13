package xlog

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bytedance/mockey"
	c "github.com/smartystreets/goconvey/convey"
	"github.com/xiaoshicae/xone/v2/xutil"
)

// newTestWriter 在新建临时目录中创建写入器并注入可控时间源
func newTestWriter(t *testing.T, maxAge, rotate time.Duration, now *time.Time) (*rotateWriter, string) {
	t.Helper()
	dir := t.TempDir()
	return newTestWriterInDir(t, dir, maxAge, rotate, now), dir
}

// newTestWriterInDir 在指定目录中创建写入器并注入可控时间源
// 重启场景必须复用同一目录且注入同一时间源，否则第二个写入器会按真实时间落到别的文件
func newTestWriterInDir(t *testing.T, dir string, maxAge, rotate time.Duration, now *time.Time) *rotateWriter {
	t.Helper()
	base := filepath.Join(dir, "app.log")

	w, err := newRotateWriter(base, maxAge, rotate)
	if err != nil {
		t.Fatalf("newRotateWriter failed: %v", err)
	}
	if now != nil {
		w.clock = func() time.Time { return *now }
		// 时间源注入后按当前时刻重新定位文件
		if name := w.filenameFor(*now); name != w.currentName {
			if err := w.rotateTo(name); err != nil {
				t.Fatalf("rotateTo failed: %v", err)
			}
		}
	}
	t.Cleanup(func() { _ = w.Close() })
	return w
}

func TestRotateWriter(t *testing.T) {
	mockey.PatchConvey("TestRotateWriter-WriteAndSymlink", t, func() {
		now := time.Date(2026, 9, 12, 10, 0, 0, 0, time.Local)
		w, dir := newTestWriter(t, 7*24*time.Hour, 24*time.Hour, &now)

		n, err := w.Write([]byte("hello\n"))
		c.So(err, c.ShouldBeNil)
		c.So(n, c.ShouldEqual, 6)

		// 文件名带日期后缀
		content, err := os.ReadFile(filepath.Join(dir, "app.log.20260912"))
		c.So(err, c.ShouldBeNil)
		c.So(string(content), c.ShouldEqual, "hello\n")

		// 符号链接指向当前文件
		target, err := os.Readlink(filepath.Join(dir, "app.log"))
		c.So(err, c.ShouldBeNil)
		c.So(target, c.ShouldEqual, "app.log.20260912")
	})

	mockey.PatchConvey("TestRotateWriter-RotatesOnPeriodChange", t, func() {
		now := time.Date(2026, 9, 12, 23, 59, 0, 0, time.Local)
		w, dir := newTestWriter(t, 7*24*time.Hour, 24*time.Hour, &now)

		_, err := w.Write([]byte("day1\n"))
		c.So(err, c.ShouldBeNil)

		// 跨到次日，应写入新文件
		now = now.Add(2 * time.Minute)
		_, err = w.Write([]byte("day2\n"))
		c.So(err, c.ShouldBeNil)

		day1, err := os.ReadFile(filepath.Join(dir, "app.log.20260912"))
		c.So(err, c.ShouldBeNil)
		c.So(string(day1), c.ShouldEqual, "day1\n")

		day2, err := os.ReadFile(filepath.Join(dir, "app.log.20260913"))
		c.So(err, c.ShouldBeNil)
		c.So(string(day2), c.ShouldEqual, "day2\n")

		// 符号链接跟随到最新文件
		target, _ := os.Readlink(filepath.Join(dir, "app.log"))
		c.So(target, c.ShouldEqual, "app.log.20260913")
	})

	mockey.PatchConvey("TestRotateWriter-SubDayRotation", t, func() {
		// 轮转周期小于一天时使用更细的时间后缀，否则会生成同名文件而无法真正轮转
		now := time.Date(2026, 9, 12, 10, 30, 0, 0, time.Local)
		w, dir := newTestWriter(t, 7*24*time.Hour, time.Hour, &now)

		_, err := w.Write([]byte("h10\n"))
		c.So(err, c.ShouldBeNil)

		now = now.Add(time.Hour)
		_, err = w.Write([]byte("h11\n"))
		c.So(err, c.ShouldBeNil)

		_, err = os.Stat(filepath.Join(dir, "app.log.2026091210"))
		c.So(err, c.ShouldBeNil)
		_, err = os.Stat(filepath.Join(dir, "app.log.2026091211"))
		c.So(err, c.ShouldBeNil)
	})

	mockey.PatchConvey("TestRotateWriter-AppendsToExistingFile", t, func() {
		// 同一周期内重启不应截断已有日志
		now := time.Date(2026, 9, 12, 10, 0, 0, 0, time.Local)
		w, dir := newTestWriter(t, 7*24*time.Hour, 24*time.Hour, &now)
		_, _ = w.Write([]byte("first\n"))
		c.So(w.Close(), c.ShouldBeNil)

		w2 := newTestWriterInDir(t, dir, 7*24*time.Hour, 24*time.Hour, &now)
		_, _ = w2.Write([]byte("second\n"))

		content, _ := os.ReadFile(w2.currentName)
		c.So(string(content), c.ShouldContainSubstring, "first")
		c.So(string(content), c.ShouldContainSubstring, "second")
	})

	mockey.PatchConvey("TestRotateWriter-CloseIsIdempotent", t, func() {
		now := time.Date(2026, 9, 12, 10, 0, 0, 0, time.Local)
		w, _ := newTestWriter(t, 0, 24*time.Hour, &now)
		c.So(w.Close(), c.ShouldBeNil)
		c.So(w.Close(), c.ShouldBeNil)

		_, err := w.Write([]byte("after close\n"))
		c.So(err, c.ShouldEqual, os.ErrClosed)
	})

	mockey.PatchConvey("TestRotateWriter-OpenFail", t, func() {
		// 目录不存在时应在构造阶段就报错，而非首次写日志才失败
		_, err := newRotateWriter(filepath.Join(t.TempDir(), "no-such-dir", "app.log"), time.Hour, time.Hour)
		c.So(err, c.ShouldNotBeNil)
		c.So(err.Error(), c.ShouldContainSubstring, "open log file failed")
	})
}

func TestRotateWriterPurge(t *testing.T) {
	mockey.PatchConvey("TestRotateWriterPurge-RemovesExpired", t, func() {
		now := time.Date(2026, 9, 12, 10, 0, 0, 0, time.Local)
		w, dir := newTestWriter(t, 48*time.Hour, 24*time.Hour, &now)

		// 造一个 10 天前的历史文件
		old := filepath.Join(dir, "app.log.20260902")
		c.So(os.WriteFile(old, []byte("old\n"), logFilePerm), c.ShouldBeNil)
		oldTime := now.Add(-10 * 24 * time.Hour)
		c.So(os.Chtimes(old, oldTime, oldTime), c.ShouldBeNil)

		w.purge(w.currentName)

		_, err := os.Stat(old)
		c.So(os.IsNotExist(err), c.ShouldBeTrue)
	})

	mockey.PatchConvey("TestRotateWriterPurge-KeepsRecentAndCurrent", t, func() {
		now := time.Date(2026, 9, 12, 10, 0, 0, 0, time.Local)
		w, dir := newTestWriter(t, 48*time.Hour, 24*time.Hour, &now)

		recent := filepath.Join(dir, "app.log.20260911")
		c.So(os.WriteFile(recent, []byte("recent\n"), logFilePerm), c.ShouldBeNil)
		recentTime := now.Add(-1 * time.Hour)
		c.So(os.Chtimes(recent, recentTime, recentTime), c.ShouldBeNil)

		w.purge(w.currentName)

		_, err := os.Stat(recent)
		c.So(err, c.ShouldBeNil)
		// 当前文件与符号链接都不应被删除
		_, err = os.Stat(w.currentName)
		c.So(err, c.ShouldBeNil)
		_, err = os.Lstat(filepath.Join(dir, "app.log"))
		c.So(err, c.ShouldBeNil)
	})

	mockey.PatchConvey("TestRotateWriterPurge-DisabledWhenMaxAgeZero", t, func() {
		now := time.Date(2026, 9, 12, 10, 0, 0, 0, time.Local)
		w, dir := newTestWriter(t, 0, 24*time.Hour, &now)

		old := filepath.Join(dir, "app.log.20250101")
		c.So(os.WriteFile(old, []byte("old\n"), logFilePerm), c.ShouldBeNil)
		oldTime := now.Add(-365 * 24 * time.Hour)
		c.So(os.Chtimes(old, oldTime, oldTime), c.ShouldBeNil)

		w.purge(w.currentName)

		_, err := os.Stat(old)
		c.So(err, c.ShouldBeNil)
	})
}

func TestTruncateInLocation(t *testing.T) {
	mockey.PatchConvey("TestTruncateInLocation", t, func() {
		mockey.PatchConvey("按天轮转对齐本地零点", func() {
			// 直接使用 time.Truncate 会以 UTC 零点为基准，落在本地时间的非零点时刻
			loc := time.FixedZone("UTC+8", 8*3600)
			ts := time.Date(2026, 9, 12, 3, 18, 0, 0, loc)
			got := truncateInLocation(ts, 24*time.Hour)
			c.So(got.Year(), c.ShouldEqual, 2026)
			c.So(got.Month(), c.ShouldEqual, time.September)
			c.So(got.Day(), c.ShouldEqual, 12)
			c.So(got.Hour(), c.ShouldEqual, 0)
			c.So(got.Location(), c.ShouldEqual, loc)
		})

		mockey.PatchConvey("UTC 时区走原生截断", func() {
			ts := time.Date(2026, 9, 12, 3, 18, 0, 0, time.UTC)
			got := truncateInLocation(ts, time.Hour)
			c.So(got.Hour(), c.ShouldEqual, 3)
			c.So(got.Minute(), c.ShouldEqual, 0)
		})

		mockey.PatchConvey("周期非正时原样返回", func() {
			ts := time.Date(2026, 9, 12, 3, 18, 0, 0, time.UTC)
			c.So(truncateInLocation(ts, 0), c.ShouldEqual, ts)
		})
	})
}

func TestRotateLayoutFor(t *testing.T) {
	mockey.PatchConvey("TestRotateLayoutFor", t, func() {
		c.So(rotateLayoutFor(24*time.Hour), c.ShouldEqual, rotateLayoutDay)
		c.So(rotateLayoutFor(7*24*time.Hour), c.ShouldEqual, rotateLayoutDay)
		c.So(rotateLayoutFor(time.Hour), c.ShouldEqual, rotateLayoutHour)
		c.So(rotateLayoutFor(6*time.Hour), c.ShouldEqual, rotateLayoutHour)
		c.So(rotateLayoutFor(30*time.Minute), c.ShouldEqual, rotateLayoutMinute)
	})
}

func TestRotateWriterErrorPaths(t *testing.T) {
	mockey.PatchConvey("TestRotateWriterErrorPaths", t, func() {
		mockey.PatchConvey("轮转失败时降级继续写旧文件", func() {
			// 直接返回错误的话，从第一次轮转失败起每条日志都走同一条失败路径，
			// 而 asyncWriter 只保留第一个错误且只在 Close 时返回——
			// 现象就是某天日志突然断了，没有任何报错
			now := time.Date(2026, 9, 12, 10, 0, 0, 0, time.Local)
			w, _ := newTestWriter(t, time.Hour, 24*time.Hour, &now)

			if _, err := w.Write([]byte("before\n")); err != nil {
				t.Fatalf("轮转前写入应成功: %v", err)
			}
			current := w.currentName

			// os.ReadFile 内部也走 os.OpenFile，写完后要立刻解除 mock 才能读回文件
			openMock := mockey.Mock(os.OpenFile).Return(nil, errors.New("permission denied")).Build()
			now = now.Add(48 * time.Hour) // 触发轮转

			n, err := w.Write([]byte("after\n"))
			openMock.UnPatch()

			c.So(err, c.ShouldBeNil)
			c.So(n, c.ShouldEqual, len("after\n"))
			// 仍然写在原来的文件上，没有切到新文件
			c.So(w.currentName, c.ShouldEqual, current)

			content, readErr := os.ReadFile(current)
			c.So(readErr, c.ShouldBeNil)
			c.So(string(content), c.ShouldContainSubstring, "after")
		})

		mockey.PatchConvey("轮转失败告警按间隔降噪", func() {
			now := time.Date(2026, 9, 12, 10, 0, 0, 0, time.Local)
			w, _ := newTestWriter(t, time.Hour, 24*time.Hour, &now)

			warned := 0
			mockey.Mock(xutil.WarnIfEnableDebug).To(func(_ string, _ ...any) { warned++ }).Build()
			mockey.Mock(os.OpenFile).Return(nil, errors.New("permission denied")).Build()
			now = now.Add(48 * time.Hour)

			for i := 0; i < 5; i++ {
				_, _ = w.Write([]byte("x"))
			}
			// 轮转周期到了之后每条日志都会重试，不限流会把告警刷爆
			c.So(warned, c.ShouldEqual, 1)

			now = now.Add(2 * rotateErrLogInterval)
			_, _ = w.Write([]byte("x"))
			c.So(warned, c.ShouldEqual, 2)
		})

		mockey.PatchConvey("旧文件关闭失败不影响继续写入", func() {
			now := time.Date(2026, 9, 12, 10, 0, 0, 0, time.Local)
			w, dir := newTestWriter(t, time.Hour, 24*time.Hour, &now)

			mockey.Mock((*os.File).Close).Return(errors.New("close failed")).Build()
			now = now.Add(24 * time.Hour)

			n, err := w.Write([]byte("next day\n"))
			c.So(err, c.ShouldBeNil)
			c.So(n, c.ShouldEqual, 9)
			_, statErr := os.Stat(filepath.Join(dir, "app.log.20260913"))
			c.So(statErr, c.ShouldBeNil)
		})

		mockey.PatchConvey("零值写入器关闭安全", func() {
			// file 为 nil 的写入器不应 panic
			c.So((&rotateWriter{}).Close(), c.ShouldBeNil)
		})

		mockey.PatchConvey("未配置符号链接时跳过创建", func() {
			dir := t.TempDir()
			w := &rotateWriter{
				base:   filepath.Join(dir, "app.log"),
				layout: rotateLayoutDay,
				rotate: 24 * time.Hour,
				clock:  time.Now,
			}
			c.So(w.rotateTo(w.filenameFor(time.Now())), c.ShouldBeNil)
			defer func() { _ = w.Close() }()

			// linkName 为空，不应生成任何符号链接
			entries, err := os.ReadDir(dir)
			c.So(err, c.ShouldBeNil)
			for _, e := range entries {
				c.So(e.Name(), c.ShouldNotEqual, "app.log")
			}
		})

		mockey.PatchConvey("创建符号链接失败不影响日志写入", func() {
			mockey.Mock(os.Symlink).Return(errors.New("symlink unsupported")).Build()
			now := time.Date(2026, 9, 12, 10, 0, 0, 0, time.Local)
			w, dir := newTestWriter(t, time.Hour, 24*time.Hour, &now)

			_, err := w.Write([]byte("hello\n"))
			c.So(err, c.ShouldBeNil)
			content, readErr := os.ReadFile(filepath.Join(dir, "app.log.20260912"))
			c.So(readErr, c.ShouldBeNil)
			c.So(string(content), c.ShouldEqual, "hello\n")
		})

		mockey.PatchConvey("替换符号链接失败不影响日志写入", func() {
			mockey.Mock(os.Rename).Return(errors.New("rename failed")).Build()
			now := time.Date(2026, 9, 12, 10, 0, 0, 0, time.Local)
			w, dir := newTestWriter(t, time.Hour, 24*time.Hour, &now)

			_, err := w.Write([]byte("hello\n"))
			c.So(err, c.ShouldBeNil)
			content, readErr := os.ReadFile(filepath.Join(dir, "app.log.20260912"))
			c.So(readErr, c.ShouldBeNil)
			c.So(string(content), c.ShouldEqual, "hello\n")
		})
	})
}

func TestRotateWriterPurgeErrorPaths(t *testing.T) {
	mockey.PatchConvey("TestRotateWriterPurgeErrorPaths", t, func() {
		now := time.Date(2026, 9, 12, 10, 0, 0, 0, time.Local)

		mockey.PatchConvey("匹配文件列表失败时直接返回", func() {
			w, _ := newTestWriter(t, 48*time.Hour, 24*time.Hour, &now)
			mockey.Mock(filepath.Glob).Return(nil, errors.New("glob failed")).Build()
			w.purge(w.currentName) // 不应 panic
		})

		mockey.PatchConvey("跳过无法 stat 的文件", func() {
			w, dir := newTestWriter(t, 48*time.Hour, 24*time.Hour, &now)
			stale := filepath.Join(dir, "app.log.20260901")
			c.So(os.WriteFile(stale, []byte("x"), logFilePerm), c.ShouldBeNil)

			mockey.Mock(os.Lstat).Return(nil, errors.New("stat failed")).Build()
			w.purge(w.currentName)

			// stat 失败的文件应被跳过而非误删
			_, err := os.Stat(stale)
			c.So(err, c.ShouldBeNil)
		})

		mockey.PatchConvey("跳过符号链接", func() {
			w, dir := newTestWriter(t, 48*time.Hour, 24*time.Hour, &now)
			link := filepath.Join(dir, "app.log.link")
			c.So(os.Symlink(w.currentName, link), c.ShouldBeNil)
			old := now.Add(-10 * 24 * time.Hour)
			_ = os.Chtimes(link, old, old)

			w.purge(w.currentName)

			_, err := os.Lstat(link)
			c.So(err, c.ShouldBeNil)
		})

		mockey.PatchConvey("删除失败仅记录不中断", func() {
			w, dir := newTestWriter(t, 48*time.Hour, 24*time.Hour, &now)
			stale := filepath.Join(dir, "app.log.20260901")
			c.So(os.WriteFile(stale, []byte("x"), logFilePerm), c.ShouldBeNil)
			old := now.Add(-10 * 24 * time.Hour)
			c.So(os.Chtimes(stale, old, old), c.ShouldBeNil)

			mockey.Mock(os.Remove).Return(errors.New("remove failed")).Build()
			w.purge(w.currentName) // 不应 panic
		})
	})
}
