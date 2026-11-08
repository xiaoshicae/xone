// Package llm 定义「调用一次大模型」的接口、与 provider 无关的消息类型，以及发给模型的工具声明。
//
// 这是 xagent 最底下的一层，不依赖 xagent 的其他包：tool 包依赖它（工具的声明就是 ToolDef），
// xagent 依赖它和 tool。
//
// 各家 provider（Anthropic、OpenAI……）的适配各自实现 Model，
// 在适配层把这里的 Message 与它们自己的请求 / 响应格式互相转换。
package llm
