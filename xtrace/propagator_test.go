package xtrace

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel/propagation"

	. "github.com/bytedance/mockey"
	. "github.com/smartystreets/goconvey/convey"
)

// mapCarrier 用于测试的 TextMapCarrier 实现
type mapCarrier map[string]string

func (c mapCarrier) Get(key string) string { return c[key] }
func (c mapCarrier) Set(key, value string) { c[key] = value }
func (c mapCarrier) Keys() []string {
	keys := make([]string, 0, len(c))
	for k := range c {
		keys = append(keys, k)
	}
	return keys
}

// ==================== NewHeaderPropagator ====================

func TestNewHeaderPropagator(t *testing.T) {
	PatchConvey("TestNewHeaderPropagator", t, func() {
		PatchConvey("Normalize", func() {
			p := NewHeaderPropagator([]string{"x-request-id", "X-TENANT-ID"}, nil)
			So(p.globalHeaders, ShouldResemble, []string{"X-Request-Id", "X-Tenant-Id"})
			So(p.allHeaders, ShouldResemble, []string{"X-Request-Id", "X-Tenant-Id"})
		})

		PatchConvey("EmptyList", func() {
			p := NewHeaderPropagator([]string{}, nil)
			So(p.globalHeaders, ShouldBeEmpty)
			So(p.allHeaders, ShouldBeEmpty)
		})

		PatchConvey("NilList", func() {
			p := NewHeaderPropagator(nil, nil)
			So(p.globalHeaders, ShouldBeEmpty)
			So(p.allHeaders, ShouldBeEmpty)
		})

		PatchConvey("FilterEmptyStrings", func() {
			p := NewHeaderPropagator([]string{"x-request-id", "", "x-tenant-id"}, nil)
			So(p.globalHeaders, ShouldResemble, []string{"X-Request-Id", "X-Tenant-Id"})
		})

		PatchConvey("WithRules", func() {
			rules := []ForwardHeaderRule{
				{
					Domains: []string{"api.example.com", "*.internal.com"},
					Headers: []string{"X-Auth-Token", "X-Tenant-Id"},
				},
			}
			p := NewHeaderPropagator([]string{"X-Request-Id"}, rules)
			So(p.globalHeaders, ShouldResemble, []string{"X-Request-Id"})
			So(len(p.rules), ShouldEqual, 1)
			So(p.rules[0].domains, ShouldResemble, []string{"api.example.com", "*.internal.com"})
			So(p.rules[0].headers, ShouldResemble, []string{"X-Auth-Token", "X-Tenant-Id"})
			// allHeaders 包含全局 + 规则中的 header，去重
			So(p.allHeaders, ShouldResemble, []string{"X-Request-Id", "X-Auth-Token", "X-Tenant-Id"})
		})

		PatchConvey("RulesDedup", func() {
			// 全局和规则中有重复 header
			rules := []ForwardHeaderRule{
				{Domains: []string{"a.com"}, Headers: []string{"X-Request-Id", "X-Auth"}},
			}
			p := NewHeaderPropagator([]string{"X-Request-Id"}, rules)
			So(p.allHeaders, ShouldResemble, []string{"X-Request-Id", "X-Auth"})
		})

		PatchConvey("SkipEmptyRules", func() {
			rules := []ForwardHeaderRule{
				{Domains: []string{}, Headers: []string{"X-Auth"}},        // 无域名，跳过
				{Domains: []string{"a.com"}, Headers: []string{}},         // 无 header，跳过
				{Domains: []string{""}, Headers: []string{"X-Auth"}},      // 域名为空串，跳过
				{Domains: []string{"a.com"}, Headers: []string{"X-Auth"}}, // 有效
			}
			p := NewHeaderPropagator(nil, rules)
			So(len(p.rules), ShouldEqual, 1)
			So(p.rules[0].domains, ShouldResemble, []string{"a.com"})
		})
	})
}

// ==================== Extract ====================

