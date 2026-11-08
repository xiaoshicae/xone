package llm

// StopReason 模型为什么停下来。只出现在 assistant 消息上。
type StopReason string

const (
	StopEndTurn   StopReason = "end_turn"   // 正常说完
	StopToolUse   StopReason = "tool_use"   // 要调用工具，等结果
	StopMaxTokens StopReason = "max_tokens" // 输出被长度上限截断，内容可能不完整
)
