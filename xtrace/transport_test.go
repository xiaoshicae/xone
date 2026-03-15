package xtrace

import (
	"net/http"
	"testing"

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

		PatchConvey("NextNilFallsBackToDefault", func() {
			// Next 为 nil 时使用 http.DefaultTransport，不 panic
			transport := &HostAwareTransport{Next: nil}
			req, _ := http.NewRequest("GET", "https://example.com/test", nil)
			// 由于没有真正的服务器，会返回网络错误，但不应 panic
			So(func() { transport.RoundTrip(req) }, ShouldNotPanic)
		})
	})
}
