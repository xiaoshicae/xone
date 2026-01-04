package xhttp

import (
	"context"

	"github.com/go-resty/resty/v2"
)

var defaultClient = resty.New()

// C 获取http client，推荐直接使用 RWithCtx()，保证ctx中内容能传递到下游(trace等)
func C() *resty.Client {
	return defaultClient
}

// RWithCtx 可以保证ctx中内容能传递到下游(trace等)
func RWithCtx(ctx context.Context) *resty.Request {
	return C().R().SetContext(ctx)
}

func setDefaultClient(client *resty.Client) {
	defaultClient = client
}
