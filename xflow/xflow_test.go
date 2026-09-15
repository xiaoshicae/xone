package xflow

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/xiaoshicae/xone/v3/xconfig"
	"github.com/xiaoshicae/xone/v3/xutil"

	. "github.com/bytedance/mockey"
	. "github.com/smartystreets/goconvey/convey"
)

// ==================== 测试夹具 ====================

// orderData 模拟业务共享数据：入参、出参与中间数据都是普通字段
type orderData struct {
	UserID int
	Amount int
	Steps  []string
}

// testProcessor 可编排行为的处理器
type testProcessor struct {
	name     string
	dep      Dependency
	process  func(context.Context, *orderData) error
	rollback func(context.Context, *orderData) error
}

func (p *testProcessor) Name() string           { return p.name }
func (p *testProcessor) Dependency() Dependency { return p.dep }

func (p *testProcessor) Process(ctx context.Context, d *orderData) error {
	if p.process != nil {
		return p.process(ctx, d)
	}
	d.Steps = append(d.Steps, "process:"+p.name)
	return nil
}

func (p *testProcessor) Rollback(ctx context.Context, d *orderData) error {
	if p.rollback != nil {
		return p.rollback(ctx, d)
	}
	d.Steps = append(d.Steps, "rollback:"+p.name)
	return nil
}

// ok 构造一个正常执行的强依赖处理器
func ok(name string) *testProcessor { return &testProcessor{name: name, dep: Strong} }

// failing 构造一个执行失败的处理器
func failing(name string, dep Dependency, err error) *testProcessor {
	return &testProcessor{name: name, dep: dep, process: func(context.Context, *orderData) error { return err }}
}

// resetConfig 恢复默认运行时配置
func resetConfig() { applyConfig(configMergeDefault(nil)) }

// ==================== processor.go ====================

func TestDependencyString(t *testing.T) {
	PatchConvey("TestDependencyString", t, func() {
		So(Strong.String(), ShouldEqual, "Strong")
		So(Weak.String(), ShouldEqual, "Weak")
		So(Dependency(99).String(), ShouldEqual, "Unknown")
	})
}

// ==================== flow.go ====================

func TestNew(t *testing.T) {
	PatchConvey("TestNew", t, func() {
		PatchConvey("保存名称与处理器顺序", func() {
			a, b := ok("a"), ok("b")
			f := New("order", a, b)
			So(f.Name(), ShouldEqual, "order")
			So(f.processors, ShouldResemble, []Processor[*orderData]{a, b})
		})

		PatchConvey("无处理器也能构建", func() {
			f := New[*orderData]("empty")
			So(f.Name(), ShouldEqual, "empty")
			So(f.processors, ShouldBeEmpty)
		})

		PatchConvey("nil 处理器在构建期就 panic，而不是留到执行时空指针", func() {
			So(func() { New[*orderData]("bad", nil) }, ShouldPanicWith,
				"XOne xflow New failed, err=[processor at index 0 is nil, flow=[bad]]")
			So(func() { New("bad", ok("a"), nil) }, ShouldPanicWith,
				"XOne xflow New failed, err=[processor at index 1 is nil, flow=[bad]]")
		})
	})
}

func TestExecuteSuccess(t *testing.T) {
	PatchConvey("TestExecuteSuccess", t, func() {
		resetConfig()

		PatchConvey("按顺序执行，data 贯穿全程", func() {
			d := &orderData{UserID: 7}
			r := New("order",
				&testProcessor{name: "扣券", dep: Strong, process: func(_ context.Context, d *orderData) error {
					d.Amount -= 10
					d.Steps = append(d.Steps, "扣券")
					return nil
				}},
				&testProcessor{name: "扣款", dep: Strong, process: func(_ context.Context, d *orderData) error {
					d.Amount -= 90
					d.Steps = append(d.Steps, "扣款")
					return nil
				}},
			).Execute(context.Background(), d)

			So(r.Success(), ShouldBeTrue)
			So(r.IsRolled(), ShouldBeFalse)
			So(d.Steps, ShouldResemble, []string{"扣券", "扣款"})
			So(d.Amount, ShouldEqual, -100)
			So(d.UserID, ShouldEqual, 7)
		})

		PatchConvey("ctx 为 nil 时回落到 Background", func() {
			d := &orderData{}
			//nolint:staticcheck // 显式验证 nil ctx 的兜底
			r := New("f", ok("a")).Execute(nil, d)
			So(r.Success(), ShouldBeTrue)
			So(d.Steps, ShouldResemble, []string{"process:a"})
		})
	})
}

