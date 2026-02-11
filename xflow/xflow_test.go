package xflow

import (
	"context"
	"errors"
	"testing"
	"time"

	. "github.com/bytedance/mockey"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/xiaoshicae/xone/v2/xconfig"
)

// ==================== 测试用 Processor 实现 ====================

type mockProcessor[T any] struct {
	name       string
	dependency Dependency
	processFn  func(ctx context.Context, data T) error
	rollbackFn func(ctx context.Context, data T) error
}

func (m *mockProcessor[T]) Name() string           { return m.name }
func (m *mockProcessor[T]) Dependency() Dependency { return m.dependency }

func (m *mockProcessor[T]) Process(ctx context.Context, data T) error {
	if m.processFn != nil {
		return m.processFn(ctx, data)
	}
	return nil
}

func (m *mockProcessor[T]) Rollback(ctx context.Context, data T) error {
	if m.rollbackFn != nil {
		return m.rollbackFn(ctx, data)
	}
	return nil
}

// ==================== 测试用 Monitor ====================

type monitorCall struct {
	method        string // "OnProcessDone" / "OnRollbackDone" / "OnFlowDone"
	flowName      string
	processorName string
	dependency    Dependency
	err           error
	duration      time.Duration
	result        *ExecuteResult
}

type testMonitor struct {
	calls []monitorCall
}

func (m *testMonitor) OnProcessDone(_ context.Context, e *StepEvent) {
	m.calls = append(m.calls, monitorCall{
		method:        "OnProcessDone",
		flowName:      e.FlowName,
		processorName: e.ProcessorName,
		dependency:    e.Dependency,
		err:           e.Err,
		duration:      e.Duration,
	})
}

func (m *testMonitor) OnRollbackDone(_ context.Context, e *StepEvent) {
	m.calls = append(m.calls, monitorCall{
		method:        "OnRollbackDone",
		flowName:      e.FlowName,
		processorName: e.ProcessorName,
		dependency:    e.Dependency,
		err:           e.Err,
		duration:      e.Duration,
	})
}

func (m *testMonitor) OnFlowDone(_ context.Context, e *FlowEvent) {
	m.calls = append(m.calls, monitorCall{
		method:   "OnFlowDone",
		flowName: e.FlowName,
		result:   e.Result,
		duration: e.Duration,
	})
}

// processDoneCalls 返回所有 OnProcessDone 调用
func (m *testMonitor) processDoneCalls() []monitorCall {
	var result []monitorCall
	for _, c := range m.calls {
		if c.method == "OnProcessDone" {
			result = append(result, c)
		}
	}
	return result
}

// rollbackDoneCalls 返回所有 OnRollbackDone 调用
func (m *testMonitor) rollbackDoneCalls() []monitorCall {
	var result []monitorCall
	for _, c := range m.calls {
		if c.method == "OnRollbackDone" {
			result = append(result, c)
		}
	}
	return result
}

// flowDoneCalls 返回所有 OnFlowDone 调用
func (m *testMonitor) flowDoneCalls() []monitorCall {
	var result []monitorCall
	for _, c := range m.calls {
		if c.method == "OnFlowDone" {
			result = append(result, c)
		}
	}
	return result
}

// ==================== 测试用数据 ====================

type testData struct {
	Value   string
	Counter int
}

// ==================== Dependency 测试 ====================

func TestDependency_String(t *testing.T) {
	PatchConvey("TestDependency_String-Strong", t, func() {
		So(Strong.String(), ShouldEqual, "Strong")
	})

	PatchConvey("TestDependency_String-Weak", t, func() {
		So(Weak.String(), ShouldEqual, "Weak")
	})

	PatchConvey("TestDependency_String-Unknown", t, func() {
		So(Dependency(99).String(), ShouldEqual, "Unknown")
	})
}

// ==================== Monitor 测试 ====================

