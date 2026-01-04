package xgorm

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/xiaoshicae/xone/xlog"

	"github.com/spf13/cast"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func newGormLogger(c *Config) *gormLogger {
	return &gormLogger{
		logLevel:                  resolveLoglevel(xlog.XLogLevel()),
		slowThreshold:             cast.ToDuration(c.SlowThreshold),
		ignoreRecordNotFoundError: c.IgnoreRecordNotFoundErrorLog,
	}
}

type gormLogger struct {
	logLevel                  logger.LogLevel
	ignoreRecordNotFoundError bool
	slowThreshold             time.Duration
}

func (l *gormLogger) LogMode(level logger.LogLevel) logger.Interface {
	l.logLevel = level
	return l
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
	costMS := fmt.Sprintf("%vms", cost.Milliseconds())

	switch {
	case err != nil && l.logLevel >= logger.Error && (!errors.Is(err, gorm.ErrRecordNotFound) || !l.ignoreRecordNotFoundError):
		sql, rows := fc()
		if rows == -1 {
			xlog.Error(ctx, "latency: %s, rowsAffected: %v, sql: %s, err: %v", costMS, "-", sql, err)
		} else {
			xlog.Error(ctx, "latency: %s, rowsAffected: %v, sql: %s, err: %v", costMS, rows, sql, err)
		}
	case cost > l.slowThreshold && l.slowThreshold != 0 && l.logLevel >= logger.Warn:
		sql, rows := fc()
		slowLog := fmt.Sprintf("SLOW SQL >= %v", l.slowThreshold)
		if rows == -1 {
			xlog.Warn(ctx, "%s, latency: %s, rowsAffected: %v, sql: %s", slowLog, costMS, "-", sql)
		} else {
			xlog.Warn(ctx, "%s, latency: %s, rowsAffected: %v, sql: %s", slowLog, costMS, rows, sql)
		}
	case l.logLevel == logger.Info:
		sql, rows := fc()
		if rows == -1 {
			xlog.Info(ctx, "latency: %s, rowsAffected: %v, sql: %s", costMS, "-", sql)
		} else {
			xlog.Info(ctx, "latency: %s, rowsAffected: %v, sql: %s", costMS, rows, sql)
		}
	}
}

func resolveLoglevel(l string) logger.LogLevel {
	l = strings.ToLower(l)
	switch l {
	case "info":
		return logger.Info
	case "warn", "warning":
		return logger.Warn
	case "error":
		return logger.Error
	default:
		return logger.Info
	}
}