func TestExecuteStrongFailure(t *testing.T) {
	PatchConvey("TestExecuteStrongFailure", t, func() {
		resetConfig()
		boom := errors.New("余额不足")

		PatchConvey("中断流程并逆序回滚已执行的处理器", func() {
			d := &orderData{}
			r := New("order", ok("扣券"), ok("扣库存"), failing("扣款", Strong, boom), ok("发通知")).
				Execute(context.Background(), d)

			So(r.Success(), ShouldBeFalse)
			So(r.IsRolled(), ShouldBeTrue)
			So(errors.Is(r.Err, boom), ShouldBeTrue)

			var se *StepError
			So(errors.As(r.Err, &se), ShouldBeTrue)
			So(se.ProcessorName, ShouldEqual, "扣款")
			So(se.Dependency, ShouldEqual, Strong)

			// 失败处理器之后的不执行，之前的逆序回滚
			So(d.Steps, ShouldResemble, []string{
				"process:扣券", "process:扣库存", "rollback:扣库存", "rollback:扣券",
			})
		})

		PatchConvey("流程失败时，此前写入 data 的内容依然保留", func() {
			d := &orderData{}
			New("order",
				&testProcessor{name: "填充", dep: Strong, process: func(_ context.Context, d *orderData) error {
					d.Amount = 42
					return nil
				}},
				failing("失败", Strong, boom),
			).Execute(context.Background(), d)

			So(d.Amount, ShouldEqual, 42)
		})

		PatchConvey("回滚自身失败时汇总到 RollbackErrors，且不中断其余回滚", func() {
			rollbackErr := errors.New("退券失败")
			bad := &testProcessor{name: "扣券", dep: Strong,
				rollback: func(context.Context, *orderData) error { return rollbackErr }}
			d := &orderData{}
			r := New("order", bad, ok("扣库存"), failing("扣款", Strong, boom)).Execute(context.Background(), d)

			So(r.HasRollbackErrors(), ShouldBeTrue)
			So(r.RollbackErrors, ShouldHaveLength, 1)
			So(errors.Is(r.RollbackErrors[0], rollbackErr), ShouldBeTrue)
			// 扣库存 的回滚仍然执行了
			So(d.Steps, ShouldContain, "rollback:扣库存")
		})
	})
}

func TestExecuteWeakFailure(t *testing.T) {
	PatchConvey("TestExecuteWeakFailure", t, func() {
		resetConfig()
		soft := errors.New("通知发送失败")

		PatchConvey("弱依赖失败不中断流程，记入 SkippedErrors", func() {
			d := &orderData{}
			r := New("order", ok("扣券"), failing("发通知", Weak, soft), ok("落库")).
				Execute(context.Background(), d)

			So(r.Success(), ShouldBeTrue)
			So(r.IsRolled(), ShouldBeFalse)
			So(r.HasSkippedErrors(), ShouldBeTrue)
			So(r.SkippedErrors, ShouldHaveLength, 1)
			So(r.SkippedErrors[0].ProcessorName, ShouldEqual, "发通知")
			So(r.SkippedErrors[0].Dependency, ShouldEqual, Weak)
			So(d.Steps, ShouldResemble, []string{"process:扣券", "process:落库"})
		})

		PatchConvey("失败的弱依赖也纳入回滚范围", func() {
			d := &orderData{}
			New("order", ok("扣券"), failing("发通知", Weak, soft), failing("扣款", Strong, errors.New("boom"))).
				Execute(context.Background(), d)

			So(d.Steps, ShouldContain, "rollback:发通知")
			So(d.Steps, ShouldContain, "rollback:扣券")
		})
	})
}