func TestHeaderPropagator_Extract(t *testing.T) {
	PatchConvey("TestHeaderPropagator_Extract", t, func() {
		PatchConvey("AllMatched", func() {
			p := NewHeaderPropagator([]string{"X-Request-Id", "X-Tenant-Id"}, nil)
			carrier := mapCarrier{
				"X-Request-Id": "abc123",
				"X-Tenant-Id":  "tenant-1",
			}
			ctx := p.Extract(context.Background(), carrier)
			m := forwardHeadersFromContextRaw(ctx)
			So(m, ShouldNotBeNil)
			So(m["X-Request-Id"], ShouldEqual, "abc123")
			So(m["X-Tenant-Id"], ShouldEqual, "tenant-1")
		})

		PatchConvey("PartialMatched", func() {
			p := NewHeaderPropagator([]string{"X-Request-Id", "X-Tenant-Id"}, nil)
			carrier := mapCarrier{
				"X-Request-Id": "abc123",
			}
			ctx := p.Extract(context.Background(), carrier)
			m := forwardHeadersFromContextRaw(ctx)
			So(m, ShouldNotBeNil)
			So(m["X-Request-Id"], ShouldEqual, "abc123")
			So(m["X-Tenant-Id"], ShouldEqual, "")
		})

		PatchConvey("NoMatched", func() {
			p := NewHeaderPropagator([]string{"X-Request-Id"}, nil)
			carrier := mapCarrier{}
			ctx := p.Extract(context.Background(), carrier)
			m := forwardHeadersFromContextRaw(ctx)
			So(m, ShouldBeNil)
		})

		PatchConvey("MergeExisting", func() {
			p := NewHeaderPropagator([]string{"X-Request-Id", "X-Tenant-Id"}, nil)
			// 先注入已有值
			existing := map[string]string{"X-Request-Id": "old-value"}
			ctx := context.WithValue(context.Background(), forwardHeadersContextKey{}, existing)

			carrier := mapCarrier{
				"X-Tenant-Id": "tenant-1",
			}
			ctx = p.Extract(ctx, carrier)
			m := forwardHeadersFromContextRaw(ctx)
			So(m, ShouldNotBeNil)
			So(m["X-Request-Id"], ShouldEqual, "old-value")
			So(m["X-Tenant-Id"], ShouldEqual, "tenant-1")
		})

		PatchConvey("OverwriteExisting", func() {
			p := NewHeaderPropagator([]string{"X-Request-Id"}, nil)
			existing := map[string]string{"X-Request-Id": "old-value"}
			ctx := context.WithValue(context.Background(), forwardHeadersContextKey{}, existing)

			carrier := mapCarrier{
				"X-Request-Id": "new-value",
			}
			ctx = p.Extract(ctx, carrier)
			m := forwardHeadersFromContextRaw(ctx)
			So(m["X-Request-Id"], ShouldEqual, "new-value")
		})

		PatchConvey("EmptyHeaders", func() {
			p := NewHeaderPropagator([]string{}, nil)
			carrier := mapCarrier{"X-Request-Id": "abc123"}
			ctx := p.Extract(context.Background(), carrier)
			// 空 headers 不应修改 context
			m := forwardHeadersFromContextRaw(ctx)
			So(m, ShouldBeNil)
		})

		PatchConvey("ExtractBothGlobalAndRuleHeaders", func() {
			// Extract 应提取所有配置的 header（全局 + 规则），不区分域名
			rules := []ForwardHeaderRule{
				{Domains: []string{"a.com"}, Headers: []string{"X-Auth-Token"}},
			}
			p := NewHeaderPropagator([]string{"X-Request-Id"}, rules)
			carrier := mapCarrier{
				"X-Request-Id": "req-001",
				"X-Auth-Token": "token-abc",
			}
			ctx := p.Extract(context.Background(), carrier)
			m := forwardHeadersFromContextRaw(ctx)
			So(m, ShouldNotBeNil)
			So(m["X-Request-Id"], ShouldEqual, "req-001")
			So(m["X-Auth-Token"], ShouldEqual, "token-abc")
		})
	})
}

// ==================== Inject ====================