func TestDefaultMonitor(t *testing.T) {
	PatchConvey("TestDefaultMonitor-不 panic", t, func() {
		m := &defaultMonitor{}
		ctx := context.Background()
		result := &ExecuteResult{}

		se := &StepEvent{FlowName: "flow", ProcessorName: "proc", Dependency: Strong, Duration: time.Millisecond}
		seErr := &StepEvent{FlowName: "flow", ProcessorName: "proc", Dependency: Strong, Err: errors.New("err"), Duration: time.Millisecond}
		fe := &FlowEvent{FlowName: "flow", Result: result, Duration: time.Millisecond}
		feErr := &FlowEvent{FlowName: "flow", Result: &ExecuteResult{Err: errors.New("fail")}, Duration: time.Millisecond}

		So(func() { m.OnProcessDone(ctx, se) }, ShouldNotPanic)
		So(func() { m.OnProcessDone(ctx, seErr) }, ShouldNotPanic)
		So(func() { m.OnRollbackDone(ctx, se) }, ShouldNotPanic)
		So(func() { m.OnRollbackDone(ctx, seErr) }, ShouldNotPanic)
		So(func() { m.OnFlowDone(ctx, fe) }, ShouldNotPanic)
		So(func() { m.OnFlowDone(ctx, feErr) }, ShouldNotPanic)
	})
}

func TestSetDefaultMonitor(t *testing.T) {
	PatchConvey("TestSetDefaultMonitor-替换全局默认 Monitor", t, func() {
		original := GetDefaultMonitor()
		So(original, ShouldNotBeNil)

		custom := &testMonitor{}
		SetDefaultMonitor(custom)
		So(GetDefaultMonitor(), ShouldEqual, custom)

		// 恢复
		SetDefaultMonitor(original)
		So(GetDefaultMonitor(), ShouldEqual, original)
	})
}

// ==================== StepError 测试 ====================

func TestStepError(t *testing.T) {
	PatchConvey("TestStepError-Error 格式化", t, func() {
		se := &StepError{
			ProcessorName: "validate",
			Dependency:    Strong,
			Err:           errors.New("invalid input"),
		}
		So(se.Error(), ShouldContainSubstring, "validate")
		So(se.Error(), ShouldContainSubstring, "Strong")
		So(se.Error(), ShouldContainSubstring, "invalid input")
	})

	PatchConvey("TestStepError-Unwrap", t, func() {
		original := errors.New("original error")
		se := &StepError{
			ProcessorName: "test",
			Dependency:    Weak,
			Err:           original,
		}
		So(se.Unwrap(), ShouldEqual, original)
		So(errors.Is(se, original), ShouldBeTrue)
	})
}

// ==================== ExecuteResult 测试 ====================

func TestExecuteResult(t *testing.T) {
	PatchConvey("TestExecuteResult-Success", t, func() {
		r := &ExecuteResult{}
		So(r.Success(), ShouldBeTrue)
		So(r.String(), ShouldBeEmpty)
	})

	PatchConvey("TestExecuteResult-Failed", t, func() {
		r := &ExecuteResult{
			Err:    &StepError{ProcessorName: "p1", Err: errors.New("fail")},
			Rolled: true,
		}
		So(r.Success(), ShouldBeFalse)
		So(r.String(), ShouldContainSubstring, "flow failed")
		So(r.String(), ShouldContainSubstring, "rolled back")
	})

	PatchConvey("TestExecuteResult-HasSkippedErrors", t, func() {
		r := &ExecuteResult{
			SkippedErrors: []*StepError{{ProcessorName: "weak1", Err: errors.New("skip")}},
		}
		So(r.HasSkippedErrors(), ShouldBeTrue)
		So(r.HasRollbackErrors(), ShouldBeFalse)
	})

	PatchConvey("TestExecuteResult-HasRollbackErrors", t, func() {
		r := &ExecuteResult{
			Err:            &StepError{ProcessorName: "p1", Err: errors.New("fail")},
			Rolled:         true,
			RollbackErrors: []*StepError{{ProcessorName: "p0", Err: errors.New("rollback fail")}},
		}
		So(r.HasRollbackErrors(), ShouldBeTrue)
		So(r.String(), ShouldContainSubstring, "rollback errors=[1]")
	})
}

