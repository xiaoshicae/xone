package xagent

import (
	"github.com/xiaoshicae/xone/xagent/llm"
	"github.com/xiaoshicae/xone/xagent/tool"
	"github.com/xiaoshicae/xone/xerror"
)

// Agent 把一个模型和一组工具组合起来。创建后只读，可以并发调用 Run。
type Agent struct {
	model    llm.Model
	tools    *tool.Set
	system   string
	maxSteps int
}

// New 创建 Agent。不阻塞、不碰全局。
func New(model llm.Model, tools []tool.Tool, opts ...Option) (*Agent, error) {
	if model == nil {
		return nil, xerror.Newf("xagent", "new", "model is nil")
	}
	set, err := tool.NewSet(tools...)
	if err != nil {
		return nil, err // 已经是 xagent 的 xerror，不再包一层
	}
	a := &Agent{model: model, tools: set, maxSteps: DefaultMaxSteps}
	for _, opt := range opts {
		opt(a)
	}
	if a.maxSteps <= 0 {
		return nil, xerror.Newf("xagent", "new", "max steps must be positive, got %d", a.maxSteps)
	}
	return a, nil
}
