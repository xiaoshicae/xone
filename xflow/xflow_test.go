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

type mockProcessor[Req, Resp any] struct {
	name       string
	dependency Dependency
	processFn  func(ctx context.Context, data *FlowData[Req, Resp]) error
	rollbackFn func(ctx context.Context, data *FlowData[Req, Resp]) error
}

func (m *mockProcessor[Req, Resp]) Name() string           { return m.name }
func (m *mockProcessor[Req, Resp]) Dependency() Dependency { return m.dependency }

func (m *mockProcessor[Req, Resp]) Process(ctx context.Context, data *FlowData[Req, Resp]) error {
	if m.processFn != nil {
		return m.processFn(ctx, data)
	}
	return nil
}

func (m *mockProcessor[Req, Resp]) Rollback(ctx context.Context, data *FlowData[Req, Resp]) error {
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
	result        ResultSummary
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

type testReq struct {
	UserID int
	Input  string
}

type testResp struct {
	OrderID string
	Total   float64
}

// ==================== FlowData 测试 ====================

func TestFlowData_SetGet(t *testing.T) {
	PatchConvey("TestFlowData_SetGet", t, func() {
		PatchConvey("基本存取", func() {
			d := &FlowData[testReq, testResp]{Request: testReq{UserID: 1}}
			d.Set("key1", "value1")
			v, ok := d.Get("key1")
			So(ok, ShouldBeTrue)
			So(v, ShouldEqual, "value1")
		})

		PatchConvey("不存在的 key", func() {
			d := &FlowData[testReq, testResp]{}
			v, ok := d.Get("not-exist")
			So(ok, ShouldBeFalse)
			So(v, ShouldBeNil)
		})

		PatchConvey("惰性初始化", func() {
			d := &FlowData[testReq, testResp]{}
			// extra 为 nil 时 Get 不 panic
			_, ok := d.Get("any")
			So(ok, ShouldBeFalse)
			// Set 触发初始化
			d.Set("k", 42)
			v, ok := d.Get("k")
			So(ok, ShouldBeTrue)
			So(v, ShouldEqual, 42)
		})
	})
}

func TestFlowData_TypedExtra(t *testing.T) {
	PatchConvey("TestFlowData_TypedExtra", t, func() {
		PatchConvey("类型安全存取", func() {
			d := &FlowData[testReq, testResp]{}
			key := NewKey[string]("level")
			SetExtra(d, key, "VIP")
			v, ok := GetExtra(d, key)
			So(ok, ShouldBeTrue)
			So(v, ShouldEqual, "VIP")
		})

		PatchConvey("类型不匹配返回 false", func() {
			d := &FlowData[testReq, testResp]{}
			d.Set("num", "not-a-number")
			key := NewKey[int]("num")
			v, ok := GetExtra(d, key)
			So(ok, ShouldBeFalse)
			So(v, ShouldEqual, 0)
		})

		PatchConvey("key 不存在返回零值", func() {
			d := &FlowData[testReq, testResp]{}
			key := NewKey[string]("missing")
			v, ok := GetExtra(d, key)
			So(ok, ShouldBeFalse)
			So(v, ShouldBeEmpty)
		})
	})
}

// ==================== Dependency 测试 ====================

func TestDependency_String(t *testing.T) {
	PatchConvey("TestDependency_String", t, func() {
		So(Strong.String(), ShouldEqual, "Strong")
		So(Weak.String(), ShouldEqual, "Weak")
		So(Dependency(99).String(), ShouldEqual, "Unknown")
	})
}

// ==================== Monitor 测试 ====================

func TestDefaultMonitor(t *testing.T) {
	PatchConvey("TestDefaultMonitor-不 panic", t, func() {
		m := &defaultMonitor{}
		ctx := context.Background()
		result := &ExecuteResult[testResp]{}

		se := &StepEvent{FlowName: "flow", ProcessorName: "proc", Dependency: Strong, Duration: time.Millisecond}
		seErr := &StepEvent{FlowName: "flow", ProcessorName: "proc", Dependency: Strong, Err: errors.New("err"), Duration: time.Millisecond}
		fe := &FlowEvent{FlowName: "flow", Result: result, Duration: time.Millisecond}
		feErr := &FlowEvent{FlowName: "flow", Result: &ExecuteResult[testResp]{Err: errors.New("fail")}, Duration: time.Millisecond}

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
	PatchConvey("TestStepError", t, func() {
		PatchConvey("Error 格式化", func() {
			se := &StepError{
				ProcessorName: "validate",
				Dependency:    Strong,
				Err:           errors.New("invalid input"),
			}
			So(se.Error(), ShouldContainSubstring, "validate")
			So(se.Error(), ShouldContainSubstring, "Strong")
			So(se.Error(), ShouldContainSubstring, "invalid input")
		})

		PatchConvey("Unwrap", func() {
			original := errors.New("original error")
			se := &StepError{
				ProcessorName: "test",
				Dependency:    Weak,
				Err:           original,
			}
			So(se.Unwrap(), ShouldEqual, original)
			So(errors.Is(se, original), ShouldBeTrue)
		})
	})
}

// ==================== ExecuteResult 测试 ====================

func TestExecuteResult(t *testing.T) {
	PatchConvey("TestExecuteResult", t, func() {
		PatchConvey("Success", func() {
			r := &ExecuteResult[testResp]{}
			So(r.Success(), ShouldBeTrue)
			So(r.IsRolled(), ShouldBeFalse)
			So(r.String(), ShouldBeEmpty)
		})

		PatchConvey("Success with Data", func() {
			r := &ExecuteResult[testResp]{
				Data: testResp{OrderID: "ORD-1", Total: 99.9},
			}
			So(r.Success(), ShouldBeTrue)
			So(r.Data.OrderID, ShouldEqual, "ORD-1")
			So(r.Data.Total, ShouldEqual, 99.9)
		})

		PatchConvey("Failed", func() {
			r := &ExecuteResult[testResp]{
				Err:    &StepError{ProcessorName: "p1", Err: errors.New("fail")},
				Rolled: true,
			}
			So(r.Success(), ShouldBeFalse)
			So(r.IsRolled(), ShouldBeTrue)
			So(r.String(), ShouldContainSubstring, "flow failed")
			So(r.String(), ShouldContainSubstring, "rolled back")
		})

		PatchConvey("HasSkippedErrors", func() {
			r := &ExecuteResult[testResp]{
				SkippedErrors: []*StepError{{ProcessorName: "weak1", Err: errors.New("skip")}},
			}
			So(r.HasSkippedErrors(), ShouldBeTrue)
			So(r.HasRollbackErrors(), ShouldBeFalse)
		})

		PatchConvey("HasRollbackErrors", func() {
			r := &ExecuteResult[testResp]{
				Err:            &StepError{ProcessorName: "p1", Err: errors.New("fail")},
				Rolled:         true,
				RollbackErrors: []*StepError{{ProcessorName: "p0", Err: errors.New("rollback fail")}},
			}
			So(r.HasRollbackErrors(), ShouldBeTrue)
			So(r.String(), ShouldContainSubstring, "rollback errors=[1]")
		})

		PatchConvey("ResultSummary 接口兼容", func() {
			var summary ResultSummary = &ExecuteResult[testResp]{
				Err:    errors.New("fail"),
				Rolled: true,
			}
			So(summary.Success(), ShouldBeFalse)
			So(summary.IsRolled(), ShouldBeTrue)
		})
	})
}

// ==================== Flow.Execute 测试 ====================

func TestFlow_Execute(t *testing.T) {
	PatchConvey("TestFlow_Execute", t, func() {
		Mock(GetConfig).Return(&Config{DisableMonitor: true}).Build()

		PatchConvey("空流程", func() {
			flow := &Flow[testReq, testResp]{Name: "empty"}
			result := flow.Execute(context.Background(), testReq{})

			So(result.Success(), ShouldBeTrue)
			So(result.Rolled, ShouldBeFalse)
			So(result.SkippedErrors, ShouldBeEmpty)
		})

		PatchConvey("全部成功-Response 填充", func() {
			flow := &Flow[testReq, testResp]{
				Name: "all-success",
				Processors: []Processor[testReq, testResp]{
					&mockProcessor[testReq, testResp]{
						name:       "p1",
						dependency: Strong,
						processFn: func(ctx context.Context, data *FlowData[testReq, testResp]) error {
							data.Response.OrderID = "ORD-123"
							return nil
						},
					},
					&mockProcessor[testReq, testResp]{
						name:       "p2",
						dependency: Strong,
						processFn: func(ctx context.Context, data *FlowData[testReq, testResp]) error {
							data.Response.Total = 99.9
							return nil
						},
					},
					&mockProcessor[testReq, testResp]{name: "p3", dependency: Weak},
				},
			}
			result := flow.Execute(context.Background(), testReq{UserID: 1})

			So(result.Success(), ShouldBeTrue)
			So(result.Rolled, ShouldBeFalse)
			So(result.HasSkippedErrors(), ShouldBeFalse)
			So(result.Data.OrderID, ShouldEqual, "ORD-123")
			So(result.Data.Total, ShouldEqual, 99.9)
		})

		PatchConvey("Request 传递验证", func() {
			flow := &Flow[testReq, testResp]{
				Name: "req-pass",
				Processors: []Processor[testReq, testResp]{
					&mockProcessor[testReq, testResp]{
						name:       "reader",
						dependency: Strong,
						processFn: func(ctx context.Context, data *FlowData[testReq, testResp]) error {
							if data.Request.UserID != 42 {
								return errors.New("wrong user id")
							}
							data.Response.OrderID = "ORD-42"
							return nil
						},
					},
				},
			}
			result := flow.Execute(context.Background(), testReq{UserID: 42})

			So(result.Success(), ShouldBeTrue)
			So(result.Data.OrderID, ShouldEqual, "ORD-42")
		})

		PatchConvey("Extra 跨 Processor 传递", func() {
			levelKey := NewKey[string]("level")

			flow := &Flow[testReq, testResp]{
				Name: "extra-pass",
				Processors: []Processor[testReq, testResp]{
					&mockProcessor[testReq, testResp]{
						name:       "writer",
						dependency: Strong,
						processFn: func(ctx context.Context, data *FlowData[testReq, testResp]) error {
							SetExtra(data, levelKey, "VIP")
							return nil
						},
					},
					&mockProcessor[testReq, testResp]{
						name:       "reader",
						dependency: Strong,
						processFn: func(ctx context.Context, data *FlowData[testReq, testResp]) error {
							level, ok := GetExtra(data, levelKey)
							if !ok || level != "VIP" {
								return errors.New("extra not passed")
							}
							data.Response.OrderID = "VIP-ORDER"
							return nil
						},
					},
				},
			}
			result := flow.Execute(context.Background(), testReq{UserID: 1})

			So(result.Success(), ShouldBeTrue)
			So(result.Data.OrderID, ShouldEqual, "VIP-ORDER")
		})

		PatchConvey("强依赖失败触发回滚", func() {
			var rollbackOrder []string

			flow := &Flow[testReq, testResp]{
				Name: "strong-fail",
				Processors: []Processor[testReq, testResp]{
					&mockProcessor[testReq, testResp]{
						name:       "p1",
						dependency: Strong,
						rollbackFn: func(ctx context.Context, data *FlowData[testReq, testResp]) error {
							rollbackOrder = append(rollbackOrder, "p1")
							return nil
						},
					},
					&mockProcessor[testReq, testResp]{
						name:       "p2",
						dependency: Strong,
						rollbackFn: func(ctx context.Context, data *FlowData[testReq, testResp]) error {
							rollbackOrder = append(rollbackOrder, "p2")
							return nil
						},
					},
					&mockProcessor[testReq, testResp]{
						name:       "p3",
						dependency: Strong,
						processFn: func(ctx context.Context, data *FlowData[testReq, testResp]) error {
							return errors.New("p3 failed")
						},
					},
				},
			}
			result := flow.Execute(context.Background(), testReq{})

			So(result.Success(), ShouldBeFalse)
			So(result.Rolled, ShouldBeTrue)
			So(result.Err, ShouldNotBeNil)
			// 失败时 Data 保持零值
			So(result.Data.OrderID, ShouldBeEmpty)

			// 验证回滚逆序：p2 → p1
			So(rollbackOrder, ShouldResemble, []string{"p2", "p1"})

			// 验证 Unwrap 到原始错误
			var se *StepError
			So(errors.As(result.Err, &se), ShouldBeTrue)
			So(se.ProcessorName, ShouldEqual, "p3")
			So(se.Err.Error(), ShouldEqual, "p3 failed")
		})

		PatchConvey("弱依赖跳过继续执行", func() {
			var executed []string

			flow := &Flow[testReq, testResp]{
				Name: "weak-skip",
				Processors: []Processor[testReq, testResp]{
					&mockProcessor[testReq, testResp]{
						name:       "p1",
						dependency: Strong,
						processFn: func(ctx context.Context, data *FlowData[testReq, testResp]) error {
							executed = append(executed, "p1")
							return nil
						},
					},
					&mockProcessor[testReq, testResp]{
						name:       "weak1",
						dependency: Weak,
						processFn: func(ctx context.Context, data *FlowData[testReq, testResp]) error {
							executed = append(executed, "weak1")
							return errors.New("weak error")
						},
					},
					&mockProcessor[testReq, testResp]{
						name:       "p3",
						dependency: Strong,
						processFn: func(ctx context.Context, data *FlowData[testReq, testResp]) error {
							executed = append(executed, "p3")
							return nil
						},
					},
				},
			}
			result := flow.Execute(context.Background(), testReq{})

			So(result.Success(), ShouldBeTrue)
			So(result.HasSkippedErrors(), ShouldBeTrue)
			So(len(result.SkippedErrors), ShouldEqual, 1)
			So(result.SkippedErrors[0].ProcessorName, ShouldEqual, "weak1")

			// p3 应继续执行
			So(executed, ShouldResemble, []string{"p1", "weak1", "p3"})
		})

		PatchConvey("弱+强混合失败", func() {
			var rollbackOrder []string

			flow := &Flow[testReq, testResp]{
				Name: "mixed-fail",
				Processors: []Processor[testReq, testResp]{
					&mockProcessor[testReq, testResp]{
						name:       "p1",
						dependency: Strong,
						rollbackFn: func(ctx context.Context, data *FlowData[testReq, testResp]) error {
							rollbackOrder = append(rollbackOrder, "p1")
							return nil
						},
					},
					&mockProcessor[testReq, testResp]{
						name:       "weak1",
						dependency: Weak,
						processFn: func(ctx context.Context, data *FlowData[testReq, testResp]) error {
							return errors.New("weak error")
						},
						rollbackFn: func(ctx context.Context, data *FlowData[testReq, testResp]) error {
							rollbackOrder = append(rollbackOrder, "weak1")
							return nil
						},
					},
					&mockProcessor[testReq, testResp]{
						name:       "p3",
						dependency: Strong,
						processFn: func(ctx context.Context, data *FlowData[testReq, testResp]) error {
							return errors.New("strong error")
						},
					},
				},
			}
			result := flow.Execute(context.Background(), testReq{})

			So(result.Success(), ShouldBeFalse)
			So(result.Rolled, ShouldBeTrue)
			So(result.HasSkippedErrors(), ShouldBeTrue)
			So(len(result.SkippedErrors), ShouldEqual, 1)

			// 弱依赖失败后加入 succeeded，回滚时也会被回滚
			So(rollbackOrder, ShouldResemble, []string{"weak1", "p1"})
		})

		PatchConvey("Process panic 被捕获", func() {
			flow := &Flow[testReq, testResp]{
				Name: "panic",
				Processors: []Processor[testReq, testResp]{
					&mockProcessor[testReq, testResp]{
						name:       "panic-processor",
						dependency: Strong,
						processFn: func(ctx context.Context, data *FlowData[testReq, testResp]) error {
							panic("unexpected panic")
						},
					},
				},
			}
			result := flow.Execute(context.Background(), testReq{})

			So(result.Success(), ShouldBeFalse)
			var se *StepError
			So(errors.As(result.Err, &se), ShouldBeTrue)
			So(se.Err.Error(), ShouldContainSubstring, "panic")
		})

		PatchConvey("Rollback 失败不中断", func() {
			var rollbackOrder []string

			flow := &Flow[testReq, testResp]{
				Name: "rollback-fail",
				Processors: []Processor[testReq, testResp]{
					&mockProcessor[testReq, testResp]{
						name:       "p1",
						dependency: Strong,
						rollbackFn: func(ctx context.Context, data *FlowData[testReq, testResp]) error {
							rollbackOrder = append(rollbackOrder, "p1")
							return nil
						},
					},
					&mockProcessor[testReq, testResp]{
						name:       "p2",
						dependency: Strong,
						rollbackFn: func(ctx context.Context, data *FlowData[testReq, testResp]) error {
							rollbackOrder = append(rollbackOrder, "p2")
							return errors.New("rollback error")
						},
					},
					&mockProcessor[testReq, testResp]{
						name:       "p3",
						dependency: Strong,
						processFn: func(ctx context.Context, data *FlowData[testReq, testResp]) error {
							return errors.New("p3 failed")
						},
					},
				},
			}
			result := flow.Execute(context.Background(), testReq{})

			So(result.Success(), ShouldBeFalse)
			So(result.Rolled, ShouldBeTrue)
			So(result.HasRollbackErrors(), ShouldBeTrue)
			So(len(result.RollbackErrors), ShouldEqual, 1)
			So(result.RollbackErrors[0].ProcessorName, ShouldEqual, "p2")

			// p2 回滚失败不影响 p1 继续回滚
			So(rollbackOrder, ShouldResemble, []string{"p2", "p1"})
		})

		PatchConvey("Rollback panic 被捕获", func() {
			flow := &Flow[testReq, testResp]{
				Name: "rollback-panic",
				Processors: []Processor[testReq, testResp]{
					&mockProcessor[testReq, testResp]{
						name:       "p1",
						dependency: Strong,
						rollbackFn: func(ctx context.Context, data *FlowData[testReq, testResp]) error {
							panic("rollback panic")
						},
					},
					&mockProcessor[testReq, testResp]{
						name:       "p2",
						dependency: Strong,
						processFn: func(ctx context.Context, data *FlowData[testReq, testResp]) error {
							return errors.New("p2 failed")
						},
					},
				},
			}
			result := flow.Execute(context.Background(), testReq{})

			So(result.Success(), ShouldBeFalse)
			So(result.Rolled, ShouldBeTrue)
			So(result.HasRollbackErrors(), ShouldBeTrue)
			So(result.RollbackErrors[0].Err.Error(), ShouldContainSubstring, "panic")
		})

		PatchConvey("nil ctx 自动填充 Background", func() {
			flow := &Flow[testReq, testResp]{
				Name: "nil-ctx",
				Processors: []Processor[testReq, testResp]{
					&mockProcessor[testReq, testResp]{name: "p1", dependency: Strong},
				},
			}
			//nolint:staticcheck // 测试 nil ctx 处理
			result := flow.Execute(nil, testReq{})

			So(result.Success(), ShouldBeTrue)
		})

		PatchConvey("弱依赖失败后强依赖失败时弱依赖也回滚", func() {
			var rollbackOrder []string

			flow := &Flow[testReq, testResp]{
				Name: "weak-rollback",
				Processors: []Processor[testReq, testResp]{
					&mockProcessor[testReq, testResp]{
						name:       "strong1",
						dependency: Strong,
						rollbackFn: func(ctx context.Context, data *FlowData[testReq, testResp]) error {
							rollbackOrder = append(rollbackOrder, "strong1")
							return nil
						},
					},
					&mockProcessor[testReq, testResp]{
						name:       "weak1",
						dependency: Weak,
						processFn: func(ctx context.Context, data *FlowData[testReq, testResp]) error {
							return errors.New("weak fail")
						},
						rollbackFn: func(ctx context.Context, data *FlowData[testReq, testResp]) error {
							rollbackOrder = append(rollbackOrder, "weak1")
							return nil
						},
					},
					&mockProcessor[testReq, testResp]{
						name:       "strong2",
						dependency: Strong,
						rollbackFn: func(ctx context.Context, data *FlowData[testReq, testResp]) error {
							rollbackOrder = append(rollbackOrder, "strong2")
							return nil
						},
					},
					&mockProcessor[testReq, testResp]{
						name:       "strong3",
						dependency: Strong,
						processFn: func(ctx context.Context, data *FlowData[testReq, testResp]) error {
							return errors.New("strong fail")
						},
					},
				},
			}
			result := flow.Execute(context.Background(), testReq{})

			So(result.Success(), ShouldBeFalse)
			So(result.Rolled, ShouldBeTrue)
			// 回滚逆序：strong2 → weak1 → strong1
			So(rollbackOrder, ShouldResemble, []string{"strong2", "weak1", "strong1"})
		})

		PatchConvey("弱依赖 Process panic 被跳过", func() {
			flow := &Flow[testReq, testResp]{
				Name: "weak-panic",
				Processors: []Processor[testReq, testResp]{
					&mockProcessor[testReq, testResp]{
						name:       "weak-panic",
						dependency: Weak,
						processFn: func(ctx context.Context, data *FlowData[testReq, testResp]) error {
							panic("weak panic")
						},
					},
					&mockProcessor[testReq, testResp]{
						name:       "p2",
						dependency: Strong,
					},
				},
			}
			result := flow.Execute(context.Background(), testReq{})

			So(result.Success(), ShouldBeTrue)
			So(result.HasSkippedErrors(), ShouldBeTrue)
			So(result.SkippedErrors[0].Err.Error(), ShouldContainSubstring, "panic")
		})

		PatchConvey("MonitorDisabled 不调用 Monitor", func() {
			mon := &testMonitor{}
			MockValue(&defaultMonitorInstance).To(mon)

			flow := &Flow[testReq, testResp]{
				Name: "no-monitor",
				Processors: []Processor[testReq, testResp]{
					&mockProcessor[testReq, testResp]{name: "p1", dependency: Strong},
				},
			}
			result := flow.Execute(context.Background(), testReq{})

			So(result.Success(), ShouldBeTrue)
			// DisableMonitor=true 时 Monitor 不被调用
			So(len(mon.calls), ShouldEqual, 0)
		})
	})
}

// ==================== Monitor 集成测试 ====================

func TestFlow_Execute_Monitor(t *testing.T) {
	PatchConvey("TestFlow_Execute_Monitor", t, func() {
		Mock(GetConfig).Return(&Config{DisableMonitor: false}).Build()
		mon := &testMonitor{}
		MockValue(&defaultMonitorInstance).To(mon)

		PatchConvey("全部成功调用验证", func() {
			flow := &Flow[testReq, testResp]{
				Name: "with-monitor",
				Processors: []Processor[testReq, testResp]{
					&mockProcessor[testReq, testResp]{name: "p1", dependency: Strong},
					&mockProcessor[testReq, testResp]{name: "p2", dependency: Weak},
				},
			}
			result := flow.Execute(context.Background(), testReq{})

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

		PatchConvey("强依赖失败调用验证", func() {
			flow := &Flow[testReq, testResp]{
				Name: "monitor-fail",
				Processors: []Processor[testReq, testResp]{
					&mockProcessor[testReq, testResp]{
						name:       "p1",
						dependency: Strong,
						rollbackFn: func(ctx context.Context, data *FlowData[testReq, testResp]) error { return nil },
					},
					&mockProcessor[testReq, testResp]{
						name:       "p2",
						dependency: Strong,
						processFn: func(ctx context.Context, data *FlowData[testReq, testResp]) error {
							return errors.New("p2 failed")
						},
					},
				},
			}
			result := flow.Execute(context.Background(), testReq{})

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
			So(flowCalls[0].result.IsRolled(), ShouldBeTrue)
		})

		PatchConvey("弱依赖跳过记录", func() {
			flow := &Flow[testReq, testResp]{
				Name: "monitor-weak",
				Processors: []Processor[testReq, testResp]{
					&mockProcessor[testReq, testResp]{name: "p1", dependency: Strong},
					&mockProcessor[testReq, testResp]{
						name:       "weak1",
						dependency: Weak,
						processFn: func(ctx context.Context, data *FlowData[testReq, testResp]) error {
							return errors.New("weak error")
						},
					},
					&mockProcessor[testReq, testResp]{name: "p3", dependency: Strong},
				},
			}
			result := flow.Execute(context.Background(), testReq{})

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
	})
}

// ==================== New 函数测试 ====================

func TestNew(t *testing.T) {
	PatchConvey("TestNew-函数式构建", t, func() {
		Mock(GetConfig).Return(&Config{DisableMonitor: true}).Build()

		p1 := &mockProcessor[testReq, testResp]{name: "p1", dependency: Strong}
		p2 := &mockProcessor[testReq, testResp]{name: "p2", dependency: Weak}
		flow := New("test-flow", p1, p2)

		So(flow.Name, ShouldEqual, "test-flow")
		So(len(flow.Processors), ShouldEqual, 2)
		// 验证可正常执行
		result := flow.Execute(context.Background(), testReq{})
		So(result.Success(), ShouldBeTrue)
	})
}

// ==================== Config 测试 ====================

func TestGetConfig(t *testing.T) {
	PatchConvey("TestGetConfig", t, func() {
		PatchConvey("UnmarshalConfig 成功", func() {
			c := GetConfig()
			So(c, ShouldNotBeNil)
			So(c.DisableMonitor, ShouldBeFalse)
		})

		PatchConvey("UnmarshalConfig 失败返回默认值", func() {
			Mock(xconfig.UnmarshalConfig).To(func(key string, conf any) error {
				return errors.New("unmarshal failed")
			}).Build()
			c := GetConfig()
			So(c, ShouldNotBeNil)
			So(c.DisableMonitor, ShouldBeFalse)
		})
	})
}

func TestConfigMergeDefault(t *testing.T) {
	PatchConvey("TestConfigMergeDefault", t, func() {
		PatchConvey("nil 输入", func() {
			c := configMergeDefault(nil)
			So(c, ShouldNotBeNil)
			So(c.DisableMonitor, ShouldBeFalse)
		})

		PatchConvey("非 nil 输入", func() {
			input := &Config{DisableMonitor: true}
			c := configMergeDefault(input)
			So(c, ShouldEqual, input)
			So(c.DisableMonitor, ShouldBeTrue)
		})
	})
}
