// Package tool 定义 agent 可以调用的工具。
//
// 工具的声明（名字、描述、参数 schema）就是 llm.ToolDef，这里不再重复定义一份：
// llm 管「发给模型的是什么」，tool 在它之上加「怎么执行」。依赖是单向的 tool → llm。
package tool

import (
	"context"
	"encoding/json"

	"github.com/xiaoshicae/xone/xagent/llm"
)

// Tool 一个可被模型调用的工具。
type Tool interface {
	// Def 这个工具的声明，会原样发给模型。描述写得越清楚，模型越会用。
	Def() llm.ToolDef

	// Call 执行一次调用。input 是模型给的参数（JSON），不保证合法，要自己校验。
	//
	// 返回的 error 不会中断 agent：它会作为一条错误结果回给模型，
	// 模型通常会据此换个参数重试。所以错误消息要写给模型看：说清楚哪里不对、应该怎么改。
	Call(ctx context.Context, input json.RawMessage) (string, error)
}
