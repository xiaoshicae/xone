package xlog

import (
	"context"

	"github.com/xiaoshicae/xone/v2/xconfig"
	"github.com/xiaoshicae/xone/v2/xutil"

	"github.com/sirupsen/logrus"
)

const XLogCtxKVContainerKey = "__xlog__ctx__kv__container__"

func Error(ctx context.Context, msg string, args ...any) {
	RawLog(ctx, logrus.ErrorLevel, msg, args...)
}

func Warn(ctx context.Context, msg string, args ...any) {
	RawLog(ctx, logrus.WarnLevel, msg, args...)
}

func Info(ctx context.Context, msg string, args ...any) {
	RawLog(ctx, logrus.InfoLevel, msg, args...)
}

func Debug(ctx context.Context, msg string, args ...any) {
	RawLog(ctx, logrus.DebugLevel, msg, args...)
}

func RawLog(ctx context.Context, level logrus.Level, msg string, args ...any) {
	if ctx == nil {
		return
	}

	// Fast path：无参数调用，跳过 slice 分配和 options 处理
	if len(args) == 0 {
		logrus.WithContext(ctx).Log(level, msg)
		return
	}

	// 分离 logArgs 和 opts
	logArgs := make([]any, 0, len(args))
	opts := make([]Option, 0, 2)

	for _, arg := range args {
		if opt, ok := arg.(Option); ok {
			opts = append(opts, opt)
		} else {
			logArgs = append(logArgs, arg)
		}
	}

	if len(opts) == 0 {
		// 无 Option，直接用 logArgs 格式化输出
		logrus.WithContext(ctx).Logf(level, msg, logArgs...)
		return
	}

	dos := defaultOptions()
	for _, o := range opts {
		o(dos)
	}

	fields := logrus.Fields{}
	for k, v := range dos.KV {
		fields[k] = v
	}

	logrus.WithContext(ctx).WithFields(fields).Logf(level, msg, logArgs...)
}

// CtxWithKV 向ctx注入kv，在记录日志时会以json格式同时记录下来
// 每次调用都会创建新的map副本，保证并发安全
func CtxWithKV(ctx context.Context, kvs map[string]any) context.Context {
	if kvs == nil {
		kvs = make(map[string]any)
	}
	kvContainer, ok := ctx.Value(XLogCtxKVContainerKey).(map[string]any)
	if !ok || kvContainer == nil {
		// 创建副本避免外部修改影响
		newKvs := make(map[string]any, len(kvs))
		for k, v := range kvs {
			newKvs[k] = v
		}
		return context.WithValue(ctx, XLogCtxKVContainerKey, newKvs)
	}
	// 合并已有的和新的kv，创建新map保证并发安全
	newContainer := make(map[string]any, len(kvContainer)+len(kvs))
	for k, v := range kvContainer {
		newContainer[k] = v
	}
	for k, v := range kvs {
		newContainer[k] = v
	}
	return context.WithValue(ctx, XLogCtxKVContainerKey, newContainer)
}

func XLogLevel() string {
	return xutil.GetOrDefault(xconfig.GetString(XLogConfigKey+".Level"), "Info")
}
