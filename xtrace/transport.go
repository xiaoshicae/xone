package xtrace

import "net/http"

// HostAwareTransport 在 RoundTrip 时将目标 Host 存入 context，
// 使 HeaderPropagator 能按域名过滤透传 Header
type HostAwareTransport struct {
	// Next 是实际执行请求的 RoundTripper（通常是 otelhttp.Transport）
	Next http.RoundTripper
}

// RoundTrip 实现 http.RoundTripper 接口
func (t *HostAwareTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	next := t.Next
	if next == nil {
		next = http.DefaultTransport
	}
	ctx := WithTargetHost(req.Context(), req.URL.Host)
	return next.RoundTrip(req.WithContext(ctx))
}
