package xagent

import (
	"github.com/xiaoshicae/xone/xagent/llm"
)

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
