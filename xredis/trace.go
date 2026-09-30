package xredis

import (
	"context"

	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// redisotel v9.22.0 记错误就是 span.RecordError(err) 加 span.SetStatus(codes.Error, err.Error())
// （tracing.go recordError），服务端的原文由此进了 exception.message 和状态描述——
// 原文里带着参数（见 redactedError）。它没有改写错误的选项，这里在它拿到的 Span 外面包一层：
// 两处都换成命令日志里的那份文本。Span 的其余部分（属性、事件、结束）原样交给里面那个。

// redactingTracerProvider 交给 redisotel 的 TracerProvider，发出的 Span 都是 redactingSpan
type redactingTracerProvider struct{ trace.TracerProvider }

func (p redactingTracerProvider) Tracer(name string, opts ...trace.TracerOption) trace.Tracer {
	return redactingTracer{p.TracerProvider.Tracer(name, opts...)}
}

type redactingTracer struct{ trace.Tracer }

// Start ctx 里放的仍是里面那个 Span：之后的钩子（命令日志的 trace_id / span_id）照旧从 ctx 取
func (t redactingTracer) Start(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	ctx, span := t.Tracer.Start(ctx, name, opts...)
	return ctx, &redactingSpan{Span: span}
}

// redactingSpan 记错误时只记 redactedError 给出的文本。
// 一个 Span 只在一个命令的钩子里用，不会并发
type redactingSpan struct {
	trace.Span
	text string // 上一次 RecordError 的错误收过之后的文本，SetStatus 用它
}

// RecordError 原文可以照记的（网络错误、超时）照旧记成事件，其余不记：事件里的 exception.message 就是原文
func (s *redactingSpan) RecordError(err error, opts ...trace.EventOption) {
	s.text, _ = redactedError(err)
	if clientSafe(err) {
		s.Span.RecordError(err, opts...)
	}
}

// SetStatus 出错时的描述换成 RecordError 收过的文本；前面没有 RecordError 的，不知道描述里是什么，一律不记原文
func (s *redactingSpan) SetStatus(code codes.Code, desc string) {
	if code == codes.Error {
		desc = s.text
		if desc == "" {
			desc = "redis error (message omitted, it may contain argument values)"
		}
	}
	s.Span.SetStatus(code, desc)
}