// ==================== Flow.Execute 测试 ====================

func TestFlow_Execute_EmptyProcessors(t *testing.T) {
	PatchConvey("TestFlow_Execute-空流程", t, func() {
		Mock(GetConfig).Return(&Config{DisableMonitor: true}).Build()

		flow := &Flow[testData]{Name: "empty"}
		result := flow.Execute(context.Background(), testData{})

		So(result.Success(), ShouldBeTrue)
		So(result.Rolled, ShouldBeFalse)
		So(result.SkippedErrors, ShouldBeEmpty)
	})
}

func TestFlow_Execute_AllSuccess(t *testing.T) {
	PatchConvey("TestFlow_Execute-全部成功", t, func() {
		Mock(GetConfig).Return(&Config{DisableMonitor: true}).Build()

		flow := &Flow[testData]{
			Name: "all-success",
			Processors: []Processor[testData]{
				&mockProcessor[testData]{name: "p1", dependency: Strong},
				&mockProcessor[testData]{name: "p2", dependency: Strong},
				&mockProcessor[testData]{name: "p3", dependency: Weak},
			},
		}
		result := flow.Execute(context.Background(), testData{})

		So(result.Success(), ShouldBeTrue)
		So(result.Rolled, ShouldBeFalse)
		So(result.HasSkippedErrors(), ShouldBeFalse)
	})
}

func TestFlow_Execute_StrongDependencyFail(t *testing.T) {
	PatchConvey("TestFlow_Execute-强依赖失败触发回滚", t, func() {
		Mock(GetConfig).Return(&Config{DisableMonitor: true}).Build()

		var rollbackOrder []string

		flow := &Flow[testData]{
			Name: "strong-fail",
			Processors: []Processor[testData]{
				&mockProcessor[testData]{
					name:       "p1",
					dependency: Strong,
					rollbackFn: func(ctx context.Context, data testData) error {
						rollbackOrder = append(rollbackOrder, "p1")
						return nil
					},
				},
				&mockProcessor[testData]{
					name:       "p2",
					dependency: Strong,
					rollbackFn: func(ctx context.Context, data testData) error {
						rollbackOrder = append(rollbackOrder, "p2")
						return nil
					},
				},
				&mockProcessor[testData]{
					name:       "p3",
					dependency: Strong,
					processFn: func(ctx context.Context, data testData) error {
						return errors.New("p3 failed")
					},
				},
			},
		}
		result := flow.Execute(context.Background(), testData{})

		So(result.Success(), ShouldBeFalse)
		So(result.Rolled, ShouldBeTrue)
		So(result.Err, ShouldNotBeNil)

		// 验证回滚逆序：p2 → p1
		So(rollbackOrder, ShouldResemble, []string{"p2", "p1"})

		// 验证 Unwrap 到原始错误
		var se *StepError
		So(errors.As(result.Err, &se), ShouldBeTrue)
		So(se.ProcessorName, ShouldEqual, "p3")
		So(se.Err.Error(), ShouldEqual, "p3 failed")
	})
}

func TestFlow_Execute_WeakDependencySkip(t *testing.T) {
	PatchConvey("TestFlow_Execute-弱依赖跳过继续执行", t, func() {
		Mock(GetConfig).Return(&Config{DisableMonitor: true}).Build()

		var executed []string

		flow := &Flow[testData]{
			Name: "weak-skip",
			Processors: []Processor[testData]{
				&mockProcessor[testData]{
					name:       "p1",
					dependency: Strong,
					processFn: func(ctx context.Context, data testData) error {
						executed = append(executed, "p1")
						return nil
					},
				},
				&mockProcessor[testData]{
					name:       "weak1",
					dependency: Weak,
					processFn: func(ctx context.Context, data testData) error {
						executed = append(executed, "weak1")
						return errors.New("weak error")
					},
				},
				&mockProcessor[testData]{
					name:       "p3",
					dependency: Strong,
					processFn: func(ctx context.Context, data testData) error {
						executed = append(executed, "p3")
						return nil
					},
				},
			},
		}
		result := flow.Execute(context.Background(), testData{})

		So(result.Success(), ShouldBeTrue)
		So(result.HasSkippedErrors(), ShouldBeTrue)
		So(len(result.SkippedErrors), ShouldEqual, 1)
		So(result.SkippedErrors[0].ProcessorName, ShouldEqual, "weak1")

		// p3 应继续执行
		So(executed, ShouldResemble, []string{"p1", "weak1", "p3"})
	})
}

