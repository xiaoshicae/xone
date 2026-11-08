package xagent

import (
	"context"
	"fmt"

	"github.com/xiaoshicae/xone/xagent/agenterr"
	"github.com/xiaoshicae/xone/xagent/llm"
	"github.com/xiaoshicae/xone/xerror"
)

// Run 从给定的对话开始跑 ReAct 循环，直到模型不再调用工具。
//
// 出错时同时返回 error 和已经产生的 Result，中间状态不丢。传入的 messages 不会被修改。
// 错误是 *xerror.Error，原因用 errors.Is 判断：agenterr 里的哨兵，或者 context.Canceled / DeadlineExceeded。
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
	return res, xerror.Newf("xagent", "execute", "%w (%d)", agenterr.ErrMaxSteps, a.maxSteps)
}

// generate 第①步：调用一次模型。出错时仍返回模型给出的消息，用量照样计入。
func (a *Agent) generate(ctx context.Context, messages []llm.Message) (llm.Message, error) {
	resp, err := a.model.Generate(ctx, &llm.Request{System: a.system, Messages: messages, Tools: a.tools.Defs()})
	if err != nil {
		return llm.Message{}, fmt.Errorf("generate: %w", err)
	}
	if resp == nil {
		return llm.Message{}, fmt.Errorf("generate: model returned a nil response")
	}
	reply := resp.Message
	// 被截断的回复不进历史：里面的工具调用参数可能只写了一半，
	// 而且一旦带着没有结果的工具调用进了历史，这段对话就再也发不出去了
	if reply.StopReason == llm.StopMaxTokens {
		return reply, fmt.Errorf("generate: %w", agenterr.ErrTruncated)
	}
	reply.Role = llm.RoleAssistant // 模型产出的一定是 assistant 消息，适配层漏填也不影响历史
	return reply, nil
}
