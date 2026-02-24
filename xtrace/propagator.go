package xtrace

import (
	"context"
	"maps"
	"net/http"

	"go.opentelemetry.io/otel/propagation"
)

// forwardHeadersContextKey 用于在 context 中存储透传 Header 的 key
type forwardHeadersContextKey struct{}

// HeaderPropagator 实现 propagation.TextMapPropagator 接口，
// 用于在链路中透传指定的自定义 HTTP Header（如 X-Request-ID、X-Tenant-ID）
type HeaderPropagator struct {
	headers []string
}

// NewHeaderPropagator 创建 HeaderPropagator，headers 会被 http.CanonicalHeaderKey 规范化
func NewHeaderPropagator(headers []string) *HeaderPropagator {
	normalized := make([]string, 0, len(headers))
	for _, h := range headers {
		if h != "" {
			normalized = append(normalized, http.CanonicalHeaderKey(h))
		}
	}
	return &HeaderPropagator{headers: normalized}
}

// Extract 从 carrier 中读取配置的 Header 并存入 context
func (p *HeaderPropagator) Extract(ctx context.Context, carrier propagation.TextMapCarrier) context.Context {
	if len(p.headers) == 0 {
		return ctx
	}

	// 获取已有的透传值，合并新值
	existing := forwardHeadersFromContextRaw(ctx)
	var merged map[string]string

	for _, h := range p.headers {
		v := carrier.Get(h)
		if v == "" {
			continue
		}
		if merged == nil {
			// 延迟初始化，只在有值时才创建 map
			merged = make(map[string]string, len(p.headers))
			maps.Copy(merged, existing)
		}
		merged[h] = v
	}

	if merged == nil {
		return ctx
	}
	return context.WithValue(ctx, forwardHeadersContextKey{}, merged)
}

// Inject 从 context 中取出透传值写入 carrier，只写入配置的 Header
func (p *HeaderPropagator) Inject(ctx context.Context, carrier propagation.TextMapCarrier) {
	if len(p.headers) == 0 {
		return
	}

	vals := forwardHeadersFromContextRaw(ctx)
	if len(vals) == 0 {
		return
	}

	for _, h := range p.headers {
		if v, ok := vals[h]; ok && v != "" {
			carrier.Set(h, v)
		}
	}
}

// Fields 返回该 Propagator 管理的 Header 列表（拷贝）
func (p *HeaderPropagator) Fields() []string {
	cp := make([]string, len(p.headers))
	copy(cp, p.headers)
	return cp
}

// forwardHeadersFromContextRaw 从 context 中获取原始的透传 Header map
func forwardHeadersFromContextRaw(ctx context.Context) map[string]string {
	val := ctx.Value(forwardHeadersContextKey{})
	if val == nil {
		return nil
	}
	m, ok := val.(map[string]string)
	if !ok {
		return nil
	}
	return m
}

// ForwardHeadersFromContext 从 context 中获取所有透传的 Header 键值对（返回拷贝）
func ForwardHeadersFromContext(ctx context.Context) map[string]string {
	m := forwardHeadersFromContextRaw(ctx)
	if len(m) == 0 {
		return nil
	}
	cp := make(map[string]string, len(m))
	maps.Copy(cp, m)
	return cp
}

// ForwardHeaderFromContext 从 context 中获取指定 Header 的值，大小写不敏感
func ForwardHeaderFromContext(ctx context.Context, key string) string {
	m := forwardHeadersFromContextRaw(ctx)
	if len(m) == 0 {
		return ""
	}
	return m[http.CanonicalHeaderKey(key)]
}