func TestFlow_Execute_WeakAndStrongMixedFail(t *testing.T) {
	PatchConvey("TestFlow_Execute-弱+强混合失败", t, func() {
		Mock(GetConfig).Return(&Config{DisableMonitor: true}).Build()

		var rollbackOrder []string

		flow := &Flow[testData]{
			Name: "mixed-fail",
			Processors: []Processor[testData]{
				&mockProcessor[testData]{
					name:       "p1",
					dependency: Strong,
					rollbackFn: func(ctx context.Context, data testData) error {
						rollbackOrder = append(rollbackOrder, "p1")
						return nil
					},
				},
				&mockProcessor[testData]{
					name:       "weak1",
					dependency: Weak,
					processFn: func(ctx context.Context, data testData) error {
						return errors.New("weak error")
					},
					rollbackFn: func(ctx context.Context, data testData) error {
						rollbackOrder = append(rollbackOrder, "weak1")
						return nil
					},
				},
				&mockProcessor[testData]{
					name:       "p3",
					dependency: Strong,
					processFn: func(ctx context.Context, data testData) error {
						return errors.New("strong error")
					},
				},
			},
		}
		result := flow.Execute(context.Background(), testData{})

		So(result.Success(), ShouldBeFalse)
		So(result.Rolled, ShouldBeTrue)
		So(result.HasSkippedErrors(), ShouldBeTrue)
		So(len(result.SkippedErrors), ShouldEqual, 1)

		// 弱依赖失败后加入 succeeded，回滚时也会被回滚
		So(rollbackOrder, ShouldResemble, []string{"weak1", "p1"})
	})
}

func TestFlow_Execute_ProcessPanic(t *testing.T) {
	PatchConvey("TestFlow_Execute-Process panic 被捕获", t, func() {
		Mock(GetConfig).Return(&Config{DisableMonitor: true}).Build()

		flow := &Flow[testData]{
			Name: "panic",
			Processors: []Processor[testData]{
				&mockProcessor[testData]{
					name:       "panic-processor",
					dependency: Strong,
					processFn: func(ctx context.Context, data testData) error {
						panic("unexpected panic")
					},
				},
			},
		}
		result := flow.Execute(context.Background(), testData{})

		So(result.Success(), ShouldBeFalse)
		var se *StepError
		So(errors.As(result.Err, &se), ShouldBeTrue)
		So(se.Err.Error(), ShouldContainSubstring, "panic")
	})
}

func TestFlow_Execute_RollbackFail(t *testing.T) {
	PatchConvey("TestFlow_Execute-Rollback 失败不中断", t, func() {
		Mock(GetConfig).Return(&Config{DisableMonitor: true}).Build()

		var rollbackOrder []string

		flow := &Flow[testData]{
			Name: "rollback-fail",
			Processors: []Processor[testData]{
				&mockProcessor[testData]{
					name:       "p1",
					dependency: Strong,
					rollbackFn: func(ctx context.Context, data testData) error {
						rollbackOrder = append(rollbackOrder, "p1")
						return nil
					},
				},
				&mockProcessor[testData]{
					name:       "p2",
					dependency: Strong,
					rollbackFn: func(ctx context.Context, data testData) error {
						rollbackOrder = append(rollbackOrder, "p2")
						return errors.New("rollback error")
					},
				},
				&mockProcessor[testData]{
					name:       "p3",
					dependency: Strong,
					processFn: func(ctx context.Context, data testData) error {
						return errors.New("p3 failed")
					},
				},
			},
		}
		result := flow.Execute(context.Background(), testData{})

		So(result.Success(), ShouldBeFalse)
		So(result.Rolled, ShouldBeTrue)
		So(result.HasRollbackErrors(), ShouldBeTrue)
		So(len(result.RollbackErrors), ShouldEqual, 1)
		So(result.RollbackErrors[0].ProcessorName, ShouldEqual, "p2")

		// p2 回滚失败不影响 p1 继续回滚
		So(rollbackOrder, ShouldResemble, []string{"p2", "p1"})
	})
}