func TestExecutePanicRecovery(t *testing.T) {
	PatchConvey("TestExecutePanicRecovery", t, func() {
		resetConfig()

		PatchConvey("Process panic 被捕获并转为错误", func() {
			p := &testProcessor{name: "炸", dep: Strong,
				process: func(context.Context, *orderData) error { panic("process 炸了") }}
			r := New("f", p).Execute(context.Background(), &orderData{})

			So(r.Success(), ShouldBeFalse)
			So(r.Err.Error(), ShouldContainSubstring, "panic occurred, process 炸了")
		})

		PatchConvey("Rollback panic 被捕获并记入 RollbackErrors", func() {
			p := &testProcessor{name: "炸", dep: Strong,
				rollback: func(context.Context, *orderData) error { panic("rollback 炸了") }}
			r := New("f", p, failing("失败", Strong, errors.New("boom"))).Execute(context.Background(), &orderData{})

			So(r.RollbackErrors, ShouldHaveLength, 1)
			So(r.RollbackErrors[0].Error(), ShouldContainSubstring, "panic occurred, rollback 炸了")
		})
	})
}

// ==================== context 语义 ====================

func TestExecuteRespectsCancellation(t *testing.T) {
	PatchConvey("TestExecuteRespectsCancellation", t, func() {
		resetConfig()

		PatchConvey("ctx 取消后不再启动新的处理器", func() {
			d := &orderData{}
			ctx, cancel := context.WithCancel(context.Background())
			cancel()

			r := New("f", ok("a"), ok("b"), ok("c")).Execute(ctx, d)

			So(r.Success(), ShouldBeFalse)
			So(errors.Is(r.Err, context.Canceled), ShouldBeTrue)
			So(r.Err.Error(), ShouldContainSubstring, "canceled before processor=[a]")
			So(d.Steps, ShouldBeEmpty)
		})

		PatchConvey("执行途中被取消时，已完成的部分仍会回滚", func() {
			d := &orderData{}
			ctx, cancel := context.WithCancel(context.Background())

			first := &testProcessor{name: "扣券", dep: Strong, process: func(_ context.Context, d *orderData) error {
				d.Steps = append(d.Steps, "process:扣券")
				cancel() // 第一个处理器执行完后调用方放弃
				return nil
			}}

			r := New("f", first, ok("扣款")).Execute(ctx, d)

			So(r.IsRolled(), ShouldBeTrue)
			So(errors.Is(r.Err, context.Canceled), ShouldBeTrue)
			So(d.Steps, ShouldResemble, []string{"process:扣券", "rollback:扣券"})
		})
	})
}

