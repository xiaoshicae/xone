// Package xagent 一个最小的 ReAct 循环：调用模型 → 执行工具 → 把结果塞回对话，直到模型不再调用工具。
//
// 分层与依赖（单向）：
//
//	xagent/llm   模型接口、消息类型、工具声明 ToolDef
//	xagent/tool  可执行的工具：在 ToolDef 之上加 Call       （依赖 llm）
//	xagent       循环                                       （依赖 llm、tool）
package xagent

import (
	"context"
	"fmt"

	"github.com/xiaoshicae/xone/xagent/llm"
	"github.com/xiaoshicae/xone/xagent/tool"
	"github.com/xiaoshicae/xone/xerror"
)

// DefaultMaxSteps 默认最多调用模型的次数。防止模型反复调用工具停不下来。
const DefaultMaxSteps = 10

// Agent 把一个模型和一组工具组合起来。创建后只读，可以并发调用 Run。
type Agent struct {
	model    llm.Model
	tools    *tool.Set
	system   string
	maxSteps int
}

// Option 配置 Agent。
type Option func(*Agent)

// WithSystem 设置系统提示词。
func WithSystem(prompt string) Option {
	return func(a *Agent) { a.system = prompt }
}

// WithMaxSteps 设置最多调用模型的次数，默认 DefaultMaxSteps。
func WithMaxSteps(n int) Option {
	return func(a *Agent) { a.maxSteps = n }
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

// Result 一次 Run 的结果。
type Result struct {
	// Messages 完整对话：传入的历史 + 本次新增的消息。
	// 无论 Run 成功与否，它都是一段合法的对话（每个工具调用都有结果），可以直接作为下一轮的输入。
	Messages []llm.Message
	// Steps 本次调用了几次模型。
	Steps int
	// Usage 本次所有模型调用的用量之和。每一次的明细在对应 assistant 消息的 Usage 上。
	Usage llm.Usage
}

// Text 最后一条 assistant 消息的文本，通常就是最终回答。
func (r *Result) Text() string {
	for i := len(r.Messages) - 1; i >= 0; i-- {
		if r.Messages[i].Role == llm.RoleAssistant {
			return r.Messages[i].Text()
		}
	}
	return ""
}

// Run 从给定的对话开始跑 ReAct 循环，直到模型不再调用工具。
//
// 出错时同时返回 error 和已经产生的 Result，中间状态不丢。传入的 messages 不会被修改。
func (a *Agent) Run(ctx context.Context, messages []llm.Message) (*Result, error) {
	// 复制一份：后面的 append 不能写到调用方的底层数组里
	res := &Result{Messages: append([]llm.Message(nil), messages...)}

	for res.Steps < a.maxSteps {
		if err := ctx.Err(); err != nil {
			return res, xerror.Newf("xagent", "execute", "canceled after %d steps: %w", res.Steps, err)
		}

		// ① 调用模型
		reply, err := a.generate(ctx, res.Messages)
		res.Steps++
		res.Usage.InputTokens += reply.Usage.InputTokens
		res.Usage.OutputTokens += reply.Usage.OutputTokens
		if err != nil {
			return res, xerror.Newf("xagent", "execute", "step %d: %w", res.Steps, err)
		}
		// 先把模型的回复记进历史，再执行工具：工具结果必须跟在发起调用的那条消息后面
		res.Messages = append(res.Messages, reply)

		calls := reply.ToolCalls()
		if len(calls) == 0 {
			return res, nil // 模型不再调用工具：这就是最终回答
		}

		// ② 执行工具
		results := a.callTools(ctx, calls)

		// ③ 把结果塞回对话：每个结果一条 tool 消息
		res.Messages = append(res.Messages, results...)
	}
	return res, xerror.Newf("xagent", "execute", "exceeded max steps %d", a.maxSteps)
}

// generate 第①步：调用一次模型。出错时仍返回模型给出的消息，用量照样计入。
func (a *Agent) generate(ctx context.Context, messages []llm.Message) (llm.Message, error) {
	reply, err := a.model.Generate(ctx, &llm.Request{System: a.system, Messages: messages, Tools: a.tools.Defs()})
	if err != nil {
		return reply, fmt.Errorf("generate: %w", err)
	}
	// 被截断的回复不进历史：里面的工具调用参数可能只写了一半，
	// 而且一旦带着没有结果的工具调用进了历史，这段对话就再也发不出去了
	if reply.StopReason == llm.StopMaxTokens {
		return reply, fmt.Errorf("generate: output truncated by max tokens")
	}
	reply.Role = llm.RoleAssistant // 模型产出的一定是 assistant 消息，适配层漏填也不影响历史
	return reply, nil
}

// callTools 第②步：按顺序执行每个工具调用，每个调用产出一条 tool 消息。
//
// 保证每个调用都有一个结果，顺序与 calls 一致：模型要求每个工具调用都有对应的结果，
// 缺一个，下一轮请求就会被 provider 拒绝。所以任何失败都变成一条错误结果，不提前返回。
func (a *Agent) callTools(ctx context.Context, calls []llm.ToolCall) []llm.Message {
	results := make([]llm.Message, len(calls))
	for i, c := range calls {
		content, err := a.callTool(ctx, c)
		if err != nil {
			results[i] = llm.ToolResult(c, err.Error(), true)
		} else {
			results[i] = llm.ToolResult(c, content, false)
		}
	}
	return results
}

// callTool 执行一个工具调用。工具找不到、ctx 已取消、工具出错或 panic，都作为 error 返回。
func (a *Agent) callTool(ctx context.Context, c llm.ToolCall) (content string, err error) {
	// 已经取消了就不再执行，但仍然返回一条结果，保证对话合法
	if err := ctx.Err(); err != nil {
		return "", fmt.Errorf("tool call skipped: %w", err)
	}
	t, ok := a.tools.Get(c.Name)
	if !ok {
		return "", fmt.Errorf("unknown tool %q", c.Name)
	}
	// 一个工具 panic 不该带垮整个 agent：转成错误结果，让模型知道这个工具出了问题
	defer func() {
		if r := recover(); r != nil {
			content, err = "", fmt.Errorf("tool %q panicked: %v", c.Name, r)
		}
	}()
	return t.Call(ctx, c.Input)
}