func TestFlow_Execute_RollbackPanic(t *testing.T) {
	PatchConvey("TestFlow_Execute-Rollback panic 被捕获", t, func() {
		Mock(GetConfig).Return(&Config{DisableMonitor: true}).Build()

		flow := &Flow[testData]{
			Name: "rollback-panic",
			Processors: []Processor[testData]{
				&mockProcessor[testData]{
					name:       "p1",
					dependency: Strong,
					rollbackFn: func(ctx context.Context, data testData) error {
						panic("rollback panic")
					},
				},
				&mockProcessor[testData]{
					name:       "p2",
					dependency: Strong,
					processFn: func(ctx context.Context, data testData) error {
						return errors.New("p2 failed")
					},
				},
			},
		}
		result := flow.Execute(context.Background(), testData{})

		So(result.Success(), ShouldBeFalse)
		So(result.Rolled, ShouldBeTrue)
		So(result.HasRollbackErrors(), ShouldBeTrue)
		So(result.RollbackErrors[0].Err.Error(), ShouldContainSubstring, "panic")
	})
}

func TestFlow_Execute_DataPassBetweenProcessors(t *testing.T) {
	PatchConvey("TestFlow_Execute-Processor 间数据传递（指针类型）", t, func() {
		Mock(GetConfig).Return(&Config{DisableMonitor: true}).Build()

		flow := &Flow[*testData]{
			Name: "data-pass",
			Processors: []Processor[*testData]{
				&mockProcessor[*testData]{
					name:       "writer",
					dependency: Strong,
					processFn: func(ctx context.Context, data *testData) error {
						data.Value = "written"
						data.Counter = 10
						return nil
					},
				},
				&mockProcessor[*testData]{
					name:       "reader",
					dependency: Strong,
					processFn: func(ctx context.Context, data *testData) error {
						if data.Value != "written" || data.Counter != 10 {
							return errors.New("data not passed")
						}
						data.Counter = 20
						return nil
					},
				},
			},
		}
		data := &testData{}
		result := flow.Execute(context.Background(), data)

		So(result.Success(), ShouldBeTrue)
		So(data.Value, ShouldEqual, "written")
		So(data.Counter, ShouldEqual, 20)
	})
}

// ==================== Monitor 集成测试 ====================

func TestFlow_Execute_MonitorDisabled(t *testing.T) {
	PatchConvey("TestFlow_Execute-MonitorDisabled 不调用 Monitor", t, func() {
		Mock(GetConfig).Return(&Config{DisableMonitor: true}).Build()

		mon := &testMonitor{}
		MockValue(&defaultMonitorInstance).To(mon)

		flow := &Flow[testData]{
			Name: "no-monitor",
			Processors: []Processor[testData]{
				&mockProcessor[testData]{name: "p1", dependency: Strong},
			},
		}
		result := flow.Execute(context.Background(), testData{})

		So(result.Success(), ShouldBeTrue)
		// DisableMonitor=true 时 Monitor 不被调用
		So(len(mon.calls), ShouldEqual, 0)
	})
}

