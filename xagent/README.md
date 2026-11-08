# xagent

一个最小的 ReAct 循环（核心模块，零第三方依赖，实验性）：调用模型 → 执行工具 → 把结果塞回对话，直到模型不再调用工具。

- 分层参考了 [pi](https://github.com/earendil-works/pi)（pi-ai / pi-agent-core）：`xagent/llm` 是最底层（模型接口、消息、工具声明），`xagent/tool` 在声明之上加执行，`xagent` 是循环；依赖单向 `xagent → tool → llm`
- 不读配置、不登记钩子：`xagent.New` 是纯构造器，不跑 `xone.Run` 也能用
- 目前还没有任何 provider 的适配：`llm.Model` 需要自己实现，或者等后续的适配模块

## 快速上手

```go
type weather struct{}

func (weather) Def() llm.ToolDef {
	return llm.ToolDef{
		Name:        "get_weather",
		Description: "查询某个城市当前的天气",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"city":{"type":"string"}},"required":["city"]}`),
	}
}

func (weather) Call(ctx context.Context, input json.RawMessage) (string, error) {
	var in struct{ City string }
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("input must be {\"city\": string}: %w", err) // 写给模型看：说清楚怎么改
	}
	return in.City + ": 晴，21°C", nil
}

agent, err := xagent.New(model, []tool.Tool{weather{}}, xagent.WithSystem("回答要简短"))
if err != nil {
	return err
}
res, err := agent.Run(ctx, []llm.Message{llm.UserText("柏林今天天气怎么样？")})
if err != nil {
	return err
}
fmt.Println(res.Text())
```

## 三个包

| 包 | 接口 | 职责 |
|---|---|---|
| `xagent/llm` | `Model.Generate(ctx, *Request) (*Response, error)` | 调用一次模型：给一段对话，拿回模型的回复（`Response.Message`）。还定义了 `Message` / `Part` / `ToolCall` 和工具声明 `ToolDef` |
| `xagent/tool` | `Tool.Def() llm.ToolDef`、`Tool.Call(ctx, input) (string, error)` | 一个可执行的工具：声明直接用 `llm.ToolDef`，再加上执行。`Set` 按名字查找 |
| `xagent` | `New(model, tools, opts...)`、`Agent.Run(ctx, messages)` | 循环本身 |
| `xagent/agenterr` | 哨兵错误 `ErrMaxSteps`、`ErrRateLimited` …… | 各层共用的错误原因，用 `errors.Is` 判断；只依赖标准库，provider 适配层也用它 |

## 行为

每一步做三件事：① 调用模型；② 依次执行回复里的每个工具调用；③ 每个结果作为一条 `RoleTool` 消息追加到对话。模型的回复里没有工具调用时结束。

消息有三种角色：`user`、`assistant`、`tool`。assistant 消息上带着这次调用的 `StopReason` 和 `Usage`，tool 消息上带着 `ToolCallID`、`ToolName`、`IsError`，
所以对话历史本身就记录了每一步。各家 provider 的格式不同（例如有的要求把多个结果合进一条 user 消息），由适配层负责转换。

| 情况 | 处理 |
|---|---|
| 一个回复里有多个工具调用 | 按顺序依次执行，每个结果一条 tool 消息，顺序与调用一致，`ToolCallID` 一一对应 |
| 工具返回 error、工具 panic、工具名不存在 | 变成一条 `IsError` 的 tool 消息回给模型，循环继续 |
| 执行工具时 ctx 被取消 | 剩下的工具不再执行，但每个调用仍然有一条错误结果；随后 `Run` 返回 |
| 模型回复被长度上限截断（`StopMaxTokens`） | 报错，这条回复不进对话：其中的工具调用参数可能不完整。它的用量仍计入 `Result.Usage` |
| 调用模型的次数达到上限（默认 10，`WithMaxSteps`） | 报错 |

出错时 `Run` 同时返回 error 和 `Result`。`Result.Messages` 始终是一段合法的对话（每个工具调用都有结果），可以直接作为下一轮的输入。传入的 `messages` 不会被修改。`Result.Usage` 是本次所有模型调用的用量之和。

## 排错

`Run` / `New` 返回的错误都是 `*xerror.Error`，模块名 `xagent`。具体原因用 `errors.Is` 判断，不要匹配错误文本：

| 原因 | 来自 | 怎么处理 |
|---|---|---|
| `agenterr.ErrMaxSteps` | 循环 | 调大 `WithMaxSteps`，或检查工具结果是否让模型陷入了重试 |
| `agenterr.ErrTruncated` | 循环 | 调大 provider 侧的 max tokens |
| `agenterr.ErrRateLimited` | 模型适配层 | 退避后重试 |
| `agenterr.ErrUnavailable` | 模型适配层 | 重试 |
| `agenterr.ErrContextTooLong` | 模型适配层 | 缩短对话历史后再试，直接重试不会好转 |
| `agenterr.ErrAuth` | 模型适配层 | 检查 API key，直接重试不会好转 |
| `context.Canceled` / `context.DeadlineExceeded` | 调用方的 ctx | 取消和超时不另外定义哨兵 |

实现 `llm.Model` 时，把 provider 的错误尽量用 `%w` 归到上面四类模型错误里，归不进去的保持原样。

| 错误文本 | 原因 |
|---|---|
| `xone xagent new failed, err=[model is nil]` | `New` 的第一个参数是 nil |
| `xone xagent new failed, err=[duplicate tool name "x"]` | 两个工具的 `Def().Name` 相同 |
| `xone xagent execute failed, err=[max steps exceeded (N)]` | 模型连续 N 次都在调用工具；调大 `WithMaxSteps`，或检查工具结果是否让模型陷入了重试 |
| `xone xagent execute failed, err=[step N: generate: output truncated by max tokens]` | 模型输出超过了长度上限；调大 provider 侧的 max tokens |
| `xone xagent execute failed, err=[canceled after N steps: context canceled]` | 调用方的 ctx 被取消或超时 |
| `xone xagent execute failed, err=[step N: generate: model returned a nil response]` | `llm.Model` 的实现没报错却返回了 nil，是实现的问题 |
