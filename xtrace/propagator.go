package xtrace

import (
	"context"
	"maps"
	"net"
	"net/http"
	"strings"

	"go.opentelemetry.io/otel/propagation"
)

// forwardHeadersContextKey 用于在 context 中存储透传 Header 的 key
type forwardHeadersContextKey struct{}

// targetHostContextKey 用于在 context 中存储目标请求 Host 的 key
type targetHostContextKey struct{}

// headerRule 按域名透传的内部规则（已规范化）
type headerRule struct {
	domains []string // 小写域名模式，支持 *.example.com 通配
	headers []string // 规范化的 header 名称
}

// HeaderPropagator 实现 propagation.TextMapPropagator 接口，
// 用于在链路中透传指定的自定义 HTTP Header（如 X-Request-ID、X-Tenant-ID）
// 支持全局透传和按域名透传两种模式
type HeaderPropagator struct {
	// globalHeaders 对所有域名透传的 header
	globalHeaders []string
	// rules 按域名透传的规则
	rules []headerRule
	// allHeaders 所有 header 的去重集合，用于 Extract 和 Fields
	allHeaders []string
}

// NewHeaderPropagator 创建 HeaderPropagator
// globalHeaders 会向所有域名透传，rules 按域名匹配后透传
// header 名会被 http.CanonicalHeaderKey 规范化，域名会转为小写
func NewHeaderPropagator(globalHeaders []string, rules []ForwardHeaderRule) *HeaderPropagator {
	seen := make(map[string]struct{})

	// 规范化全局 header
	normalizedGlobal := make([]string, 0, len(globalHeaders))
	for _, h := range globalHeaders {
		if h == "" {
			continue
		}
		canonical := http.CanonicalHeaderKey(h)
		normalizedGlobal = append(normalizedGlobal, canonical)
		seen[canonical] = struct{}{}
	}

	// 规范化规则
	normalizedRules := make([]headerRule, 0, len(rules))
	for _, r := range rules {
		if len(r.Domains) == 0 || len(r.Headers) == 0 {
			continue
		}
		domains := make([]string, 0, len(r.Domains))
		for _, d := range r.Domains {
			d = strings.TrimSpace(d)
			if d != "" {
				domains = append(domains, strings.ToLower(d))
			}
		}
		if len(domains) == 0 {
			continue
		}
		headers := make([]string, 0, len(r.Headers))
		for _, h := range r.Headers {
			if h == "" {
				continue
			}
			canonical := http.CanonicalHeaderKey(h)
			headers = append(headers, canonical)
			seen[canonical] = struct{}{}
		}
		if len(headers) == 0 {
			continue
		}
		normalizedRules = append(normalizedRules, headerRule{domains: domains, headers: headers})
	}

	// 构建 allHeaders 去重列表，保持稳定顺序：全局 header 在前，规则 header 在后
	allHeaders := make([]string, 0, len(seen))
	added := make(map[string]struct{})
	for _, h := range normalizedGlobal {
		if _, exists := added[h]; !exists {
			added[h] = struct{}{}
			allHeaders = append(allHeaders, h)
		}
	}
	for _, r := range normalizedRules {
		for _, h := range r.headers {
			if _, exists := added[h]; !exists {
				added[h] = struct{}{}
				allHeaders = append(allHeaders, h)
			}
		}
	}

	return &HeaderPropagator{
		globalHeaders: normalizedGlobal,
		rules:         normalizedRules,
		allHeaders:    allHeaders,
	}
}

// Extract 从 carrier 中读取所有配置的 Header 并存入 context
// 提取所有配置的 header（全局 + 规则），不区分域名
func (p *HeaderPropagator) Extract(ctx context.Context, carrier propagation.TextMapCarrier) context.Context {
	if len(p.allHeaders) == 0 {
		return ctx
	}

	// 获取已有的透传值，合并新值
	existing := forwardHeadersFromContextRaw(ctx)
	var merged map[string]string

	for _, h := range p.allHeaders {
		v := carrier.Get(h)
		if v == "" {
			continue
		}
		if merged == nil {
			// 延迟初始化，只在有值时才创建 map
			merged = make(map[string]string, len(p.allHeaders))
			maps.Copy(merged, existing)
		}
		merged[h] = v
	}

	if merged == nil {
		return ctx
	}
	return context.WithValue(ctx, forwardHeadersContextKey{}, merged)
}

// Inject 从 context 中取出透传值写入 carrier
// 全局 header 直接注入；规则 header 仅当目标域名匹配时注入
func (p *HeaderPropagator) Inject(ctx context.Context, carrier propagation.TextMapCarrier) {
	if len(p.allHeaders) == 0 {
		return
	}

	vals := forwardHeadersFromContextRaw(ctx)
	if len(vals) == 0 {
		return
	}

	// 注入全局 header
	for _, h := range p.globalHeaders {
		if v, ok := vals[h]; ok && v != "" {
			carrier.Set(h, v)
		}
	}

	// 注入域名匹配的规则 header
	if len(p.rules) > 0 {
		host := targetHostFromContext(ctx)
		if host != "" {
			for _, rule := range p.rules {
				if matchDomains(host, rule.domains) {
					for _, h := range rule.headers {
						if v, ok := vals[h]; ok && v != "" {
							carrier.Set(h, v)
						}
					}
				}
			}
		}
	}
}

// Fields 返回该 Propagator 管理的所有 Header 列表（拷贝）
func (p *HeaderPropagator) Fields() []string {
	cp := make([]string, len(p.allHeaders))
	copy(cp, p.allHeaders)
	return cp
}

// WithTargetHost 将目标请求的 Host 存入 context，供 HeaderPropagator 按域名过滤
func WithTargetHost(ctx context.Context, host string) context.Context {
	return context.WithValue(ctx, targetHostContextKey{}, host)
}

// targetHostFromContext 从 context 中获取目标请求的 Host
func targetHostFromContext(ctx context.Context) string {
	val := ctx.Value(targetHostContextKey{})
	if val == nil {
		return ""
	}
	host, ok := val.(string)
	if !ok {
		return ""
	}
	return host
}

// matchDomains 检查 host 是否匹配域名模式列表中的任一模式
// 支持精确匹配和通配符前缀匹配（*.example.com）
func matchDomains(host string, patterns []string) bool {
	// 去除端口
	h, _, err := net.SplitHostPort(host)
	if err != nil {
		h = host // 没有端口，直接使用
	}
	h = strings.ToLower(h)

	for _, pattern := range patterns {
		if strings.HasPrefix(pattern, "*.") {
			// 通配符匹配：*.example.com 匹配 sub.example.com
			suffix := pattern[1:] // ".example.com"
			if strings.HasSuffix(h, suffix) && len(h) > len(suffix) {
				return true
			}
		} else {
			// 精确匹配
			if h == pattern {
				return true
			}
		}
	}
	return false
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
