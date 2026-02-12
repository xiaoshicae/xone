package xgorm

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/xiaoshicae/xone/v2/xlog"
	"github.com/xiaoshicae/xone/v2/xutil"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// logLevelMapping 日志级别映射
var logLevelMapping = map[string]logger.LogLevel{
	"info":    logger.Info,
	"warn":    logger.Warn,
	"warning": logger.Warn,
	"error":   logger.Error,
}

func newGormLogger(c *Config) *gormLogger {
	return &gormLogger{
		logLevel:                  resolveLoglevel(xlog.XLogLevel()),
		slowThreshold:             xutil.ToDuration(c.SlowThreshold),
		ignoreRecordNotFoundError: c.IgnoreRecordNotFoundErrorLog,
	}
}

type gormLogger struct {
	logLevel                  logger.LogLevel
	ignoreRecordNotFoundError bool
	slowThreshold             time.Duration
}

func (l *gormLogger) LogMode(level logger.LogLevel) logger.Interface {
	// 创建副本，避免修改共享实例导致并发安全问题（GORM 约定返回新实例）
	newLogger := *l
	newLogger.logLevel = level
	return &newLogger
}

func (l *gormLogger) Info(ctx context.Context, s string, i ...interface{}) {
	xlog.Info(ctx, s, i...)
}

func (l *gormLogger) Warn(ctx context.Context, s string, i ...interface{}) {
	xlog.Warn(ctx, s, i...)
}

func (l *gormLogger) Error(ctx context.Context, s string, i ...interface{}) {
	xlog.Error(ctx, s, i...)
}

func (l *gormLogger) Trace(ctx context.Context, begin time.Time, fc func() (sql string, rowsAffected int64), err error) {
	cost := time.Since(begin)

	switch {
	case err != nil && l.logLevel >= logger.Error && (!errors.Is(err, gorm.ErrRecordNotFound) || !l.ignoreRecordNotFoundError):
		sql, rows := fc()
		costMS := strconv.FormatInt(cost.Milliseconds(), 10) + "ms"
		xlog.Error(ctx, "latency: %s, rowsAffected: %v, sql: %s, err: %v", costMS, formatRows(rows), sql, err)
	case cost > l.slowThreshold && l.slowThreshold != 0 && l.logLevel >= logger.Warn:
		sql, rows := fc()
		costMS := strconv.FormatInt(cost.Milliseconds(), 10) + "ms"
		xlog.Warn(ctx, "SLOW SQL >= %v, latency: %s, rowsAffected: %v, sql: %s", l.slowThreshold, costMS, formatRows(rows), sql)
	case l.logLevel == logger.Info:
		sql, rows := fc()
		costMS := strconv.FormatInt(cost.Milliseconds(), 10) + "ms"
		xlog.Info(ctx, "latency: %s, rowsAffected: %v, sql: %s", costMS, formatRows(rows), sql)
	}
}

// formatRows 格式化行数显示，-1 表示未知
func formatRows(rows int64) any {
	if rows == -1 {
		return "-"
	}
	return rows
}

func resolveLoglevel(l string) logger.LogLevel {
	if level, ok := logLevelMapping[strings.ToLower(l)]; ok {
		return level
	}
	return logger.Info
}
