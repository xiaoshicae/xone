package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/xiaoshicae/xone/v2/xlog"
)

// Session session中间件
// 提前注入一些请求上下文(log上下文容器等，保证日志kv tag能从一开始就初始化好，后续能在整个请求带下去)
func Session() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		ctx = xlog.CtxWithKV(ctx, nil)
		c.Request = c.Request.WithContext(ctx)

		c.Next() // 继续处理
	}
}
