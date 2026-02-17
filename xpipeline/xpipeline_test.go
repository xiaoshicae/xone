package xpipeline

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	. "github.com/bytedance/mockey"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/xiaoshicae/xone/v2/xconfig"
)

// ==================== 测试用 Processor 实现 ====================

type mockProcessor struct {
	name      string
	processFn func(ctx context.Context, input <-chan Frame, output chan<- Frame) error
}

func (m *mockProcessor) Name() string { return m.name }

func (m *mockProcessor) Process(ctx context.Context, input <-chan Frame, output chan<- Frame) error {
	if m.processFn != nil {
		return m.processFn(ctx, input, output)
	}
	// 默认行为：透传所有帧
	for frame := range input {
		select {
		case output <- frame:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

// ==================== 测试用 Monitor ====================

type monitorCall struct {
	method        string // "OnProcessorDone" / "OnPipelineDone"
	pipelineName  string
	processorName string
	err           error
	duration      time.Duration
	result        ResultSummary
}

type testMonitor struct {
	mu    sync.Mutex
	calls []monitorCall
	done  chan struct{} // OnPipelineDone 触发后关闭
}

func newTestMonitor() *testMonitor {
	return &testMonitor{done: make(chan struct{})}
}

func (m *testMonitor) OnProcessorDone(_ context.Context, e *StepEvent) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, monitorCall{
		method:        "OnProcessorDone",
		pipelineName:  e.PipelineName,
		processorName: e.ProcessorName,
		err:           e.Err,
		duration:      e.Duration,
	})
}

func (m *testMonitor) OnPipelineDone(_ context.Context, e *PipelineEvent) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, monitorCall{
		method:       "OnPipelineDone",
		pipelineName: e.PipelineName,
		result:       e.Result,
		duration:     e.Duration,
	})
	close(m.done)
}

// waitDone 等待 OnPipelineDone 被调用
func (m *testMonitor) waitDone(timeout time.Duration) bool {
	select {
	case <-m.done:
		return true
	case <-time.After(timeout):
		return false
	}
}

func (m *testMonitor) processorDoneCalls() []monitorCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []monitorCall
	for _, c := range m.calls {
		if c.method == "OnProcessorDone" {
			result = append(result, c)
		}
	}
	return result
}

func (m *testMonitor) pipelineDoneCalls() []monitorCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []monitorCall
	for _, c := range m.calls {
		if c.method == "OnPipelineDone" {
			result = append(result, c)
		}
	}
	return result
}

// ==================== 测试用 Frame ====================

type textFrame struct {
	Text string
}

func (f *textFrame) FrameType() string { return "text" }

// ==================== Frame 测试 ====================

func TestFrame_FrameType(t *testing.T) {
	PatchConvey("TestFrame_FrameType", t, func() {
		So((&StartFrame{}).FrameType(), ShouldEqual, "start")
		So((&EndFrame{}).FrameType(), ShouldEqual, "end")
		So((&ErrorFrame{}).FrameType(), ShouldEqual, "error")
		So((&MetadataFrame{}).FrameType(), ShouldEqual, "metadata")
	})
}

// ==================== StepError 测试 ====================

func TestStepError(t *testing.T) {
	PatchConvey("TestStepError", t, func() {
		PatchConvey("Error 格式化", func() {
			se := &StepError{
				ProcessorName: "tts",
				Err:           errors.New("synthesis failed"),
			}
			So(se.Error(), ShouldContainSubstring, "tts")
			So(se.Error(), ShouldContainSubstring, "synthesis failed")
		})

		PatchConvey("Unwrap", func() {
			original := errors.New("original error")
			se := &StepError{
				ProcessorName: "test",
				Err:           original,
			}
			So(se.Unwrap(), ShouldEqual, original)
			So(errors.Is(se, original), ShouldBeTrue)
		})
	})
}

// ==================== RunResult 测试 ====================

