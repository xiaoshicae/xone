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
			p := NewHeaderPropagator([]string{"x-request-id", "X-TENANT-ID"})
			So(p.headers, ShouldResemble, []string{"X-Request-Id", "X-Tenant-Id"})
		})

		PatchConvey("EmptyList", func() {
			p := NewHeaderPropagator([]string{})
			So(p.headers, ShouldBeEmpty)
		})

		PatchConvey("NilList", func() {
			p := NewHeaderPropagator(nil)
			So(p.headers, ShouldBeEmpty)
		})

		PatchConvey("FilterEmptyStrings", func() {
			p := NewHeaderPropagator([]string{"x-request-id", "", "x-tenant-id"})
			So(p.headers, ShouldResemble, []string{"X-Request-Id", "X-Tenant-Id"})
		})
	})
}

// ==================== Extract ====================

func TestHeaderPropagator_Extract(t *testing.T) {
	PatchConvey("TestHeaderPropagator_Extract", t, func() {
		PatchConvey("AllMatched", func() {
			p := NewHeaderPropagator([]string{"X-Request-Id", "X-Tenant-Id"})
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
			p := NewHeaderPropagator([]string{"X-Request-Id", "X-Tenant-Id"})
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
			p := NewHeaderPropagator([]string{"X-Request-Id"})
			carrier := mapCarrier{}
			ctx := p.Extract(context.Background(), carrier)
			m := forwardHeadersFromContextRaw(ctx)
			So(m, ShouldBeNil)
		})

		PatchConvey("MergeExisting", func() {
			p := NewHeaderPropagator([]string{"X-Request-Id", "X-Tenant-Id"})
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
			p := NewHeaderPropagator([]string{"X-Request-Id"})
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
			p := NewHeaderPropagator([]string{})
			carrier := mapCarrier{"X-Request-Id": "abc123"}
			ctx := p.Extract(context.Background(), carrier)
			// 空 headers 不应修改 context
			m := forwardHeadersFromContextRaw(ctx)
			So(m, ShouldBeNil)
		})
	})
}

// ==================== Inject ====================

func TestHeaderPropagator_Inject(t *testing.T) {
	PatchConvey("TestHeaderPropagator_Inject", t, func() {
		PatchConvey("NormalInject", func() {
			p := NewHeaderPropagator([]string{"X-Request-Id", "X-Tenant-Id"})
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
			p := NewHeaderPropagator([]string{"X-Request-Id", "X-Tenant-Id"})
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
			p := NewHeaderPropagator([]string{"X-Request-Id"})
			carrier := mapCarrier{}
			p.Inject(context.Background(), carrier)
			So(carrier, ShouldBeEmpty)
		})

		PatchConvey("OnlyConfiguredHeaders", func() {
			// context 中有额外 Header，但 Propagator 只注入配置的
			p := NewHeaderPropagator([]string{"X-Request-Id"})
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
			p := NewHeaderPropagator([]string{})
			vals := map[string]string{"X-Request-Id": "abc123"}
			ctx := context.WithValue(context.Background(), forwardHeadersContextKey{}, vals)
			carrier := mapCarrier{}
			p.Inject(ctx, carrier)
			So(carrier, ShouldBeEmpty)
		})
	})
}

// ==================== Fields ====================

func TestHeaderPropagator_Fields(t *testing.T) {
	PatchConvey("TestHeaderPropagator_Fields", t, func() {
		PatchConvey("ReturnsCopy", func() {
			p := NewHeaderPropagator([]string{"X-Request-Id", "X-Tenant-Id"})
			fields := p.Fields()
			So(fields, ShouldResemble, []string{"X-Request-Id", "X-Tenant-Id"})

			// 修改返回值不影响原始数据
			fields[0] = "modified"
			So(p.headers[0], ShouldEqual, "X-Request-Id")
		})

		PatchConvey("Empty", func() {
			p := NewHeaderPropagator([]string{})
			fields := p.Fields()
			So(fields, ShouldBeEmpty)
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

// ==================== 端到端 Round-Trip ====================

func TestHeaderPropagator_RoundTrip(t *testing.T) {
	PatchConvey("TestHeaderPropagator_RoundTrip", t, func() {
		headers := []string{"X-Request-Id", "X-Tenant-Id", "X-User-Id"}
		p := NewHeaderPropagator(headers)

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
}

// ==================== 接口兼容性验证 ====================

func TestHeaderPropagator_ImplementsInterface(t *testing.T) {
	PatchConvey("TestHeaderPropagator_ImplementsInterface", t, func() {
		var _ propagation.TextMapPropagator = (*HeaderPropagator)(nil)
	})
}
