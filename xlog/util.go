package xlog

import (
	"context"

	"github.com/sirupsen/logrus"
)

// logf 通过函数变量间接调用底层格式化输出
//
// 不直接写 entry.Logf(lv, msg, args...) 是有意为之：那样会被 go vet 推断为
// printf 包装函数，而本模块的 args 允许混入 Option（非格式化参数），
// 推断成立后会在所有使用方项目产生大量误报，例如
// xlog.Info(ctx, "order created", xlog.KV("orderId", "1"))
var logf = (*logrus.Entry).Logf

// ctxKVContainerKey xlog 注入 context 的 KV 容器 key 类型
// 使用私有类型而非字符串常量，避免与其他包的 context key 发生冲突
type ctxKVContainerKey struct{}

// xLogCtxKVContainerKey KV 容器在 context 中的 key
var xLogCtxKVContainerKey = ctxKVContainerKey{}

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
		return
	}

	lv := level.toLogrus()
	// 级别未开启时尽早返回，省去 Entry 分配与参数处理
	if !logger.IsLevelEnabled(lv) {
		return
	}

	// Fast path：无参数调用，跳过参数分离
	if len(args) == 0 {
		logger.WithContext(ctx).Log(lv, msg)
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
		logf(logger.WithContext(ctx), lv, msg, args...)
		return
	}

	// options.KV 与 logrus.Fields 底层同为 map[string]any，
	// 直接复用同一个 map，避免先收集再拷贝一次
	dos := &options{KV: make(map[string]any, optCount)}
	logArgs := make([]any, 0, len(args)-optCount)
	for _, arg := range args {
		if opt, ok := arg.(Option); ok {
			opt(dos)
			continue
		}
		logArgs = append(logArgs, arg)
	}

	logf(logger.WithContext(ctx).WithFields(logrus.Fields(dos.KV)), lv, msg, logArgs...)
}

// CtxWithKV 向ctx注入kv，在记录日志时会以json格式同时记录下来
// 每次调用都会创建新的map副本，保证并发安全
func CtxWithKV(ctx context.Context, kvs map[string]any) context.Context {
	kvContainer, ok := ctx.Value(xLogCtxKVContainerKey).(map[string]any)
	if !ok || kvContainer == nil {
		// 创建副本避免外部修改影响
		newKvs := make(map[string]any, len(kvs))
		for k, v := range kvs {
			newKvs[k] = v
		}
		return context.WithValue(ctx, xLogCtxKVContainerKey, newKvs)
	}

	// 合并已有的和新的kv，创建新map保证并发安全
	newContainer := make(map[string]any, len(kvContainer)+len(kvs))
	for k, v := range kvContainer {
		newContainer[k] = v
	}
	for k, v := range kvs {
		newContainer[k] = v
	}
	return context.WithValue(ctx, xLogCtxKVContainerKey, newContainer)
}

// KVFromCtx 返回 context 中已注入的 KV 副本
// 未注入过任何 KV 容器时返回 nil；返回副本避免调用方修改影响后续日志
func KVFromCtx(ctx context.Context) map[string]any {
	kvContainer := getXLogContainerFromCtx(ctx)
	if kvContainer == nil {
		return nil
	}
	out := make(map[string]any, len(kvContainer))
	for k, v := range kvContainer {
		out[k] = v
	}
	return out
}

// CurrentLevel 返回当前生效的日志级别
func CurrentLevel() Level {
	return Level(currentLevel.Load())
}

// XLogLevel 返回当前生效的日志级别名称，如 "info"
func XLogLevel() string {
	return CurrentLevel().String()
}
