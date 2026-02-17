## XPipeline 模块

### 1. 模块简介

XPipeline 是 XOne 框架的流式编排模块，提供：
- 将多个 Processor 串联成 channel 链，每个 Processor 在独立 goroutine 中并发运行
- 通过 Frame 接口传递异构数据，支持自定义业务帧和内置控制帧
- Context 取消支持，适用于需要即时中断的场景（如用户打断、超时）
- Process 中的 panic 自动捕获，channel 正常关闭不会挂起
- 可选 Monitor 监控，零开销关闭

### 2. 核心概念

| 概念 | 说明 |
|------|------|
| `Frame` | 数据单元接口，Pipeline 中所有数据通过 Frame 传递 |
| `Processor` | 处理器接口，定义 `Name` 和 `Process` 方法 |
| `Pipeline` | 流式编排器，将 Processor 串联成 channel 链 |
| `RunResult` | 运行结果，收集所有处理器的错误信息 |
| `Monitor` | 监控接口，观测各处理器和 Pipeline 整体的执行情况 |

### 3. 执行模型

```
inputCh → [Processor1] → ch[0] → [Processor2] → ch[1] → ... → [ProcessorN] → outputCh
            (goroutine)            (goroutine)                   (goroutine)
```

- 每个 Processor 运行在独立 goroutine 中
- Processor 间通过带缓冲的 channel 连接（默认缓冲 64，可配置）
- 上游 channel 关闭时，下游 Processor 的 `input` 循环自动结束
- Context 取消时，Processor 应检查 `ctx.Done()` 并尽快退出

### 4. 使用示例

#### 定义业务 Frame

```go
// 用户自定义的业务帧
type TextChunkFrame struct {
    Text       string
    ChunkIndex int
}

func (f *TextChunkFrame) FrameType() string { return "text_chunk" }

type AudioFrame struct {
    Data       []byte
    SampleRate int
}

func (f *AudioFrame) FrameType() string { return "audio" }
```

#### 定义处理器

```go
// TextProcessor 文本处理器
type TextProcessor struct{}

func (p *TextProcessor) Name() string { return "text-processor" }

func (p *TextProcessor) Process(ctx context.Context, input <-chan xpipeline.Frame, output chan<- xpipeline.Frame) error {
    for frame := range input {
        select {
        case <-ctx.Done():
            return ctx.Err()
        default:
        }

        switch f := frame.(type) {
        case *xpipeline.StartFrame:
            // 处理启动信号
            output <- f
        case *TextChunkFrame:
            // 业务处理逻辑
            processed := &TextChunkFrame{
                Text:       strings.ToUpper(f.Text),
                ChunkIndex: f.ChunkIndex,
            }
            output <- processed
        default:
            output <- frame // 透传未识别的帧
        }
    }
    return nil
}
```

#### 构建并运行 Pipeline

```go
func main() {
    pipe := xpipeline.New("data-pipeline",
        &TextProcessor{},
        &AudioProcessor{},
        &EncoderProcessor{},
    )

    inputCh := make(chan xpipeline.Frame, 1)
    inputCh <- &xpipeline.StartFrame{Context: myContext}
    close(inputCh)

    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    outputCh := pipe.Run(ctx, inputCh)
    for frame := range outputCh {
        switch f := frame.(type) {
        case *AudioFrame:
            // 处理音频输出
        case *xpipeline.ErrorFrame:
            log.Printf("错误: %s", f.Message)
        }
    }
}
```

#### 监控

监控默认开启，使用 xlog 打印日志。可通过 YAML 配置禁用：

```yaml
XPipeline:
  BufferSize: 128
  DisableMonitor: true
```

自定义全局 Monitor 实现：

```go
xpipeline.SetDefaultMonitor(myMonitor)
```

### 5. 内置控制帧

| 帧类型 | 说明 | 字段 |
|--------|------|------|
| `StartFrame` | 启动信号 | `Context any` — 携带任意上下文 |
| `EndFrame` | 结束信号 | （无） |
| `ErrorFrame` | 错误信号 | `Err error`, `Message string` |
| `MetadataFrame` | 元数据传递 | `Key string`, `Value any` |

业务帧由使用方自定义，只需实现 `Frame` 接口（`FrameType() string`）。

### 6. 处理器编写规范

```go
func (p *MyProcessor) Process(ctx context.Context, input <-chan xpipeline.Frame, output chan<- xpipeline.Frame) error {
    for frame := range input {
        // 1. 检查 Context 取消
        select {
        case <-ctx.Done():
            return ctx.Err()
        default:
        }

        // 2. 按帧类型处理
        switch f := frame.(type) {
        case *MyFrame:
            // 业务逻辑
            output <- transformedFrame
        default:
            output <- frame // 透传未识别的帧
        }
    }
    return nil
}
```

- 始终检查 `ctx.Done()` 以支持即时中断
- 未识别的帧应透传给下游（default 分支）
- 错误可通过返回 error 或发送 `ErrorFrame` 两种方式传递
- Process 中的 panic 会被自动捕获，不会导致 Pipeline 挂起

### 7. xflow vs xpipeline

| 维度 | xflow | xpipeline |
|------|-------|-----------|
| 执行模型 | 同步顺序执行 | 并发 goroutine + channel |
| 数据流 | Req → Resp（单链路） | Frame 流式传递（异构多帧） |
| 回滚 | 自动逆序回滚 | 无（错误通过 ErrorFrame 传播） |
| 适用场景 | 事务型编排（支付、订单） | 流式处理（音频、ETL、实时数据） |

### 8. 注意事项

- `Process` 中的 panic 会被自动捕获，转为错误记录在 `RunResult` 中
- `Monitor` 的回调从 goroutine 中调用，自定义实现必须并发安全
- `Monitor` 默认开启（使用 xlog 打印），可通过配置 `DisableMonitor: true` 关闭（零开销）
- `Pipeline` 的字段赋值非并发安全，必须在 `Run` 前完成
- channel 缓冲大小通过 `XPipeline.BufferSize` 配置，默认 64
