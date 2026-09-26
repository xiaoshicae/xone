package xlog

import (
	"context"
	"sync"
	"sync/atomic"
)

// ctxScopeKey KV 作用域在 context 中的 key。
// 用私有类型而不是字符串，避免与其它包的 context key 撞上。
type ctxScopeKey struct{}

// scope 一次请求（或一段调用链）共享的字段集合
//
// 存进 context 的是**指针**，所以 AddKV 的写入对所有持有该 ctx 的地方立即可见，
// 不需要把新 context 回传给调用方——业务函数在调用栈深处拿不到 *gin.Context，
// 本来也没机会回传。这正是「返回新 context」那种写法解决不了的场景。
//
// 日志可能在任意 goroutine 写出，读写都要加锁。
type scope struct {
	mu sync.RWMutex
	kv map[string]any
}

// scopeInitCap 首次写入时 map 的初始容量
const scopeInitCap = 8

func (s *scope) add(k string, v any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.kv == nil {
		s.kv = make(map[string]any, scopeInitCap)
	}
	s.kv[k] = v
}

func (s *scope) addAll(kvs map[string]any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.kv == nil {
		s.kv = make(map[string]any, max(len(kvs), scopeInitCap))
	}
	for k, v := range kvs {
		s.kv[k] = v
	}
}

// each 在读锁内遍历，避免为每一行日志复制一次 map
func (s *scope) each(f func(k string, v any)) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for k, v := range s.kv {
		f(k, v)
	}
}

// copyWith 复制一份字段，再叠上 kvs（同名以 kvs 为准）
func (s *scope) copyWith(kvs map[string]any) *scope {
	s.mu.RLock()
	defer s.mu.RUnlock()
	c := &scope{kv: make(map[string]any, max(len(s.kv)+len(kvs), scopeInitCap))}
	for k, v := range s.kv {
		c.kv[k] = v
	}
	for k, v := range kvs {
		c.kv[k] = v
	}
	return c
}

func (s *scope) len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.kv)
}

// CtxWithScope 开启一个字段作用域。
//
// 之后在这条 context 上任何位置调用 AddKV 写入的字段，都会出现在该 context
// 记录的每一条日志里——包括请求入口在 handler 返回之后写的那条访问日志。
//
// 幂等：已经有作用域时原样返回，重复调用（比如中间件被注册了两次）不会
// 清空已经写入的字段。
func CtxWithScope(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if scopeFrom(ctx) != nil {
		return ctx
	}
	return context.WithValue(ctx, ctxScopeKey{}, &scope{})
}

// CtxWithKV 派生一个带着 kvs 的新 context：父 context 已有的字段照样带着，
// 再叠上 kvs（同名以 kvs 为准）。
//
// 和 AddKV 的分工在影响范围：AddKV 原地写，同一个请求里后面的每一行日志都带上；
// CtxWithKV 只影响它返回的那个 context，适合给一段调用打标——批量处理里每一条
// 带上自己的 order_id、起一个 goroutine 带上 worker 名字——兄弟之间互不串。
//
//	for _, o := range orders {
//		ctx := xlog.CtxWithKV(ctx, map[string]any{"order_id": o.ID})
//		process(ctx, o) // 这里面的日志都带着 order_id，下一轮不带上一轮的
//	}
//
// 派生出来的 context 自带一个作用域：在它上面 AddKV 只写进它自己，
// 不回流到父 context——入口处的访问日志因此看不到这些字段。
// 派生之后父 context 再 AddKV 的字段，它也看不到：它拿的是派生那一刻的副本。
func CtxWithKV(ctx context.Context, kvs map[string]any) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	parent := scopeFrom(ctx)
	if parent == nil {
		parent = &scope{}
	}
	return context.WithValue(ctx, ctxScopeKey{}, parent.copyWith(kvs))
}

// AddKV 往当前作用域写一个字段。
//
// 没有作用域时丢弃并计数——否则「字段就是没出现在日志里」不留任何痕迹。
func AddKV(ctx context.Context, k string, v any) {
	s := scopeFrom(ctx)
	if s == nil {
		droppedKV.Add(1)
		return
	}
	s.add(k, v)
}

// AddKVs 往当前作用域批量写字段
func AddKVs(ctx context.Context, kvs map[string]any) {
	if len(kvs) == 0 {
		return
	}
	s := scopeFrom(ctx)
	if s == nil {
		droppedKV.Add(int64(len(kvs)))
		return
	}
	s.addAll(kvs)
}

// droppedKV 在没有作用域的 context 上被丢弃的字段数。
// 供排查「我明明 AddKV 了但日志里没有」——多半是漏了 CtxWithScope。
var droppedKV atomic.Int64

// DroppedKVCount 返回被丢弃的字段数，不为零说明有 AddKV 调用没有对应的作用域
func DroppedKVCount() int64 { return droppedKV.Load() }

func scopeFrom(ctx context.Context) *scope {
	if ctx == nil {
		return nil
	}
	s, _ := ctx.Value(ctxScopeKey{}).(*scope)
	return s
}