func TestRollbackSurvivesDeadContext(t *testing.T) {
	PatchConvey("TestRollbackSurvivesDeadContext", t, func() {
		resetConfig()

		PatchConvey("原 ctx 已超时，补偿逻辑依然能执行", func() {
			var compensated []string
			mk := func(name string) *testProcessor {
				return &testProcessor{name: name, dep: Strong,
					rollback: func(ctx context.Context, _ *orderData) error {
						// 模拟真实补偿调用：尊重传入的 ctx
						if err := ctx.Err(); err != nil {
							return fmt.Errorf("补偿调用被拒: %w", err)
						}
						compensated = append(compensated, name)
						return nil
					}}
			}
			// 扣款耗时超过调用方预算：跑完时原 ctx 已经超时，随后触发回滚
			slowFail := &testProcessor{name: "扣款", dep: Strong,
				process: func(context.Context, *orderData) error {
					time.Sleep(30 * time.Millisecond)
					return errors.New("余额不足")
				}}

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
			defer cancel()

			r := New("order", mk("扣券"), mk("扣库存"), slowFail).Execute(ctx, &orderData{})

			So(ctx.Err(), ShouldNotBeNil) // 确认原 ctx 确实已经死了
			So(r.IsRolled(), ShouldBeTrue)
			So(r.HasRollbackErrors(), ShouldBeFalse)
			So(compensated, ShouldResemble, []string{"扣库存", "扣券"})
		})

		PatchConvey("回滚保留原 ctx 上的 value", func() {
			type ctxKey struct{}
			var got any

			ctx, cancel := context.WithCancel(context.WithValue(context.Background(), ctxKey{}, "tenant-1"))
			defer cancel()

			first := &testProcessor{name: "扣券", dep: Strong,
				process: func(context.Context, *orderData) error {
					cancel() // 执行完第一步后调用方放弃
					return nil
				},
				rollback: func(ctx context.Context, _ *orderData) error {
					got = ctx.Value(ctxKey{})
					return nil
				}}

			r := New("f", first, ok("扣款")).Execute(ctx, &orderData{})
			So(r.IsRolled(), ShouldBeTrue)
			So(got, ShouldEqual, "tenant-1")
		})

		PatchConvey("回滚预算耗尽时，未补偿的处理器逐个记入 RollbackErrors", func() {
			applyConfig(configMergeDefault(&Config{RollbackTimeout: "30ms"}))
			defer resetConfig()

			slow := &testProcessor{name: "慢补偿", dep: Strong,
				rollback: func(context.Context, *orderData) error {
					time.Sleep(60 * time.Millisecond)
					return nil
				}}

			r := New("order", ok("扣券"), slow, failing("扣款", Strong, errors.New("boom"))).
				Execute(context.Background(), &orderData{})

			So(r.RollbackErrors, ShouldHaveLength, 1)
			So(r.RollbackErrors[0].ProcessorName, ShouldEqual, "扣券")
			So(r.RollbackErrors[0].Error(), ShouldContainSubstring, "rollback budget exhausted")
			So(errors.Is(r.RollbackErrors[0], context.DeadlineExceeded), ShouldBeTrue)
		})
	})
}

// ==================== 并发 ====================

func TestExecuteConcurrent(t *testing.T) {
	PatchConvey("TestExecuteConcurrent", t, func() {
		resetConfig()
		f := New("f", &testProcessor{name: "double", dep: Strong,
			process: func(_ context.Context, d *orderData) error {
				d.Amount = d.UserID * 2
				return nil
			}})

		var wg sync.WaitGroup
		results := make([]int, 50)
		for i := 0; i < 50; i++ {
			wg.Add(1)
			go func(n int) {
				defer wg.Done()
				d := &orderData{UserID: n}
				f.Execute(context.Background(), d)
				results[n] = d.Amount
			}(i)
		}
		wg.Wait()

		for i, got := range results {
			So(got, ShouldEqual, i*2)
		}
	})
}

// ==================== result.go ====================

func TestStepError(t *testing.T) {
	PatchConvey("TestStepError", t, func() {
		inner := errors.New("boom")
		se := &StepError{ProcessorName: "扣款", Dependency: Strong, Err: inner}
		So(se.Error(), ShouldEqual, "processor=[扣款], dependency=[Strong], err=[boom]")
		So(errors.Is(se, inner), ShouldBeTrue)
		So(se.Unwrap(), ShouldEqual, inner)
	})
}