func TestRunResult(t *testing.T) {
	PatchConvey("TestRunResult", t, func() {
		PatchConvey("Success", func() {
			r := &RunResult{}
			So(r.Success(), ShouldBeTrue)
			So(r.HasErrors(), ShouldBeFalse)
			So(r.String(), ShouldBeEmpty)
		})

		PatchConvey("HasErrors", func() {
			r := &RunResult{
				Errors: []*StepError{{ProcessorName: "p1", Err: errors.New("fail")}},
			}
			So(r.Success(), ShouldBeFalse)
			So(r.HasErrors(), ShouldBeTrue)
			So(r.String(), ShouldContainSubstring, "pipeline failed")
			So(r.String(), ShouldContainSubstring, "errors=[1]")
		})

		PatchConvey("ResultSummary 接口兼容", func() {
			var summary ResultSummary = &RunResult{}
			So(summary.Success(), ShouldBeTrue)
		})
	})
}

// ==================== Config 测试 ====================

func TestGetConfig(t *testing.T) {
	PatchConvey("TestGetConfig", t, func() {
		PatchConvey("UnmarshalConfig 成功", func() {
			MockValue(&cachedConfigOnce).To(sync.Once{})
			MockValue(&cachedConfig).To((*Config)(nil))
			c := GetConfig()
			So(c, ShouldNotBeNil)
			So(c.BufferSize, ShouldEqual, defaultBufferSize)
			So(c.DisableMonitor, ShouldBeFalse)
		})

		PatchConvey("UnmarshalConfig 失败返回默认值", func() {
			MockValue(&cachedConfigOnce).To(sync.Once{})
			MockValue(&cachedConfig).To((*Config)(nil))
			Mock(xconfig.UnmarshalConfig).To(func(key string, conf any) error {
				return errors.New("unmarshal failed")
			}).Build()
			c := GetConfig()
			So(c, ShouldNotBeNil)
			So(c.BufferSize, ShouldEqual, defaultBufferSize)
			So(c.DisableMonitor, ShouldBeFalse)
		})
	})
}

func TestConfigMergeDefault(t *testing.T) {
	PatchConvey("TestConfigMergeDefault", t, func() {
		PatchConvey("nil 输入", func() {
			c := configMergeDefault(nil)
			So(c, ShouldNotBeNil)
			So(c.BufferSize, ShouldEqual, defaultBufferSize)
			So(c.DisableMonitor, ShouldBeFalse)
		})

		PatchConvey("自定义 BufferSize", func() {
			input := &Config{BufferSize: 128}
			c := configMergeDefault(input)
			So(c.BufferSize, ShouldEqual, 128)
		})

		PatchConvey("BufferSize 为 0 使用默认值", func() {
			input := &Config{BufferSize: 0}
			c := configMergeDefault(input)
			So(c.BufferSize, ShouldEqual, defaultBufferSize)
		})
	})
}

// ==================== Monitor 测试 ====================

func TestDefaultMonitor(t *testing.T) {
	PatchConvey("TestDefaultMonitor-不 panic", t, func() {
		m := &defaultMonitor{}
		ctx := context.Background()

		se := &StepEvent{PipelineName: "pipe", ProcessorName: "proc", Duration: time.Millisecond}
		seErr := &StepEvent{PipelineName: "pipe", ProcessorName: "proc", Err: errors.New("err"), Duration: time.Millisecond}
		pe := &PipelineEvent{PipelineName: "pipe", Result: &RunResult{}, Duration: time.Millisecond}
		peErr := &PipelineEvent{PipelineName: "pipe", Result: &RunResult{Errors: []*StepError{{ProcessorName: "p", Err: errors.New("fail")}}}, Duration: time.Millisecond}

		So(func() { m.OnProcessorDone(ctx, se) }, ShouldNotPanic)
		So(func() { m.OnProcessorDone(ctx, seErr) }, ShouldNotPanic)
		So(func() { m.OnPipelineDone(ctx, pe) }, ShouldNotPanic)
		So(func() { m.OnPipelineDone(ctx, peErr) }, ShouldNotPanic)
	})
}

func TestSetDefaultMonitor(t *testing.T) {
	PatchConvey("TestSetDefaultMonitor-替换全局默认 Monitor", t, func() {
		original := GetDefaultMonitor()
		So(original, ShouldNotBeNil)

		custom := newTestMonitor()
		SetDefaultMonitor(custom)
		So(GetDefaultMonitor(), ShouldEqual, custom)

		// 恢复
		SetDefaultMonitor(original)
		So(GetDefaultMonitor(), ShouldEqual, original)
	})
}

// ==================== New 函数测试 ====================

