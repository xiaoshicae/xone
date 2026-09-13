package xhttp

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/xiaoshicae/xone/v2/xutil"
)

// fallbackTimeout 未初始化 / 已关闭时兜底 client 的超时
//
// 零值超时是「永不超时」而不是「有个默认值」：对端不响应时请求会一直挂着。
// 最难受的是关闭阶段——BeforeStop hook 里发一个这样的请求，
// 整个进程的退出流程就被卡死了
const fallbackTimeout = 30 * time.Second

var (
	defaultClient = newFallbackRestyClient()
	rawHttpClient *http.Client
	clientMu      sync.RWMutex

	// fallbackHttpClient 未初始化 / 已关闭时返回的原生 client，带兜底超时
	fallbackHttpClient = &http.Client{Timeout: fallbackTimeout}
)

// newFallbackRestyClient 构造带兜底超时的 resty client
// 供包初始化及 xone 启动前的调用使用
func newFallbackRestyClient() *resty.Client {
	return resty.New().SetTimeout(fallbackTimeout)
}

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
//
// 必须在 xone 启动后调用。启动前或关闭后返回一个带兜底超时的 client，
// 而不是 http.DefaultClient——后者的超时是 0，请求可以永久挂起
func RawClient() *http.Client {
	clientMu.RLock()
	defer clientMu.RUnlock()
	if rawHttpClient != nil {
		return rawHttpClient
	}
	xutil.WarnIfEnableDebug("XHttp RawClient called before xone started or after shutdown, returning fallback client with %v timeout", fallbackTimeout)
	return fallbackHttpClient
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

// swapRawHttpClient 替换原生 client 并返回被替换掉的那个，供调用方释放其连接
func swapRawHttpClient(client *http.Client) *http.Client {
	clientMu.Lock()
	defer clientMu.Unlock()
	old := rawHttpClient
	rawHttpClient = client
	return old
}
