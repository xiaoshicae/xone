package xtrace

import (
	"context"
	"net/http"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"

	. "github.com/bytedance/mockey"
	. "github.com/smartystreets/goconvey/convey"
)

// mockRoundTripper 用于测试的 RoundTripper，记录收到的请求
type mockRoundTripper struct {
	lastReq *http.Request
}

func (m *mockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	m.lastReq = req
	return &http.Response{StatusCode: 200}, nil
}

func TestHostAwareTransport_RoundTrip(t *testing.T) {
	PatchConvey("TestHostAwareTransport_RoundTrip", t, func() {
		PatchConvey("SetsTargetHostInContext", func() {
			mock := &mockRoundTripper{}
			transport := &HostAwareTransport{Next: mock}

			req, _ := http.NewRequest("GET", "https://api.example.com:8080/users", nil)
			_, err := transport.RoundTrip(req)

			So(err, ShouldBeNil)
			So(mock.lastReq, ShouldNotBeNil)
			// 验证 context 中包含目标 host
			host := targetHostFromContext(mock.lastReq.Context())
			So(host, ShouldEqual, "api.example.com:8080")
		})

		PatchConvey("SetsTargetHostWithoutPort", func() {
			mock := &mockRoundTripper{}
			transport := &HostAwareTransport{Next: mock}

			req, _ := http.NewRequest("GET", "https://api.example.com/users", nil)
			_, err := transport.RoundTrip(req)

			So(err, ShouldBeNil)
			host := targetHostFromContext(mock.lastReq.Context())
			So(host, ShouldEqual, "api.example.com")
		})

		PatchConvey("PreservesOriginalRequest", func() {
			mock := &mockRoundTripper{}
			transport := &HostAwareTransport{Next: mock}

			req, _ := http.NewRequest("POST", "https://api.example.com/data", nil)
			req.Header.Set("Content-Type", "application/json")
			_, err := transport.RoundTrip(req)

			So(err, ShouldBeNil)
			So(mock.lastReq.Method, ShouldEqual, "POST")
			So(mock.lastReq.URL.Path, ShouldEqual, "/data")
			So(mock.lastReq.Header.Get("Content-Type"), ShouldEqual, "application/json")
		})
	})
}

func TestForwardHeaderTransport_RoundTrip(t *testing.T) {
	PatchConvey("TestForwardHeaderTransport_RoundTrip", t, func() {
		defer otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator())

		PatchConvey("链路关闭时仍注入透传 Header", func() {
			otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
				NewHeaderPropagator([]string{"X-Request-Id"}, nil),
			))

			mock := &mockRoundTripper{}
			transport := &HostAwareTransport{Next: &ForwardHeaderTransport{Next: mock}}

			in := http.Header{}
			in.Set("X-Request-Id", "req-001")
			ctx := otel.GetTextMapPropagator().Extract(context.Background(), propagation.HeaderCarrier(in))

			req, _ := http.NewRequest("GET", "https://api.example.com/users", nil)
			_, err := transport.RoundTrip(req.WithContext(ctx))

			So(err, ShouldBeNil)
			So(mock.lastReq.Header.Get("X-Request-Id"), ShouldEqual, "req-001")
			// 不得改动入参请求
			So(req.Header.Get("X-Request-Id"), ShouldBeEmpty)
		})

		PatchConvey("按域名规则过滤仍然生效", func() {
			otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
				NewHeaderPropagator(nil, []ForwardHeaderRule{
					{Domains: []string{"internal.example.com"}, Headers: []string{"X-Auth-Token"}},
				}),
			))

			in := http.Header{}
			in.Set("X-Auth-Token", "secret")
			ctx := otel.GetTextMapPropagator().Extract(context.Background(), propagation.HeaderCarrier(in))

			outside := &mockRoundTripper{}
			req, _ := http.NewRequest("GET", "https://third-party.example.org/x", nil)
			_, err := (&HostAwareTransport{Next: &ForwardHeaderTransport{Next: outside}}).RoundTrip(req.WithContext(ctx))
			So(err, ShouldBeNil)
			So(outside.lastReq.Header.Get("X-Auth-Token"), ShouldBeEmpty)

			inside := &mockRoundTripper{}
			req2, _ := http.NewRequest("GET", "https://internal.example.com/x", nil)
			_, err = (&HostAwareTransport{Next: &ForwardHeaderTransport{Next: inside}}).RoundTrip(req2.WithContext(ctx))
			So(err, ShouldBeNil)
			So(inside.lastReq.Header.Get("X-Auth-Token"), ShouldEqual, "secret")
		})
	})
}

func TestNextOrDefault(t *testing.T) {
	PatchConvey("TestNextOrDefault", t, func() {
		PatchConvey("Next 为 nil 时回落到 http.DefaultTransport，而非 panic", func() {
			So(nextOrDefault(nil), ShouldEqual, http.DefaultTransport)

			// 走到真实网络之前拦下来，只验证不再 nil panic
			Mock((*http.Transport).RoundTrip).Return(&http.Response{StatusCode: 204}, nil).Build()
			req, _ := http.NewRequest("GET", "http://example.com", nil)
			resp, err := (&HostAwareTransport{}).RoundTrip(req)
			So(err, ShouldBeNil)
			So(resp.StatusCode, ShouldEqual, 204)

			resp2, err := (&ForwardHeaderTransport{}).RoundTrip(req)
			So(err, ShouldBeNil)
			So(resp2.StatusCode, ShouldEqual, 204)
		})

		PatchConvey("Next 非 nil 时原样返回", func() {
			mock := &mockRoundTripper{}
			So(nextOrDefault(mock), ShouldEqual, mock)
		})
	})
}
