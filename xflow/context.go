package xflow

import "context"

// FlowContext 流程上下文，包装标准 context 并携带泛型数据
type FlowContext[T any] struct {
	context.Context
	Data T
}

// NewFlowContext 创建流程上下文
func NewFlowContext[T any](ctx context.Context, data T) *FlowContext[T] {
	if ctx == nil {
		ctx = context.Background()
	}
	return &FlowContext[T]{
		Context: ctx,
		Data:    data,
	}
}
