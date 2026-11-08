package llm

import (
	"encoding/json"
)

// ToolDef 工具的声明：告诉模型「有这么一个工具可以用」。只描述，不执行。
//
// 可执行的工具在 tool 包，它通过 Def() 返回这份声明。
type ToolDef struct {
	Name        string          // 模型用它来指定工具
	Description string          // 这个工具做什么、什么时候该用
	InputSchema json.RawMessage // 参数的 JSON Schema
}
