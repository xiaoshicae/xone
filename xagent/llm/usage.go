package llm

// Usage 一次调用消耗的 token。只出现在 assistant 消息上。
type Usage struct {
	InputTokens  int
	OutputTokens int
}
