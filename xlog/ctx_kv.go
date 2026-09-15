package xlog

import (
	"context"
	"sync"

	"github.com/xiaoshicae/xone/v2/xutil"
)

// ctxKVScopeKey KV 作用域在 context 中的 key
//
// 使用私有类型而非字符串常量，避免与其他包的 context key 发生冲突。
type ctxKVScopeKey struct{}

var kvScopeKey = ctxKVScopeKey{}

// kvScope 一次请求（或一段调用链）共享的 KV 集合
//
// 存进 context 的是指针，因此 AddKV 的写入对所有持有该 context 的地方立即可见，
// 不需要把新 context 回传给调用方——业务函数在调用栈深处拿不到 *gin.Context，
// 本来也没机会回传。
//
// 日志写入发生在任意 goroutine（handler 起协程打日志是常态），
// 因此读写都要加锁。
type kvScope struct {
	mu sync.RWMutex
	kv map[string]any
}

// newKVScope 创建作用域，kvs 为空时不建 map
//
// 延迟到首次写入才建：请求入口无条件装一个作用域，而多数请求一个字段都不写，
// 那个空 map 就是每请求一次的纯浪费。读取侧对 nil map 天然安全
// （range nil 不循环、len(nil) 为 0），不需要额外判空。
func newKVScope(kvs map[string]any) *kvScope {
	if len(kvs) == 0 {
		return &kvScope{}
	}
	s := &kvScope{kv: make(map[string]any, len(kvs))}
	for k, v := range kvs {
		s.kv[k] = v
	}
	return s
}

// kvScopeHint 首次写入时 map 的初始容量
const kvScopeHint = 8

func (s *kvScope) add(k string, v any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.kv == nil {
		s.kv = make(map[string]any, kvScopeHint)
	}
	s.kv[k] = v
}

func (s *kvScope) addAll(kvs map[string]any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.kv == nil {
		s.kv = make(map[string]any, max(len(kvs), kvScopeHint))
	}
	for k, v := range kvs {
		s.kv[k] = v
	}
}

// rangeKV 在读锁内遍历，避免为日志的每一行复制一次 map
func (s *kvScope) rangeKV(f func(k string, v any)) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for k, v := range s.kv {
		f(k, v)
	}
}

func (s *kvScope) snapshot() map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.kv == nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(s.kv))
	for k, v := range s.kv {
		out[k] = v
	}
	return out
}

// scopeFromCtx 取出 context 中的 KV 作用域，没有则返回 nil
func scopeFromCtx(ctx context.Context) *kvScope {
	if ctx == nil {
		return nil
	}
	s, _ := ctx.Value(kvScopeKey).(*kvScope)
	return s
}

// CtxWithKVScope 为 context 开启一个 KV 作用域，之后可用 AddKV 持续写入
//
// 典型用法是在请求入口装一次（xgin 的 LogScope 中间件已经这么做），
// 后续任意调用层级用 AddKV 补充字段，全部会出现在这次请求的每一行日志里：
//
//	ctx = xlog.CtxWithKVScope(ctx)      // 入口装一次
//	xlog.AddKV(ctx, "userID", userID)   // 任意深度写入，无需回传 ctx
//	xlog.Info(ctx, "order created")     // 自动带上 userID
//
// 幂等：已有作用域时原样返回，不会覆盖已写入的内容，重复装不会丢字段。
//
// 与 CtxWithKV 的区别是写入方向：本函数装的作用域是共享可变的，
// 谁写都对所有人可见；CtxWithKV 则是派生一个快照，只影响它返回的那个 context。
func CtxWithKVScope(ctx context.Context) context.Context {
	if scopeFromCtx(ctx) != nil {
		return ctx
	}
	return context.WithValue(ctx, kvScopeKey, newKVScope(nil))
}

// AddKV 向当前 KV 作用域写入一个字段，之后该 context 的每一行日志都会带上它
//
// 原地生效，不返回新 context——这正是它与 CtxWithKV 的分工：
// 业务函数在调用栈深处只拿得到 ctx，没有办法把新 context 交还给上层中间件。
//
// 同名 key 覆盖旧值。
//
// context 中没有作用域时本次写入会被丢弃：作用域必须由持有 context 的一方
// （通常是请求入口中间件）先用 CtxWithKVScope 装上，这里无法凭空创建一个
// 让调用方看得见的。丢弃时会打一条 debug 日志，否则"日志里就是没有这个字段"
// 将没有任何线索。
func AddKV(ctx context.Context, k string, v any) {
	s := scopeFromCtx(ctx)
	if s == nil {
		xutil.WarnIfEnableDebug("XOne xlog AddKV called without a KV scope, dropped, key=[%s]; call xlog.CtxWithKVScope(ctx) at the entry point first", k)
		return
	}
	s.add(k, v)
}

// AddKVs 向当前 KV 作用域批量写入，语义同 AddKV
func AddKVs(ctx context.Context, kvs map[string]any) {
	if len(kvs) == 0 {
		return
	}

	s := scopeFromCtx(ctx)
	if s == nil {
		xutil.WarnIfEnableDebug("XOne xlog AddKVs called without a KV scope, dropped, keys=[%d]; call xlog.CtxWithKVScope(ctx) at the entry point first", len(kvs))
		return
	}
	s.addAll(kvs)
}

// CtxWithKV 派生一个带有额外 KV 的新 context，在记录日志时以 json 格式一并输出
//
// 派生而非就地写入：返回的 context 拥有独立的 KV 副本，写入不会影响传入的 ctx，
// 因此适合"只给这一段调用链打个标"的场景。需要让写入对整条请求可见，
// 用 CtxWithKVScope + AddKV。
//
// 同名 key 以传入的 kvs 为准。
func CtxWithKV(ctx context.Context, kvs map[string]any) context.Context {
	merged := kvs
	if s := scopeFromCtx(ctx); s != nil {
		merged = s.snapshot()
		for k, v := range kvs {
			merged[k] = v
		}
	}
	return context.WithValue(ctx, kvScopeKey, newKVScope(merged))
}

// KVFromCtx 返回 context 中已写入的 KV 快照
//
// 未开启过作用域时返回 nil；返回的是副本，改动它不会影响后续日志。
func KVFromCtx(ctx context.Context) map[string]any {
	s := scopeFromCtx(ctx)
	if s == nil {
		return nil
	}
	return s.snapshot()
}

// rangeCtxKV 遍历 context 中的 KV，供日志 handler 在热路径上使用
//
// 不走 KVFromCtx：那会为每一行日志复制一次 map。
func rangeCtxKV(ctx context.Context, f func(k string, v any)) {
	s := scopeFromCtx(ctx)
	if s == nil {
		return
	}
	s.rangeKV(f)
}
