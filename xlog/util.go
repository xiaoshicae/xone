package xlog

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// sprintf 通过函数变量间接调用格式化
//
// 不直接写 fmt.Sprintf(msg, args...) 是有意为之：那样会被 go vet 推断为
// printf 包装函数，而本模块的 args 允许混入 Option（非格式化参数），
// 推断成立后会在所有使用方项目产生大量误报，例如
// xlog.Info(ctx, "order created", xlog.KV("orderId", "1"))
var sprintf = fmt.Sprintf

func Error(ctx context.Context, msg string, args ...any) {
	RawLog(ctx, ErrorLevel, msg, args...)
}

func Warn(ctx context.Context, msg string, args ...any) {
	RawLog(ctx, WarnLevel, msg, args...)
}

func Info(ctx context.Context, msg string, args ...any) {
	RawLog(ctx, InfoLevel, msg, args...)
}

func Debug(ctx context.Context, msg string, args ...any) {
	RawLog(ctx, DebugLevel, msg, args...)
}

// RawLog 按指定级别输出日志
// args 中的 Option 会被提取为日志的 KV 字段，其余参数用于 msg 的格式化占位符
func RawLog(ctx context.Context, level Level, msg string, args ...any) {
	if ctx == nil {
		// 传 nil ctx 是调用方的疏忽，但为此丢掉一整条（可能是 Error 级的）日志
		// 代价太大，用 Background 兜底，只是取不到 trace 与 ctx 中的 KV
		ctx = context.Background()
	}

	h := handler.Load()
	lv := level.toSlog()
	// 级别未开启时尽早返回，省去参数处理与 Record 构造
	if h == nil || !h.Enabled(ctx, lv) {
		return
	}

	// Fast path：无参数调用，跳过参数分离与格式化
	if len(args) == 0 {
		_ = h.Handle(ctx, slog.NewRecord(time.Now(), lv, msg, 0))
		return
	}

	// 先统计 Option 数量，无 Option 时可完全避免额外分配
	optCount := 0
	for _, arg := range args {
		if _, ok := arg.(Option); ok {
			optCount++
		}
	}
	if optCount == 0 {
		_ = h.Handle(ctx, slog.NewRecord(time.Now(), lv, sprintf(msg, args...), 0))
		return
	}

	dos := &options{KV: make(map[string]any, optCount)}
	logArgs := make([]any, 0, len(args)-optCount)
	for _, arg := range args {
		if opt, ok := arg.(Option); ok {
			opt(dos)
			continue
		}
		logArgs = append(logArgs, arg)
	}

	r := slog.NewRecord(time.Now(), lv, sprintf(msg, logArgs...), 0)
	for k, v := range dos.KV {
		r.AddAttrs(slog.Any(k, v))
	}
	_ = h.Handle(ctx, r)
}

// Handler 返回当前生效的 slog.Handler
// 可交给任何接受 slog.Handler 的第三方库，使其日志与本模块共用同一套输出配置
func Handler() slog.Handler {
	return handler.Load()
}

// Logger 返回基于当前配置的 slog.Logger
// 可交给任何接受 *slog.Logger 的第三方库
func Logger() *slog.Logger {
	return slog.New(handler.Load())
}

// CurrentLevel 返回当前生效的日志级别
//
// 需要级别名称（如 "info"）时用 CurrentLevel().String()。
func CurrentLevel() Level {
	return Level(currentLevel.Load())
}
