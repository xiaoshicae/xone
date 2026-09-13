package xtrace

import (
	"net/http"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

// HostAwareTransport 在 RoundTrip 时将目标 Host 存入 context，
// 使 HeaderPropagator 能按域名过滤透传 Header
type HostAwareTransport struct {
	// Next 是实际执行请求的 RoundTripper（通常是 otelhttp.Transport）
	// 为 nil 时回落到 http.DefaultTransport
	Next http.RoundTripper
}

// RoundTrip 实现 http.RoundTripper 接口
func (t *HostAwareTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	ctx := WithTargetHost(req.Context(), req.URL.Host)
	return nextOrDefault(t.Next).RoundTrip(req.WithContext(ctx))
}

// ForwardHeaderTransport 在发出请求前按全局 Propagator 注入透传 Header
//
// 链路开启时 otelhttp.Transport 已经做了这件事，无需再包一层；
// 本 Transport 用于链路关闭但仍配置了 Header 透传的场景。
type ForwardHeaderTransport struct {
	// Next 是实际执行请求的 RoundTripper，为 nil 时回落到 http.DefaultTransport
	Next http.RoundTripper
}

// RoundTrip 实现 http.RoundTripper 接口
//
// 按 RoundTripper 约定不修改入参请求，注入前先克隆一份。
func (t *ForwardHeaderTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	cloned := req.Clone(req.Context())
	otel.GetTextMapPropagator().Inject(cloned.Context(), propagation.HeaderCarrier(cloned.Header))
	return nextOrDefault(t.Next).RoundTrip(cloned)
}

func nextOrDefault(next http.RoundTripper) http.RoundTripper {
	if next == nil {
		return http.DefaultTransport
	}
	return next
}
