package middleware

import (
	"fmt"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	semconv "go.opentelemetry.io/otel/semconv/v1.20.0"
	"go.opentelemetry.io/otel/semconv/v1.20.0/httpconv"
	"go.opentelemetry.io/otel/trace"
)

const (
	tracerName    = "github.com/xiaoshicae/xone/xgin"
	traceIdHeader = "X-Trace-Id"
)

func GinXTraceMiddleware() gin.HandlerFunc {
	propagator := otel.GetTextMapPropagator()
	return func(c *gin.Context) {
		fullPath := c.FullPath()
		// 当请求路径未匹配任何路由时，FullPath() 返回空字符串，使用原始路径作为回退
		if fullPath == "" {
			fullPath = c.Request.URL.Path
		}

		ctx := c.Request.Context()
		ctx = propagator.Extract(ctx, propagation.HeaderCarrier(c.Request.Header))
		opts := []trace.SpanStartOption{
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(semconv.HTTPRoute(fullPath)),
		}

		spanName := fmt.Sprintf("%v %v", c.Request.Method, fullPath)

		ctx, span := otel.Tracer(tracerName).Start(ctx, spanName, opts...)
		defer span.End()

		// pass the span through the request context
		c.Request = c.Request.WithContext(ctx)

		// 设置 trace id header（必须在 c.Next() 之前，否则 response body 已发送，header 无法写入）
		if span.SpanContext().IsValid() {
			c.Header(traceIdHeader, span.SpanContext().TraceID().String())
		}

		// serve the request to the next middleware
		c.Next()

		status := c.Writer.Status()
		span.SetStatus(httpconv.ServerStatus(status))
		if status > 0 {
			span.SetAttributes(semconv.HTTPStatusCode(status))
		}
		if len(c.Errors) > 0 {
			span.SetAttributes(attribute.String("gin.errors", c.Errors.String()))
		}
	}
}