func TestNew(t *testing.T) {
	PatchConvey("TestNew-函数式构建", t, func() {
		p1 := &mockProcessor{name: "p1"}
		p2 := &mockProcessor{name: "p2"}
		pipe := New("test-pipe", p1, p2)

		So(pipe.Name, ShouldEqual, "test-pipe")
		So(len(pipe.Processors), ShouldEqual, 2)
	})
}

// ==================== Pipeline.Run 测试 ====================

func TestPipeline_Run(t *testing.T) {
	PatchConvey("TestPipeline_Run", t, func() {
		Mock(GetConfig).Return(&Config{BufferSize: 64, DisableMonitor: true}).Build()

		PatchConvey("空流程", func() {
			pipe := New("empty")
			inputCh := make(chan Frame)
			close(inputCh)
			outputCh := pipe.Run(context.Background(), inputCh)

			// 输出 channel 应已关闭
			_, ok := <-outputCh
			So(ok, ShouldBeFalse)
		})

		PatchConvey("单处理器透传", func() {
			pipe := New("passthrough", &mockProcessor{name: "pass"})

			inputCh := make(chan Frame, 2)
			inputCh <- &StartFrame{Context: "hello"}
			inputCh <- &EndFrame{}
			close(inputCh)

			outputCh := pipe.Run(context.Background(), inputCh)

			var frames []Frame
			for f := range outputCh {
				frames = append(frames, f)
			}

			So(len(frames), ShouldEqual, 2)
			So(frames[0].FrameType(), ShouldEqual, "start")
			So(frames[1].FrameType(), ShouldEqual, "end")
		})

		PatchConvey("多处理器串联", func() {
			// p1: 添加前缀
			p1 := &mockProcessor{
				name: "prefixer",
				processFn: func(ctx context.Context, input <-chan Frame, output chan<- Frame) error {
					for frame := range input {
						if tf, ok := frame.(*textFrame); ok {
							output <- &textFrame{Text: "prefix-" + tf.Text}
						} else {
							output <- frame
						}
					}
					return nil
				},
			}
			// p2: 添加后缀
			p2 := &mockProcessor{
				name: "suffixer",
				processFn: func(ctx context.Context, input <-chan Frame, output chan<- Frame) error {
					for frame := range input {
						if tf, ok := frame.(*textFrame); ok {
							output <- &textFrame{Text: tf.Text + "-suffix"}
						} else {
							output <- frame
						}
					}
					return nil
				},
			}

			pipe := New("chain", p1, p2)
			inputCh := make(chan Frame, 1)
			inputCh <- &textFrame{Text: "hello"}
			close(inputCh)

			outputCh := pipe.Run(context.Background(), inputCh)
			var frames []Frame
			for f := range outputCh {
				frames = append(frames, f)
			}

			So(len(frames), ShouldEqual, 1)
			tf, ok := frames[0].(*textFrame)
			So(ok, ShouldBeTrue)
			So(tf.Text, ShouldEqual, "prefix-hello-suffix")
		})

		PatchConvey("Context 取消停止处理", func() {
			ctx, cancel := context.WithCancel(context.Background())

			blocker := &mockProcessor{
				name: "blocker",
				processFn: func(ctx context.Context, input <-chan Frame, output chan<- Frame) error {
					<-ctx.Done()
					return ctx.Err()
				},
			}

			pipe := New("cancel-test", blocker)
			inputCh := make(chan Frame)

			outputCh := pipe.Run(ctx, inputCh)

			// 取消 context
			cancel()

			// 输出 channel 应在 processor 退出后关闭
			timeout := time.After(2 * time.Second)
			select {
			case _, ok := <-outputCh:
				So(ok, ShouldBeFalse)
			case <-timeout:
				So("timeout waiting for output channel to close", ShouldBeEmpty)
			}
		})

		PatchConvey("nil ctx 自动填充 Background", func() {
			pipe := New("nil-ctx", &mockProcessor{name: "p1"})
			inputCh := make(chan Frame)
			close(inputCh)

			//nolint:staticcheck // 测试 nil ctx 处理
			outputCh := pipe.Run(nil, inputCh)

			// 应正常关闭
			_, ok := <-outputCh
			So(ok, ShouldBeFalse)
		})

		PatchConvey("处理器 panic 被捕获", func() {
			panicker := &mockProcessor{
				name: "panicker",
				processFn: func(ctx context.Context, input <-chan Frame, output chan<- Frame) error {
					panic("unexpected panic")
				},
			}

			pipe := New("panic-test", panicker)
			inputCh := make(chan Frame)
			close(inputCh)

			outputCh := pipe.Run(context.Background(), inputCh)

			// 输出 channel 应正常关闭（不会挂起）
			timeout := time.After(2 * time.Second)
			select {
			case _, ok := <-outputCh:
				So(ok, ShouldBeFalse)
			case <-timeout:
				So("timeout waiting for output channel to close", ShouldBeEmpty)
			}
		})

		PatchConvey("处理器返回错误-channel 仍正常关闭", func() {
			errProc := &mockProcessor{
				name: "err-proc",
				processFn: func(ctx context.Context, input <-chan Frame, output chan<- Frame) error {
					// 消费所有输入
					for range input {
					}
					return errors.New("process failed")
				},
			}

			pipe := New("err-test", errProc)
			inputCh := make(chan Frame)
			close(inputCh)

			outputCh := pipe.Run(context.Background(), inputCh)

			// 输出 channel 应正常关闭
			_, ok := <-outputCh
			So(ok, ShouldBeFalse)
		})

		PatchConvey("ErrorFrame 向下游传播", func() {
			// p1: 发送 ErrorFrame
			p1 := &mockProcessor{
				name: "error-emitter",
				processFn: func(ctx context.Context, input <-chan Frame, output chan<- Frame) error {
					for range input {
					}
					output <- &ErrorFrame{Err: errors.New("something wrong"), Message: "p1 error"}
					return nil
				},
			}
			// p2: 透传
			p2 := &mockProcessor{name: "pass"}

			pipe := New("error-propagate", p1, p2)
			inputCh := make(chan Frame)
			close(inputCh)

			outputCh := pipe.Run(context.Background(), inputCh)
			var frames []Frame
			for f := range outputCh {
				frames = append(frames, f)
			}

			So(len(frames), ShouldEqual, 1)
			ef, ok := frames[0].(*ErrorFrame)
			So(ok, ShouldBeTrue)
			So(ef.Message, ShouldEqual, "p1 error")
		})

		PatchConvey("MetadataFrame 处理器间传递", func() {
			// p1: 发送 MetadataFrame
			p1 := &mockProcessor{
				name: "meta-sender",
				processFn: func(ctx context.Context, input <-chan Frame, output chan<- Frame) error {
					for range input {
					}
					output <- &MetadataFrame{Key: "sample_rate", Value: 48000}
					return nil
				},
			}
			// p2: 消费 MetadataFrame
			var receivedRate int
			p2 := &mockProcessor{
				name: "meta-consumer",
				processFn: func(ctx context.Context, input <-chan Frame, output chan<- Frame) error {
					for frame := range input {
						if mf, ok := frame.(*MetadataFrame); ok && mf.Key == "sample_rate" {
							receivedRate = mf.Value.(int)
						}
						output <- frame
					}
					return nil
				},
			}

			pipe := New("metadata-test", p1, p2)
			inputCh := make(chan Frame)
			close(inputCh)

			outputCh := pipe.Run(context.Background(), inputCh)
			for range outputCh {
			}

			So(receivedRate, ShouldEqual, 48000)
		})

		PatchConvey("StartFrame.Context 传递", func() {
			var receivedCtx any
			p := &mockProcessor{
				name: "ctx-reader",
				processFn: func(ctx context.Context, input <-chan Frame, output chan<- Frame) error {
					for frame := range input {
						if sf, ok := frame.(*StartFrame); ok {
							receivedCtx = sf.Context
						}
						output <- frame
					}
					return nil
				},
			}

			pipe := New("ctx-test", p)
			inputCh := make(chan Frame, 1)
			inputCh <- &StartFrame{Context: map[string]string{"session": "abc"}}
			close(inputCh)

			outputCh := pipe.Run(context.Background(), inputCh)
			for range outputCh {
			}

			So(receivedCtx, ShouldNotBeNil)
			m, ok := receivedCtx.(map[string]string)
			So(ok, ShouldBeTrue)
			So(m["session"], ShouldEqual, "abc")
		})
	})
}