func TestFlow_Execute_MonitorEnabled(t *testing.T) {
	PatchConvey("TestFlow_Execute-MonitorEnabled 调用验证", t, func() {
		Mock(GetConfig).Return(&Config{DisableMonitor: false}).Build()

		mon := &testMonitor{}
		MockValue(&defaultMonitorInstance).To(mon)

		flow := &Flow[testData]{
			Name: "with-monitor",
			Processors: []Processor[testData]{
				&mockProcessor[testData]{name: "p1", dependency: Strong},
				&mockProcessor[testData]{name: "p2", dependency: Weak},
			},
		}
		result := flow.Execute(context.Background(), testData{})

		So(result.Success(), ShouldBeTrue)

		// 验证 OnProcessDone 被调用 2 次
		processCalls := mon.processDoneCalls()
		So(len(processCalls), ShouldEqual, 2)
		So(processCalls[0].processorName, ShouldEqual, "p1")
		So(processCalls[0].err, ShouldBeNil)
		So(processCalls[1].processorName, ShouldEqual, "p2")
		So(processCalls[1].err, ShouldBeNil)

		// 验证 OnFlowDone 被调用 1 次
		flowCalls := mon.flowDoneCalls()
		So(len(flowCalls), ShouldEqual, 1)
		So(flowCalls[0].flowName, ShouldEqual, "with-monitor")
		So(flowCalls[0].result.Success(), ShouldBeTrue)

		// 无回滚调用
		So(len(mon.rollbackDoneCalls()), ShouldEqual, 0)
	})
}

func TestFlow_Execute_MonitorDuration(t *testing.T) {
	PatchConvey("TestFlow_Execute-MonitorDuration 验证耗时 > 0", t, func() {
		Mock(GetConfig).Return(&Config{DisableMonitor: false}).Build()

		mon := &testMonitor{}
		MockValue(&defaultMonitorInstance).To(mon)

		flow := &Flow[testData]{
			Name: "duration-check",
			Processors: []Processor[testData]{
				&mockProcessor[testData]{name: "p1", dependency: Strong},
			},
		}
		flow.Execute(context.Background(), testData{})

		// OnProcessDone duration >= 0（执行非常快，但不应为负数）
		processCalls := mon.processDoneCalls()
		So(len(processCalls), ShouldEqual, 1)
		So(processCalls[0].duration, ShouldBeGreaterThanOrEqualTo, 0)

		// OnFlowDone duration >= 0
		flowCalls := mon.flowDoneCalls()
		So(len(flowCalls), ShouldEqual, 1)
		So(flowCalls[0].duration, ShouldBeGreaterThanOrEqualTo, 0)
	})
}

func TestFlow_Execute_MonitorWithFailure(t *testing.T) {
	PatchConvey("TestFlow_Execute-MonitorWithFailure 强依赖失败", t, func() {
		Mock(GetConfig).Return(&Config{DisableMonitor: false}).Build()

		mon := &testMonitor{}
		MockValue(&defaultMonitorInstance).To(mon)

		flow := &Flow[testData]{
			Name: "monitor-fail",
			Processors: []Processor[testData]{
				&mockProcessor[testData]{
					name:       "p1",
					dependency: Strong,
					rollbackFn: func(ctx context.Context, data testData) error { return nil },
				},
				&mockProcessor[testData]{
					name:       "p2",
					dependency: Strong,
					processFn: func(ctx context.Context, data testData) error {
						return errors.New("p2 failed")
					},
				},
			},
		}
		result := flow.Execute(context.Background(), testData{})

		So(result.Success(), ShouldBeFalse)

		// OnProcessDone 调用 2 次：p1 成功 + p2 失败
		processCalls := mon.processDoneCalls()
		So(len(processCalls), ShouldEqual, 2)
		So(processCalls[0].processorName, ShouldEqual, "p1")
		So(processCalls[0].err, ShouldBeNil)
		So(processCalls[1].processorName, ShouldEqual, "p2")
		So(processCalls[1].err, ShouldNotBeNil)

		// OnRollbackDone 调用 1 次：p1 回滚
		rollbackCalls := mon.rollbackDoneCalls()
		So(len(rollbackCalls), ShouldEqual, 1)
		So(rollbackCalls[0].processorName, ShouldEqual, "p1")
		So(rollbackCalls[0].err, ShouldBeNil)

		// OnFlowDone 调用 1 次
		flowCalls := mon.flowDoneCalls()
		So(len(flowCalls), ShouldEqual, 1)
		So(flowCalls[0].result.Success(), ShouldBeFalse)
		So(flowCalls[0].result.Rolled, ShouldBeTrue)
	})
}

