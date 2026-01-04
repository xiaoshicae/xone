package xlog

import (
	"context"

	"github.com/xiaoshicae/xone/xconfig"
	"github.com/xiaoshicae/xone/xutil"

	"github.com/sirupsen/logrus"
)

const xLogCtxKVContainerKey = "__xlog__ctx__kv__container__"

func Error(ctx context.Context, msg string, args ...interface{}) {
	RawLog(ctx, logrus.ErrorLevel, msg, args...)
}

func Warn(ctx context.Context, msg string, args ...interface{}) {
	RawLog(ctx, logrus.WarnLevel, msg, args...)
}

func Info(ctx context.Context, msg string, args ...interface{}) {
	RawLog(ctx, logrus.InfoLevel, msg, args...)
}

func Debug(ctx context.Context, msg string, args ...interface{}) {
	RawLog(ctx, logrus.DebugLevel, msg, args...)
}

func RawLog(ctx context.Context, level logrus.Level, msg string, args ...interface{}) {
	logArgs := make([]interface{}, 0)

	opts := make([]Option, 0)

	for _, arg := range args {
		if _, ok := arg.(Option); ok {
			opts = append(opts, arg.(Option))
		} else {
			logArgs = append(logArgs, arg)
		}
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
func CtxWithKV(ctx context.Context, kvs map[string]interface{}) context.Context {
	if kvs == nil {
		kvs = make(map[string]interface{})
	}
	kvContainer, ok := ctx.Value(xLogCtxKVContainerKey).(map[string]interface{})
	if !ok || kvContainer == nil {
		// 创建副本避免外部修改影响
		newKvs := make(map[string]interface{}, len(kvs))
		for k, v := range kvs {
			newKvs[k] = v
		}
		return context.WithValue(ctx, xLogCtxKVContainerKey, newKvs)
	}
	// 合并已有的和新的kv，创建新map保证并发安全
	newContainer := make(map[string]interface{}, len(kvContainer)+len(kvs))
	for k, v := range kvContainer {
		newContainer[k] = v
	}
	for k, v := range kvs {
		newContainer[k] = v
	}
	return context.WithValue(ctx, xLogCtxKVContainerKey, newContainer)
}

func XLogLevel() string {
	return xutil.GetOrDefault(xconfig.GetString(XLogConfigKey+".Level"), "Info")
}
