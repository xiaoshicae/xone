package middleware

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/xiaoshicae/xone/v2/xlog"
)

// GinXSessionMiddleware session中间件
// 提前注入一些请求上下文(log上下文容器等，保证日志kv tag能从一开始就初始化好，后续能在整个请求带下去)
func GinXSessionMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		ctx = ctxWithKV(ctx, make(map[string]any))
		c.Request = c.Request.WithContext(ctx)

		c.Next() // 继续处理
	}
}

func ctxWithKV(ctx context.Context, kvs map[string]any) context.Context {
	if kvs == nil {
		kvs = make(map[string]any)
	}
	kvContainer, ok := ctx.Value(xlog.XLogCtxKVContainerKey).(map[string]any)
	if !ok || kvContainer == nil {
		// 创建副本避免外部修改影响
		newKvs := make(map[string]any, len(kvs))
		for k, v := range kvs {
			newKvs[k] = v
		}
		return context.WithValue(ctx, xlog.XLogCtxKVContainerKey, newKvs)
	}
	// 合并已有的和新的 kv，创建新 map 保证并发安全（与 xlog.CtxWithKV 行为一致）
	newContainer := make(map[string]any, len(kvContainer)+len(kvs))
	for k, v := range kvContainer {
		newContainer[k] = v
	}
	for k, v := range kvs {
		newContainer[k] = v
	}
	return context.WithValue(ctx, xlog.XLogCtxKVContainerKey, newContainer)
}
