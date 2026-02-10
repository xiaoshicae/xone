package xhttp

import (
	"context"
	"net/http"
	"sync"

	"github.com/go-resty/resty/v2"
	"github.com/xiaoshicae/xone/v2/xutil"
)

var (
	defaultClient = resty.New()
	rawHttpClient *http.Client
	clientMu      sync.RWMutex
)

// C 获取 resty client，推荐直接使用 RWithCtx()，保证 ctx 中内容能传递到下游(trace等)
func C() *resty.Client {
	clientMu.RLock()
	defer clientMu.RUnlock()
	return defaultClient
}

// RWithCtx 可以保证 ctx 中内容能传递到下游(trace等)
func RWithCtx(ctx context.Context) *resty.Request {
	return C().R().SetContext(ctx)
}

// RawClient 获取原生 http.Client，用于需要直接操作 response body 的场景（如 SSE 流式请求）
// 注意：必须在 xone 启动后调用，否则返回的 client 未经配置（无超时等）
func RawClient() *http.Client {
	clientMu.RLock()
	defer clientMu.RUnlock()
	if rawHttpClient != nil {
		return rawHttpClient
	}
	// 兜底返回，实际使用中应确保 xone 已启动
	xutil.WarnIfEnableDebug("XHttp RawClient called before xone started, returning http.DefaultClient without custom config")
	return http.DefaultClient
}

func setDefaultClient(client *resty.Client) {
	clientMu.Lock()
	defer clientMu.Unlock()
	defaultClient = client
}

func setRawHttpClient(client *http.Client) {
	clientMu.Lock()
	defer clientMu.Unlock()
	rawHttpClient = client
}
