package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/xiaoshicae/xone/v2/xmetric"
)

// MetricsHandler 返回 Prometheus /metrics 端点的 Gin handler
// 运行时调用以确保 xmetric 已初始化
func MetricsHandler() gin.HandlerFunc {
	return gin.WrapH(xmetric.Handler())
}
