// Package xagent 一个最小的 ReAct 循环：调用模型 → 执行工具 → 把结果塞回对话，直到模型不再调用工具。
//
// 分层与依赖（单向）：
//
//	xagent/llm   模型接口、消息类型、工具声明 ToolDef
//	xagent/tool  可执行的工具：在 ToolDef 之上加 Call       （依赖 llm）
//	xagent       循环                                       （依赖 llm、tool）
package xagent