func TestFlow_Execute_MonitorWeakSkip(t *testing.T) {
	PatchConvey("TestFlow_Execute-MonitorWeakSkip 弱依赖跳过记录", t, func() {
		Mock(GetConfig).Return(&Config{DisableMonitor: false}).Build()

		mon := &testMonitor{}
		MockValue(&defaultMonitorInstance).To(mon)

		flow := &Flow[testData]{
			Name: "monitor-weak",
			Processors: []Processor[testData]{
				&mockProcessor[testData]{name: "p1", dependency: Strong},
				&mockProcessor[testData]{
					name:       "weak1",
					dependency: Weak,
					processFn: func(ctx context.Context, data testData) error {
						return errors.New("weak error")
					},
				},
				&mockProcessor[testData]{name: "p3", dependency: Strong},
			},
		}
		result := flow.Execute(context.Background(), testData{})

		So(result.Success(), ShouldBeTrue)

		// OnProcessDone 调用 3 次
		processCalls := mon.processDoneCalls()
		So(len(processCalls), ShouldEqual, 3)

		// weak1 的 err 不为 nil
		So(processCalls[1].processorName, ShouldEqual, "weak1")
		So(processCalls[1].dependency, ShouldEqual, Weak)
		So(processCalls[1].err, ShouldNotBeNil)

		// 无回滚调用
		So(len(mon.rollbackDoneCalls()), ShouldEqual, 0)
	})
}

func TestFlow_Execute_MonitorDefaultImpl(t *testing.T) {
	PatchConvey("TestFlow_Execute-Monitor 为 nil 时使用全局默认实现", t, func() {
		Mock(GetConfig).Return(&Config{DisableMonitor: false}).Build()

		flow := &Flow[testData]{
			Name: "default-monitor",
			Processors: []Processor[testData]{
				&mockProcessor[testData]{name: "p1", dependency: Strong},
			},
		}

		// 不 panic 即可
		So(func() { flow.Execute(context.Background(), testData{}) }, ShouldNotPanic)
	})
}

// ==================== Setter 方法测试 ====================

func TestFlow_SetterMethods(t *testing.T) {
	PatchConvey("TestFlow_SetName", t, func() {
		f := &Flow[testData]{}
		f.SetName("new-name")
		So(f.Name, ShouldEqual, "new-name")
	})

	PatchConvey("TestFlow_AddProcessor", t, func() {
		f := &Flow[testData]{}
		p := &mockProcessor[testData]{name: "p1"}
		f.AddProcessor(p)
		So(len(f.Processors), ShouldEqual, 1)
		So(f.Processors[0].Name(), ShouldEqual, "p1")
	})
}

// ==================== nil ctx 测试 ====================

func TestFlow_Execute_NilCtx(t *testing.T) {
	PatchConvey("TestFlow_Execute-nil ctx 自动填充 Background", t, func() {
		Mock(GetConfig).Return(&Config{DisableMonitor: true}).Build()

		flow := &Flow[testData]{
			Name: "nil-ctx",
			Processors: []Processor[testData]{
				&mockProcessor[testData]{name: "p1", dependency: Strong},
			},
		}
		result := flow.Execute(nil, testData{})

		So(result.Success(), ShouldBeTrue)
	})
}

// ==================== New 函数测试 ====================

func TestNew(t *testing.T) {
	PatchConvey("TestNew-函数式构建", t, func() {
		Mock(GetConfig).Return(&Config{DisableMonitor: true}).Build()

		p1 := &mockProcessor[testData]{name: "p1", dependency: Strong}
		p2 := &mockProcessor[testData]{name: "p2", dependency: Weak}
		flow := New[testData]("test-flow", p1, p2)

		So(flow.Name, ShouldEqual, "test-flow")
		So(len(flow.Processors), ShouldEqual, 2)
		// 验证可正常执行
		result := flow.Execute(context.Background(), testData{})
		So(result.Success(), ShouldBeTrue)
	})
}

// ==================== WeakDependency 失败后回滚包含弱依赖处理器 ====================

