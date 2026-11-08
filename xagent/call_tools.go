package xagent

import (
	"context"
	"fmt"

	"github.com/xiaoshicae/xone/xagent/llm"
)

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