func TestExecuteResultString(t *testing.T) {
	PatchConvey("TestExecuteResultString", t, func() {
		PatchConvey("成功时给出明确描述，而不是空串", func() {
			r := &ExecuteResult{}
			So(r.String(), ShouldEqual, "flow succeeded")
			So(fmt.Sprint(r), ShouldEqual, "flow succeeded")
		})

		PatchConvey("成功但有弱依赖错误时带上数量", func() {
			r := &ExecuteResult{SkippedErrors: []*StepError{{ProcessorName: "a"}}}
			So(r.String(), ShouldEqual, "flow succeeded, skipped errors=[1]")
		})

		PatchConvey("失败时描述回滚情况", func() {
			r := &ExecuteResult{Err: errors.New("boom")}
			So(r.String(), ShouldEqual, "flow failed: boom")

			r.Rolled = true
			So(r.String(), ShouldEqual, "flow failed: boom, rolled back")

			r.RollbackErrors = []*StepError{{ProcessorName: "a"}, {ProcessorName: "b"}}
			So(r.String(), ShouldEqual, "flow failed: boom, rolled back, rollback errors=[2]")
		})
	})
}

// ==================== monitor.go ====================

// recordingMonitor 记录收到的事件
type recordingMonitor struct {
	mu       sync.Mutex
	process  []string
	rollback []string
	flow     []string
}

func (m *recordingMonitor) OnProcessDone(_ context.Context, e *StepEvent) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.process = append(m.process, e.FlowName+"/"+e.ProcessorName)
}

func (m *recordingMonitor) OnRollbackDone(_ context.Context, e *StepEvent) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rollback = append(m.rollback, e.FlowName+"/"+e.ProcessorName)
}

func (m *recordingMonitor) OnFlowDone(_ context.Context, e *FlowEvent) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.flow = append(m.flow, fmt.Sprintf("%s/%t", e.FlowName, e.Result.Success()))
}

// panicMonitor 每个回调都 panic
type panicMonitor struct{}

func (panicMonitor) OnProcessDone(context.Context, *StepEvent)  { panic("process 监控炸了") }
func (panicMonitor) OnRollbackDone(context.Context, *StepEvent) { panic("rollback 监控炸了") }
func (panicMonitor) OnFlowDone(context.Context, *FlowEvent)     { panic("flow 监控炸了") }

func TestMonitor(t *testing.T) {
	PatchConvey("TestMonitor", t, func() {
		resetConfig()
		old := GetDefaultMonitor()
		defer SetDefaultMonitor(old)

		PatchConvey("收到 process / rollback / flow 三类事件", func() {
			m := &recordingMonitor{}
			SetDefaultMonitor(m)

			New("order", ok("扣券"), failing("扣款", Strong, errors.New("boom"))).
				Execute(context.Background(), &orderData{})

			So(m.process, ShouldResemble, []string{"order/扣券", "order/扣款"})
			So(m.rollback, ShouldResemble, []string{"order/扣券"})
			So(m.flow, ShouldResemble, []string{"order/false"})
		})

		PatchConvey("监控实现 panic 不影响流程", func() {
			SetDefaultMonitor(panicMonitor{})
			d := &orderData{}
			var r *ExecuteResult
			So(func() {
				r = New("f", ok("a"), failing("b", Strong, errors.New("boom"))).Execute(context.Background(), d)
			}, ShouldNotPanic)

			So(r.IsRolled(), ShouldBeTrue)
			So(d.Steps, ShouldResemble, []string{"process:a", "rollback:a"})
		})

		PatchConvey("SetDefaultMonitor(nil) 关闭监控", func() {
			SetDefaultMonitor(nil)
			So(GetDefaultMonitor(), ShouldBeNil)
			So(resolveMonitor(), ShouldBeNil)
			r := New("f", ok("a")).Execute(context.Background(), &orderData{})
			So(r.Success(), ShouldBeTrue)
		})

		PatchConvey("EnableMonitor=false 时不投递事件", func() {
			m := &recordingMonitor{}
			SetDefaultMonitor(m)
			applyConfig(configMergeDefault(&Config{EnableMonitor: xutil.ToPtr(false)}))
			defer resetConfig()

			New("f", ok("a")).Execute(context.Background(), &orderData{})
			So(m.process, ShouldBeEmpty)
			So(m.flow, ShouldBeEmpty)
		})
	})
}