func TestHeaderPropagator_Inject(t *testing.T) {
	PatchConvey("TestHeaderPropagator_Inject", t, func() {
		PatchConvey("NormalInject", func() {
			p := NewHeaderPropagator([]string{"X-Request-Id", "X-Tenant-Id"}, nil)
			vals := map[string]string{
				"X-Request-Id": "abc123",
				"X-Tenant-Id":  "tenant-1",
			}
			ctx := context.WithValue(context.Background(), forwardHeadersContextKey{}, vals)
			carrier := mapCarrier{}
			p.Inject(ctx, carrier)
			So(carrier["X-Request-Id"], ShouldEqual, "abc123")
			So(carrier["X-Tenant-Id"], ShouldEqual, "tenant-1")
		})

		PatchConvey("PartialInject", func() {
			p := NewHeaderPropagator([]string{"X-Request-Id", "X-Tenant-Id"}, nil)
			vals := map[string]string{
				"X-Request-Id": "abc123",
			}
			ctx := context.WithValue(context.Background(), forwardHeadersContextKey{}, vals)
			carrier := mapCarrier{}
			p.Inject(ctx, carrier)
			So(carrier["X-Request-Id"], ShouldEqual, "abc123")
			So(carrier["X-Tenant-Id"], ShouldEqual, "")
		})

		PatchConvey("EmptyContext", func() {
			p := NewHeaderPropagator([]string{"X-Request-Id"}, nil)
			carrier := mapCarrier{}
			p.Inject(context.Background(), carrier)
			So(carrier, ShouldBeEmpty)
		})

		PatchConvey("OnlyConfiguredHeaders", func() {
			// context 中有额外 Header，但 Propagator 只注入配置的
			p := NewHeaderPropagator([]string{"X-Request-Id"}, nil)
			vals := map[string]string{
				"X-Request-Id": "abc123",
				"X-Extra":      "should-not-inject",
			}
			ctx := context.WithValue(context.Background(), forwardHeadersContextKey{}, vals)
			carrier := mapCarrier{}
			p.Inject(ctx, carrier)
			So(carrier["X-Request-Id"], ShouldEqual, "abc123")
			So(carrier["X-Extra"], ShouldEqual, "")
		})

		PatchConvey("EmptyHeaders", func() {
			p := NewHeaderPropagator([]string{}, nil)
			vals := map[string]string{"X-Request-Id": "abc123"}
			ctx := context.WithValue(context.Background(), forwardHeadersContextKey{}, vals)
			carrier := mapCarrier{}
			p.Inject(ctx, carrier)
			So(carrier, ShouldBeEmpty)
		})

		PatchConvey("RuleHeaders_MatchedDomain", func() {
			rules := []ForwardHeaderRule{
				{Domains: []string{"api.internal.com"}, Headers: []string{"X-Auth-Token"}},
			}
			p := NewHeaderPropagator([]string{"X-Request-Id"}, rules)
			vals := map[string]string{
				"X-Request-Id": "req-001",
				"X-Auth-Token": "secret-token",
			}
			ctx := context.WithValue(context.Background(), forwardHeadersContextKey{}, vals)
			ctx = WithTargetHost(ctx, "api.internal.com")
			carrier := mapCarrier{}
			p.Inject(ctx, carrier)
			// 全局 header 始终注入
			So(carrier["X-Request-Id"], ShouldEqual, "req-001")
			// 域名匹配，规则 header 也注入
			So(carrier["X-Auth-Token"], ShouldEqual, "secret-token")
		})

		PatchConvey("RuleHeaders_UnmatchedDomain", func() {
			rules := []ForwardHeaderRule{
				{Domains: []string{"api.internal.com"}, Headers: []string{"X-Auth-Token"}},
			}
			p := NewHeaderPropagator([]string{"X-Request-Id"}, rules)
			vals := map[string]string{
				"X-Request-Id": "req-001",
				"X-Auth-Token": "secret-token",
			}
			ctx := context.WithValue(context.Background(), forwardHeadersContextKey{}, vals)
			ctx = WithTargetHost(ctx, "api.external.com")
			carrier := mapCarrier{}
			p.Inject(ctx, carrier)
			// 全局 header 始终注入
			So(carrier["X-Request-Id"], ShouldEqual, "req-001")
			// 域名不匹配，规则 header 不注入
			So(carrier["X-Auth-Token"], ShouldEqual, "")
		})

		PatchConvey("RuleHeaders_NoHostInContext", func() {
			// context 中没有目标 host 时，规则 header 不注入（安全兜底）
			rules := []ForwardHeaderRule{
				{Domains: []string{"api.internal.com"}, Headers: []string{"X-Auth-Token"}},
			}
			p := NewHeaderPropagator([]string{"X-Request-Id"}, rules)
			vals := map[string]string{
				"X-Request-Id": "req-001",
				"X-Auth-Token": "secret-token",
			}
			ctx := context.WithValue(context.Background(), forwardHeadersContextKey{}, vals)
			// 不设置 target host
			carrier := mapCarrier{}
			p.Inject(ctx, carrier)
			So(carrier["X-Request-Id"], ShouldEqual, "req-001")
			So(carrier["X-Auth-Token"], ShouldEqual, "")
		})

		PatchConvey("RuleHeaders_WildcardDomain", func() {
			rules := []ForwardHeaderRule{
				{Domains: []string{"*.internal.com"}, Headers: []string{"X-Auth-Token"}},
			}
			p := NewHeaderPropagator(nil, rules)
			vals := map[string]string{"X-Auth-Token": "token-123"}
			ctx := context.WithValue(context.Background(), forwardHeadersContextKey{}, vals)
			ctx = WithTargetHost(ctx, "api.internal.com")
			carrier := mapCarrier{}
			p.Inject(ctx, carrier)
			So(carrier["X-Auth-Token"], ShouldEqual, "token-123")
		})

		PatchConvey("RuleHeaders_WildcardDomain_NoMatch", func() {
			rules := []ForwardHeaderRule{
				{Domains: []string{"*.internal.com"}, Headers: []string{"X-Auth-Token"}},
			}
			p := NewHeaderPropagator(nil, rules)
			vals := map[string]string{"X-Auth-Token": "token-123"}
			ctx := context.WithValue(context.Background(), forwardHeadersContextKey{}, vals)
			ctx = WithTargetHost(ctx, "api.external.com")
			carrier := mapCarrier{}
			p.Inject(ctx, carrier)
			So(carrier["X-Auth-Token"], ShouldEqual, "")
		})

		PatchConvey("RuleHeaders_HostWithPort", func() {
			rules := []ForwardHeaderRule{
				{Domains: []string{"api.internal.com"}, Headers: []string{"X-Auth-Token"}},
			}
			p := NewHeaderPropagator(nil, rules)
			vals := map[string]string{"X-Auth-Token": "token-123"}
			ctx := context.WithValue(context.Background(), forwardHeadersContextKey{}, vals)
			ctx = WithTargetHost(ctx, "api.internal.com:8080")
			carrier := mapCarrier{}
			p.Inject(ctx, carrier)
			// 带端口也能匹配
			So(carrier["X-Auth-Token"], ShouldEqual, "token-123")
		})

		PatchConvey("RuleHeaders_MultipleRules", func() {
			rules := []ForwardHeaderRule{
				{Domains: []string{"a.com"}, Headers: []string{"X-Auth-A"}},
				{Domains: []string{"b.com"}, Headers: []string{"X-Auth-B"}},
			}
			p := NewHeaderPropagator(nil, rules)
			vals := map[string]string{"X-Auth-A": "aaa", "X-Auth-B": "bbb"}
			ctx := context.WithValue(context.Background(), forwardHeadersContextKey{}, vals)

			// 匹配 a.com
			ctx1 := WithTargetHost(ctx, "a.com")
			carrier1 := mapCarrier{}
			p.Inject(ctx1, carrier1)
			So(carrier1["X-Auth-A"], ShouldEqual, "aaa")
			So(carrier1["X-Auth-B"], ShouldEqual, "")

			// 匹配 b.com
			ctx2 := WithTargetHost(ctx, "b.com")
			carrier2 := mapCarrier{}
			p.Inject(ctx2, carrier2)
			So(carrier2["X-Auth-A"], ShouldEqual, "")
			So(carrier2["X-Auth-B"], ShouldEqual, "bbb")
		})

		PatchConvey("SameHeaderInGlobalAndRule_DomainNotMatch", func() {
			// 同一 header 同时出现在全局和规则中，即使域名不匹配，全局也应注入
			rules := []ForwardHeaderRule{
				{Domains: []string{"api.internal.com"}, Headers: []string{"X-Request-Id"}},
			}
			p := NewHeaderPropagator([]string{"X-Request-Id"}, rules)
			vals := map[string]string{"X-Request-Id": "req-001"}
			ctx := context.WithValue(context.Background(), forwardHeadersContextKey{}, vals)
			ctx = WithTargetHost(ctx, "api.external.com")
			carrier := mapCarrier{}
			p.Inject(ctx, carrier)
			// 全局 header 不受域名限制，始终注入
			So(carrier["X-Request-Id"], ShouldEqual, "req-001")
		})

		PatchConvey("EmptyHostString", func() {
			// WithTargetHost("") 应等同于无 host，规则 header 不注入
			rules := []ForwardHeaderRule{
				{Domains: []string{"api.internal.com"}, Headers: []string{"X-Auth-Token"}},
			}
			p := NewHeaderPropagator([]string{"X-Request-Id"}, rules)
			vals := map[string]string{
				"X-Request-Id": "req-001",
				"X-Auth-Token": "secret",
			}
			ctx := context.WithValue(context.Background(), forwardHeadersContextKey{}, vals)
			ctx = WithTargetHost(ctx, "")
			carrier := mapCarrier{}
			p.Inject(ctx, carrier)
			So(carrier["X-Request-Id"], ShouldEqual, "req-001")
			So(carrier["X-Auth-Token"], ShouldEqual, "")
		})

		PatchConvey("GlobalHeader_EmptyValue", func() {
			// 全局 header 值为空字符串时不应注入
			p := NewHeaderPropagator([]string{"X-Request-Id"}, nil)
			vals := map[string]string{"X-Request-Id": ""}
			ctx := context.WithValue(context.Background(), forwardHeadersContextKey{}, vals)
			carrier := mapCarrier{}
			p.Inject(ctx, carrier)
			So(carrier["X-Request-Id"], ShouldEqual, "")
		})

		PatchConvey("RuleHeader_EmptyValue", func() {
			// 规则 header 值为空字符串时不应注入
			rules := []ForwardHeaderRule{
				{Domains: []string{"a.com"}, Headers: []string{"X-Auth-Token"}},
			}
			p := NewHeaderPropagator(nil, rules)
			vals := map[string]string{"X-Auth-Token": ""}
			ctx := context.WithValue(context.Background(), forwardHeadersContextKey{}, vals)
			ctx = WithTargetHost(ctx, "a.com")
			carrier := mapCarrier{}
			p.Inject(ctx, carrier)
			So(carrier["X-Auth-Token"], ShouldEqual, "")
		})

		PatchConvey("MultiDomainsInSingleRule", func() {
			// 一条规则包含多个域名，任一匹配即注入
			rules := []ForwardHeaderRule{
				{Domains: []string{"a.com", "b.com", "*.c.com"}, Headers: []string{"X-Auth"}},
			}
			p := NewHeaderPropagator(nil, rules)
			vals := map[string]string{"X-Auth": "token"}
			ctx := context.WithValue(context.Background(), forwardHeadersContextKey{}, vals)

			// 匹配第一个域名
			ctx1 := WithTargetHost(ctx, "a.com")
			c1 := mapCarrier{}
			p.Inject(ctx1, c1)
			So(c1["X-Auth"], ShouldEqual, "token")

			// 匹配第二个域名
			ctx2 := WithTargetHost(ctx, "b.com")
			c2 := mapCarrier{}
			p.Inject(ctx2, c2)
			So(c2["X-Auth"], ShouldEqual, "token")

			// 匹配通配域名
			ctx3 := WithTargetHost(ctx, "sub.c.com")
			c3 := mapCarrier{}
			p.Inject(ctx3, c3)
			So(c3["X-Auth"], ShouldEqual, "token")

			// 不匹配任何域名
			ctx4 := WithTargetHost(ctx, "d.com")
			c4 := mapCarrier{}
			p.Inject(ctx4, c4)
			So(c4["X-Auth"], ShouldEqual, "")
		})

		PatchConvey("OnlyRules_NoHost_EmptyCarrier", func() {
			// 仅配置规则无全局，且 context 无 host 时，carrier 应为空
			rules := []ForwardHeaderRule{
				{Domains: []string{"a.com"}, Headers: []string{"X-Auth"}},
			}
			p := NewHeaderPropagator(nil, rules)
			vals := map[string]string{"X-Auth": "token"}
			ctx := context.WithValue(context.Background(), forwardHeadersContextKey{}, vals)
			carrier := mapCarrier{}
			p.Inject(ctx, carrier)
			So(carrier, ShouldBeEmpty)
		})

		PatchConvey("WildcardDeepSubdomain_WithPort", func() {
			// 深层子域名+端口：deep.sub.internal.com:443 应匹配 *.internal.com
			rules := []ForwardHeaderRule{
				{Domains: []string{"*.internal.com"}, Headers: []string{"X-Auth"}},
			}
			p := NewHeaderPropagator(nil, rules)
			vals := map[string]string{"X-Auth": "deep-token"}
			ctx := context.WithValue(context.Background(), forwardHeadersContextKey{}, vals)
			ctx = WithTargetHost(ctx, "deep.sub.internal.com:443")
			carrier := mapCarrier{}
			p.Inject(ctx, carrier)
			So(carrier["X-Auth"], ShouldEqual, "deep-token")
		})

		PatchConvey("DomainCaseInsensitive", func() {
			// 域名匹配应大小写不敏感
			rules := []ForwardHeaderRule{
				{Domains: []string{"API.Internal.COM"}, Headers: []string{"X-Auth"}},
			}
			p := NewHeaderPropagator(nil, rules)
			vals := map[string]string{"X-Auth": "token"}
			ctx := context.WithValue(context.Background(), forwardHeadersContextKey{}, vals)
			ctx = WithTargetHost(ctx, "api.internal.com")
			carrier := mapCarrier{}
			p.Inject(ctx, carrier)
			So(carrier["X-Auth"], ShouldEqual, "token")
		})

		PatchConvey("MultipleRulesMatchSameHost", func() {
			// 多条规则都匹配同一 host，所有匹配规则的 header 都应注入
			rules := []ForwardHeaderRule{
				{Domains: []string{"api.internal.com"}, Headers: []string{"X-Auth-A"}},
				{Domains: []string{"*.internal.com"}, Headers: []string{"X-Auth-B"}},
			}
			p := NewHeaderPropagator(nil, rules)
			vals := map[string]string{"X-Auth-A": "aaa", "X-Auth-B": "bbb"}
			ctx := context.WithValue(context.Background(), forwardHeadersContextKey{}, vals)
			ctx = WithTargetHost(ctx, "api.internal.com")
			carrier := mapCarrier{}
			p.Inject(ctx, carrier)
			// 两条规则都匹配，两个 header 都应注入
			So(carrier["X-Auth-A"], ShouldEqual, "aaa")
			So(carrier["X-Auth-B"], ShouldEqual, "bbb")
		})
	})
}

