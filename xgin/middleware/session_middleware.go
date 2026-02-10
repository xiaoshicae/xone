package middleware

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/xiaoshicae/xone/xlog"
)

// GinXSessionMiddleware session中间件
// 提前注入一些请求上下文(log上下文容器等，保证日志kv tag能从一开始就初始化好，后续能在整个请求带下去)
func GinXSessionMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		ctx = ctxWithKV(ctx, make(map[string]interface{}))
		c.Request = c.Request.WithContext(ctx)

		c.Next() // 继续处理
	}
}

func ctxWithKV(ctx context.Context, kvs map[string]interface{}) context.Context {
	if kvs == nil {
		kvs = make(map[string]interface{})
	}
	kvContainer, ok := ctx.Value(xlog.XLogCtxKVContainerKey).(map[string]interface{})
	if !ok || kvContainer == nil {
		return context.WithValue(ctx, xlog.XLogCtxKVContainerKey, kvs)
	}
	for k, v := range kvs {
		kvContainer[k] = v
	}
	return ctx
}