func TestDefaultMonitor(t *testing.T) {
	PatchConvey("TestDefaultMonitor", t, func() {
		d := &defaultMonitor{}
		ctx := context.Background()

		PatchConvey("成功与失败各走一条分支", func() {
			So(func() {
				d.OnProcessDone(ctx, &StepEvent{FlowName: "f", ProcessorName: "a"})
				d.OnProcessDone(ctx, &StepEvent{FlowName: "f", ProcessorName: "a", Err: errors.New("boom")})
				d.OnRollbackDone(ctx, &StepEvent{FlowName: "f", ProcessorName: "a"})
				d.OnRollbackDone(ctx, &StepEvent{FlowName: "f", ProcessorName: "a", Err: errors.New("boom")})
				d.OnFlowDone(ctx, &FlowEvent{FlowName: "f", Result: &ExecuteResult{}})
				d.OnFlowDone(ctx, &FlowEvent{FlowName: "f", Result: &ExecuteResult{Err: errors.New("boom")}})
			}, ShouldNotPanic)
		})
	})
}

// ==================== config.go / xflow_init.go ====================

func TestConfigMergeDefault(t *testing.T) {
	PatchConvey("TestConfigMergeDefault", t, func() {
		PatchConvey("nil 入参给出全套默认值", func() {
			c := configMergeDefault(nil)
			So(*c.EnableMonitor, ShouldBeTrue)
			So(c.RollbackTimeout, ShouldEqual, defaultRollbackTimeoutStr)
		})

		PatchConvey("显式关闭监控不被覆盖", func() {
			c := configMergeDefault(&Config{EnableMonitor: xutil.ToPtr(false)})
			So(*c.EnableMonitor, ShouldBeFalse)
		})

		PatchConvey("合法的回滚超时保留", func() {
			c := configMergeDefault(&Config{RollbackTimeout: "1m"})
			So(c.RollbackTimeout, ShouldEqual, "1m")
		})

		PatchConvey("非法回滚超时回落默认值并告警", func() {
			var warned string
			Mock(xutil.WarnIfEnableDebug).To(func(msg string, args ...any) { warned = msg }).Build()
			c := configMergeDefault(&Config{RollbackTimeout: "not-a-duration"})
			So(c.RollbackTimeout, ShouldEqual, defaultRollbackTimeoutStr)
			So(warned, ShouldContainSubstring, "RollbackTimeout is invalid")
		})

		PatchConvey("未配置时不告警", func() {
			var warned string
			Mock(xutil.WarnIfEnableDebug).To(func(msg string, args ...any) { warned = msg }).Build()
			configMergeDefault(&Config{})
			So(warned, ShouldBeEmpty)
		})
	})
}

func TestInitXFlow(t *testing.T) {
	PatchConvey("TestInitXFlow", t, func() {
		defer resetConfig()

		PatchConvey("读取配置并写入运行时变量", func() {
			Mock(xconfig.UnmarshalConfig).To(func(key string, conf any) error {
				So(key, ShouldEqual, XFlowConfigKey)
				c := conf.(*Config)
				c.EnableMonitor = xutil.ToPtr(false)
				c.RollbackTimeout = "5s"
				return nil
			}).Build()

			So(initXFlow(), ShouldBeNil)
			So(monitorEnabled.Load(), ShouldBeFalse)
			So(rollbackTimeout(), ShouldEqual, 5*time.Second)
		})

		PatchConvey("配置读取失败时返回 xerror", func() {
			Mock(xconfig.UnmarshalConfig).Return(errors.New("bad config")).Build()
			err := initXFlow()
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "getConfig failed")
			So(err.Error(), ShouldContainSubstring, "bad config")
		})

		PatchConvey("无配置时使用默认值", func() {
			Mock(xconfig.UnmarshalConfig).Return(nil).Build()
			So(initXFlow(), ShouldBeNil)
			So(monitorEnabled.Load(), ShouldBeTrue)
			So(rollbackTimeout(), ShouldEqual, 30*time.Second)
		})
	})
}