// ==================== Fields ====================

func TestHeaderPropagator_Fields(t *testing.T) {
	PatchConvey("TestHeaderPropagator_Fields", t, func() {
		PatchConvey("ReturnsCopy", func() {
			p := NewHeaderPropagator([]string{"X-Request-Id", "X-Tenant-Id"}, nil)
			fields := p.Fields()
			So(fields, ShouldResemble, []string{"X-Request-Id", "X-Tenant-Id"})

			// 修改返回值不影响原始数据
			fields[0] = "modified"
			So(p.allHeaders[0], ShouldEqual, "X-Request-Id")
		})

		PatchConvey("Empty", func() {
			p := NewHeaderPropagator([]string{}, nil)
			fields := p.Fields()
			So(fields, ShouldBeEmpty)
		})

		PatchConvey("IncludesRuleHeaders", func() {
			rules := []ForwardHeaderRule{
				{Domains: []string{"a.com"}, Headers: []string{"X-Auth"}},
			}
			p := NewHeaderPropagator([]string{"X-Request-Id"}, rules)
			fields := p.Fields()
			So(fields, ShouldResemble, []string{"X-Request-Id", "X-Auth"})
		})
	})
}

// ==================== ForwardHeadersFromContext ====================

func TestForwardHeadersFromContext(t *testing.T) {
	PatchConvey("TestForwardHeadersFromContext", t, func() {
		PatchConvey("NilContext", func() {
			ctx := context.Background()
			m := ForwardHeadersFromContext(ctx)
			So(m, ShouldBeNil)
		})

		PatchConvey("WrongType", func() {
			ctx := context.WithValue(context.Background(), forwardHeadersContextKey{}, "wrong-type")
			m := ForwardHeadersFromContext(ctx)
			So(m, ShouldBeNil)
		})

		PatchConvey("NormalMap", func() {
			vals := map[string]string{"X-Request-Id": "abc123"}
			ctx := context.WithValue(context.Background(), forwardHeadersContextKey{}, vals)
			m := ForwardHeadersFromContext(ctx)
			So(m, ShouldResemble, map[string]string{"X-Request-Id": "abc123"})

			// 返回的是拷贝，修改不影响原始
			m["X-Request-Id"] = "modified"
			So(vals["X-Request-Id"], ShouldEqual, "abc123")
		})

		PatchConvey("EmptyMap", func() {
			vals := map[string]string{}
			ctx := context.WithValue(context.Background(), forwardHeadersContextKey{}, vals)
			m := ForwardHeadersFromContext(ctx)
			So(m, ShouldBeNil)
		})
	})
}