// ==================== Monitor 集成测试 ====================

func TestPipeline_Run_Monitor(t *testing.T) {
	PatchConvey("TestPipeline_Run_Monitor", t, func() {
		Mock(GetConfig).Return(&Config{BufferSize: 64, DisableMonitor: false}).Build()

		PatchConvey("全部成功调用验证", func() {
			mon := newTestMonitor()
			MockValue(&defaultMonitorInstance).To(mon)

			pipe := New("with-monitor",
				&mockProcessor{name: "p1"},
				&mockProcessor{name: "p2"},
			)
			inputCh := make(chan Frame, 1)
			inputCh <- &textFrame{Text: "test"}
			close(inputCh)

			outputCh := pipe.Run(context.Background(), inputCh)
			for range outputCh {
			}

			// 等待 OnPipelineDone
			So(mon.waitDone(2*time.Second), ShouldBeTrue)

			// 验证 OnProcessorDone 被调用 2 次
			processCalls := mon.processorDoneCalls()
			So(len(processCalls), ShouldEqual, 2)
			// 两个处理器都应成功
			for _, c := range processCalls {
				So(c.err, ShouldBeNil)
				So(c.pipelineName, ShouldEqual, "with-monitor")
			}

			// 验证 OnPipelineDone 被调用 1 次
			pipeCalls := mon.pipelineDoneCalls()
			So(len(pipeCalls), ShouldEqual, 1)
			So(pipeCalls[0].pipelineName, ShouldEqual, "with-monitor")
			So(pipeCalls[0].result.Success(), ShouldBeTrue)
		})

		PatchConvey("处理器失败记录", func() {
			mon := newTestMonitor()
			MockValue(&defaultMonitorInstance).To(mon)

			pipe := New("fail-monitor",
				&mockProcessor{
					name: "fail-proc",
					processFn: func(ctx context.Context, input <-chan Frame, output chan<- Frame) error {
						for range input {
						}
						return errors.New("process error")
					},
				},
			)
			inputCh := make(chan Frame)
			close(inputCh)

			outputCh := pipe.Run(context.Background(), inputCh)
			for range outputCh {
			}

			So(mon.waitDone(2*time.Second), ShouldBeTrue)

			processCalls := mon.processorDoneCalls()
			So(len(processCalls), ShouldEqual, 1)
			So(processCalls[0].err, ShouldNotBeNil)
			So(processCalls[0].processorName, ShouldEqual, "fail-proc")

			pipeCalls := mon.pipelineDoneCalls()
			So(len(pipeCalls), ShouldEqual, 1)
			So(pipeCalls[0].result.Success(), ShouldBeFalse)
			So(pipeCalls[0].result.HasErrors(), ShouldBeTrue)
		})

		PatchConvey("DisableMonitor 不调用 Monitor", func() {
			Mock(GetConfig).Return(&Config{BufferSize: 64, DisableMonitor: true}).Build()
			mon := newTestMonitor()
			MockValue(&defaultMonitorInstance).To(mon)

			pipe := New("no-monitor", &mockProcessor{name: "p1"})
			inputCh := make(chan Frame)
			close(inputCh)

			outputCh := pipe.Run(context.Background(), inputCh)
			for range outputCh {
			}

			// 等一下确保没有异步调用
			time.Sleep(100 * time.Millisecond)

			mon.mu.Lock()
			callCount := len(mon.calls)
			mon.mu.Unlock()
			So(callCount, ShouldEqual, 0)
		})

		PatchConvey("panic 处理器在 Monitor 中记录错误", func() {
			mon := newTestMonitor()
			MockValue(&defaultMonitorInstance).To(mon)

			pipe := New("panic-monitor",
				&mockProcessor{
					name: "panicker",
					processFn: func(ctx context.Context, input <-chan Frame, output chan<- Frame) error {
						panic("unexpected")
					},
				},
			)
			inputCh := make(chan Frame)
			close(inputCh)

			outputCh := pipe.Run(context.Background(), inputCh)
			for range outputCh {
			}

			So(mon.waitDone(2*time.Second), ShouldBeTrue)

			processCalls := mon.processorDoneCalls()
			So(len(processCalls), ShouldEqual, 1)
			So(processCalls[0].err, ShouldNotBeNil)
			So(processCalls[0].err.Error(), ShouldContainSubstring, "panic")

			pipeCalls := mon.pipelineDoneCalls()
			So(len(pipeCalls), ShouldEqual, 1)
			So(pipeCalls[0].result.Success(), ShouldBeFalse)
		})
	})
}