func TestFlow_Execute_WeakFailRollbackIncluded(t *testing.T) {
	PatchConvey("TestFlow_Execute-弱依赖失败后强依赖失败时弱依赖也回滚", t, func() {
		Mock(GetConfig).Return(&Config{DisableMonitor: true}).Build()

		var rollbackOrder []string

		flow := &Flow[testData]{
			Name: "weak-rollback",
			Processors: []Processor[testData]{
				&mockProcessor[testData]{
					name:       "strong1",
					dependency: Strong,
					rollbackFn: func(ctx context.Context, data testData) error {
						rollbackOrder = append(rollbackOrder, "strong1")
						return nil
					},
				},
				&mockProcessor[testData]{
					name:       "weak1",
					dependency: Weak,
					processFn: func(ctx context.Context, data testData) error {
						return errors.New("weak fail")
					},
					rollbackFn: func(ctx context.Context, data testData) error {
						rollbackOrder = append(rollbackOrder, "weak1")
						return nil
					},
				},
				&mockProcessor[testData]{
					name:       "strong2",
					dependency: Strong,
					rollbackFn: func(ctx context.Context, data testData) error {
						rollbackOrder = append(rollbackOrder, "strong2")
						return nil
					},
				},
				&mockProcessor[testData]{
					name:       "strong3",
					dependency: Strong,
					processFn: func(ctx context.Context, data testData) error {
						return errors.New("strong fail")
					},
				},
			},
		}
		result := flow.Execute(context.Background(), testData{})

		So(result.Success(), ShouldBeFalse)
		So(result.Rolled, ShouldBeTrue)
		// 回滚逆序：strong2 → weak1 → strong1
		So(rollbackOrder, ShouldResemble, []string{"strong2", "weak1", "strong1"})
	})
}

// ==================== Process panic 弱依赖 ====================

func TestFlow_Execute_WeakProcessPanic(t *testing.T) {
	PatchConvey("TestFlow_Execute-弱依赖 Process panic 被跳过", t, func() {
		Mock(GetConfig).Return(&Config{DisableMonitor: true}).Build()

		flow := &Flow[testData]{
			Name: "weak-panic",
			Processors: []Processor[testData]{
				&mockProcessor[testData]{
					name:       "weak-panic",
					dependency: Weak,
					processFn: func(ctx context.Context, data testData) error {
						panic("weak panic")
					},
				},
				&mockProcessor[testData]{
					name:       "p2",
					dependency: Strong,
				},
			},
		}
		result := flow.Execute(context.Background(), testData{})

		So(result.Success(), ShouldBeTrue)
		So(result.HasSkippedErrors(), ShouldBeTrue)
		So(result.SkippedErrors[0].Err.Error(), ShouldContainSubstring, "panic")
	})
}

// ==================== Config 测试 ====================

func TestGetConfig(t *testing.T) {
	PatchConvey("TestGetConfig-UnmarshalConfig 成功", t, func() {
		c := GetConfig()
		So(c, ShouldNotBeNil)
		So(c.DisableMonitor, ShouldBeFalse)
	})

	PatchConvey("TestGetConfig-UnmarshalConfig 失败返回默认值", t, func() {
		Mock(xconfig.UnmarshalConfig).To(func(key string, conf any) error {
			return errors.New("unmarshal failed")
		}).Build()
		c := GetConfig()
		So(c, ShouldNotBeNil)
		So(c.DisableMonitor, ShouldBeFalse)
	})
}

func TestConfigMergeDefault(t *testing.T) {
	PatchConvey("TestConfigMergeDefault-nil 输入", t, func() {
		c := configMergeDefault(nil)
		So(c, ShouldNotBeNil)
		So(c.DisableMonitor, ShouldBeFalse)
	})

	PatchConvey("TestConfigMergeDefault-非 nil 输入", t, func() {
		input := &Config{DisableMonitor: true}
		c := configMergeDefault(input)
		So(c, ShouldEqual, input)
		So(c.DisableMonitor, ShouldBeTrue)
	})
}