// ==================== ForwardHeaderFromContext ====================

func TestForwardHeaderFromContext(t *testing.T) {
	PatchConvey("TestForwardHeaderFromContext", t, func() {
		PatchConvey("CaseInsensitive", func() {
			vals := map[string]string{"X-Request-Id": "abc123"}
			ctx := context.WithValue(context.Background(), forwardHeadersContextKey{}, vals)
			So(ForwardHeaderFromContext(ctx, "x-request-id"), ShouldEqual, "abc123")
			So(ForwardHeaderFromContext(ctx, "X-REQUEST-ID"), ShouldEqual, "abc123")
			So(ForwardHeaderFromContext(ctx, "X-Request-Id"), ShouldEqual, "abc123")
		})

		PatchConvey("NotExist", func() {
			vals := map[string]string{"X-Request-Id": "abc123"}
			ctx := context.WithValue(context.Background(), forwardHeadersContextKey{}, vals)
			So(ForwardHeaderFromContext(ctx, "X-Missing"), ShouldEqual, "")
		})

		PatchConvey("EmptyContext", func() {
			So(ForwardHeaderFromContext(context.Background(), "X-Request-Id"), ShouldEqual, "")
		})
	})
}

// ==================== matchDomains ====================

func TestMatchDomains(t *testing.T) {
	PatchConvey("TestMatchDomains", t, func() {
		PatchConvey("ExactMatch", func() {
			So(matchDomains("api.example.com", []string{"api.example.com"}), ShouldBeTrue)
		})

		PatchConvey("ExactMatch_CaseInsensitive", func() {
			So(matchDomains("API.Example.COM", []string{"api.example.com"}), ShouldBeTrue)
		})

		PatchConvey("ExactMatch_NoMatch", func() {
			So(matchDomains("other.example.com", []string{"api.example.com"}), ShouldBeFalse)
		})

		PatchConvey("Wildcard_SubdomainMatch", func() {
			So(matchDomains("api.example.com", []string{"*.example.com"}), ShouldBeTrue)
		})

		PatchConvey("Wildcard_DeepSubdomain", func() {
			So(matchDomains("deep.sub.example.com", []string{"*.example.com"}), ShouldBeTrue)
		})

		PatchConvey("Wildcard_ExactDomainNoMatch", func() {
			// *.example.com 不匹配 example.com 本身
			So(matchDomains("example.com", []string{"*.example.com"}), ShouldBeFalse)
		})

		PatchConvey("Wildcard_NoMatch", func() {
			So(matchDomains("api.other.com", []string{"*.example.com"}), ShouldBeFalse)
		})

		PatchConvey("WithPort", func() {
			So(matchDomains("api.example.com:8080", []string{"api.example.com"}), ShouldBeTrue)
			So(matchDomains("api.example.com:443", []string{"*.example.com"}), ShouldBeTrue)
		})

		PatchConvey("MultiplePatterns", func() {
			patterns := []string{"a.com", "*.b.com"}
			So(matchDomains("a.com", patterns), ShouldBeTrue)
			So(matchDomains("sub.b.com", patterns), ShouldBeTrue)
			So(matchDomains("c.com", patterns), ShouldBeFalse)
		})
	})
}

