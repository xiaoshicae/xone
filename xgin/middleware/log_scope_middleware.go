package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/xiaoshicae/xone/v2/xlog"
)

// LogScope 请求入口中间件，为本次请求开启日志 KV 作用域
//
// 装上作用域之后，业务代码在任意调用层级都可以用 xlog.AddKV(ctx, ...) 补充字段，
// 无需把新 context 逐层回传——调用栈深处拿不到 *gin.Context，本来也没机会回传。
// 写入对整条请求可见，因此 Log 中间件在 c.Next() 之后打的访问日志也会带上这些字段。
//
// 必须排在所有中间件最前面：在它之后才有作用域可写。
func LogScope() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := xlog.CtxWithKVScope(c.Request.Context())
		c.Request = c.Request.WithContext(ctx)

		c.Next() // 继续处理
	}
}