// ==================== WithTargetHost / targetHostFromContext ====================

func TestTargetHostContext(t *testing.T) {
	PatchConvey("TestTargetHostContext", t, func() {
		PatchConvey("SetAndGet", func() {
			ctx := WithTargetHost(context.Background(), "api.example.com:8080")
			So(targetHostFromContext(ctx), ShouldEqual, "api.example.com:8080")
		})

		PatchConvey("EmptyContext", func() {
			So(targetHostFromContext(context.Background()), ShouldEqual, "")
		})

		PatchConvey("WrongType", func() {
			ctx := context.WithValue(context.Background(), targetHostContextKey{}, 12345)
			So(targetHostFromContext(ctx), ShouldEqual, "")
		})
	})
}

// ==================== 端到端 Round-Trip ====================

func TestHeaderPropagator_RoundTrip(t *testing.T) {
	PatchConvey("TestHeaderPropagator_RoundTrip", t, func() {
		PatchConvey("GlobalOnly", func() {
			headers := []string{"X-Request-Id", "X-Tenant-Id", "X-User-Id"}
			p := NewHeaderPropagator(headers, nil)

			// 模拟上游请求 Header
			inboundCarrier := mapCarrier{
				"X-Request-Id": "req-001",
				"X-Tenant-Id":  "tenant-abc",
				"X-User-Id":    "user-42",
				"X-Other":      "should-ignore",
			}

			// Extract：从上游请求中提取
			ctx := p.Extract(context.Background(), inboundCarrier)

			// Inject：注入到下游请求
			outboundCarrier := mapCarrier{}
			p.Inject(ctx, outboundCarrier)

			// 验证：配置的 Header 完整透传
			So(outboundCarrier["X-Request-Id"], ShouldEqual, "req-001")
			So(outboundCarrier["X-Tenant-Id"], ShouldEqual, "tenant-abc")
			So(outboundCarrier["X-User-Id"], ShouldEqual, "user-42")
			// 未配置的 Header 不透传
			So(outboundCarrier["X-Other"], ShouldEqual, "")
		})

		PatchConvey("WithDomainRules", func() {
			rules := []ForwardHeaderRule{
				{Domains: []string{"api.internal.com", "*.mycompany.com"}, Headers: []string{"X-Auth-Token"}},
			}
			p := NewHeaderPropagator([]string{"X-Request-Id"}, rules)

			// 模拟上游请求（包含全局和规则 header）
			inboundCarrier := mapCarrier{
				"X-Request-Id": "req-001",
				"X-Auth-Token": "secret-token",
			}
			ctx := p.Extract(context.Background(), inboundCarrier)

			// 场景1：请求内部域名 → 全局 + 规则 header 都透传
			ctx1 := WithTargetHost(ctx, "api.internal.com")
			carrier1 := mapCarrier{}
			p.Inject(ctx1, carrier1)
			So(carrier1["X-Request-Id"], ShouldEqual, "req-001")
			So(carrier1["X-Auth-Token"], ShouldEqual, "secret-token")

			// 场景2：请求通配域名 → 全局 + 规则 header 都透传
			ctx2 := WithTargetHost(ctx, "svc.mycompany.com")
			carrier2 := mapCarrier{}
			p.Inject(ctx2, carrier2)
			So(carrier2["X-Request-Id"], ShouldEqual, "req-001")
			So(carrier2["X-Auth-Token"], ShouldEqual, "secret-token")

			// 场景3：请求外部域名 → 仅全局 header 透传，规则 header 不泄漏
			ctx3 := WithTargetHost(ctx, "api.thirdparty.com")
			carrier3 := mapCarrier{}
			p.Inject(ctx3, carrier3)
			So(carrier3["X-Request-Id"], ShouldEqual, "req-001")
			So(carrier3["X-Auth-Token"], ShouldEqual, "")
		})
	})
}

// ==================== 接口兼容性验证 ====================

func TestHeaderPropagator_ImplementsInterface(t *testing.T) {
	PatchConvey("TestHeaderPropagator_ImplementsInterface", t, func() {
		var _ propagation.TextMapPropagator = (*HeaderPropagator)(nil)
	})
}
