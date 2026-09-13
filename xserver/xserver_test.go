package xserver

import (
	"errors"
	"os"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/xiaoshicae/xone/v2/xhook"
	"github.com/xiaoshicae/xone/v2/xutil"

	. "github.com/bytedance/mockey"
	. "github.com/smartystreets/goconvey/convey"
)

// ==================== mock servers ====================

type normalServer struct{}

func (d normalServer) Run() error  { return nil }
func (d normalServer) Stop() error { return nil }

type panicRunServer struct{}

func (d panicRunServer) Run() error  { panic("panic run") }
func (d panicRunServer) Stop() error { return nil }

type errRunServer struct{}

func (d errRunServer) Run() error  { return errors.New("err run") }
func (d errRunServer) Stop() error { return nil }

type panicStopServer struct{}

func (d panicStopServer) Run() error  { return nil }
func (d panicStopServer) Stop() error { panic("stop panic") }

type errStopServer struct{}

func (d errStopServer) Run() error  { return nil }
func (d errStopServer) Stop() error { return errors.New("stop err") }

// blockingErrStopServer 阻塞在 Run，Stop 时释放阻塞并返回错误
type blockingErrStopServer struct {
	quit chan struct{}
}

func (s blockingErrStopServer) Run() error {
	<-s.quit
	return nil
}

func (s blockingErrStopServer) Stop() error {
	close(s.quit)
	return errors.New("stop err")
}

// ==================== runner.go ====================

func TestRun(t *testing.T) {
	PatchConvey("TestRun", t, func() {
		Mock(run).Return(nil).Build()
		So(Run(normalServer{}), ShouldBeNil)
	})
}

func TestRunBlocking(t *testing.T) {
	PatchConvey("TestRunBlocking", t, func() {
		Mock(run).Return(nil).Build()
		So(RunBlocking(), ShouldBeNil)
	})
}

func TestR(t *testing.T) {
	PatchConvey("TestR", t, func() {
		Mock(run).Return(nil).Build()
		So(R(), ShouldBeNil)
	})
}

func TestRunInternal(t *testing.T) {
	PatchConvey("TestRunInternal", t, func() {
		PatchConvey("BeforeStartHookFail", func() {
			Mock(xhook.InvokeBeforeStartHook).Return(errors.New("hook failed")).Build()
			err := run(normalServer{})
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldEqual, "hook failed")
		})

		PatchConvey("WithServer-BothErrors", func() {
			Mock(xhook.InvokeBeforeStartHook).Return(nil).Build()
			Mock(runWithServer).Return(errors.New("run err")).Build()
			Mock(xhook.InvokeBeforeStopHook).Return(errors.New("stop err")).Build()
			err := run(normalServer{})
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldEqual, "run err\nstop err")
		})

		PatchConvey("WithServer-AllSuccess", func() {
			Mock(xhook.InvokeBeforeStartHook).Return(nil).Build()
			Mock(runWithServer).Return(nil).Build()
			Mock(xhook.InvokeBeforeStopHook).Return(nil).Build()
			err := run(normalServer{})
			So(err, ShouldBeNil)
		})

		PatchConvey("NilServer", func() {
			Mock(xhook.InvokeBeforeStartHook).Return(nil).Build()
			err := run(nil)
			So(err, ShouldBeNil)
		})
	})
}

func TestRunWithServer(t *testing.T) {
	PatchConvey("TestRunWithServer", t, func() {
		PatchConvey("Panic-NilServer", func() {
			err := runWithServer(nil)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "panic occurred")
		})

		PatchConvey("Panic-UserPanic", func() {
			err := runWithServer(panicRunServer{})
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "panic run")
		})

		PatchConvey("RunError", func() {
			err := runWithServer(errRunServer{})
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "err run")
		})

		PatchConvey("ExitWithNil", func() {
			Mock(xutil.WarnIfEnableDebug).Return().Build()
			err := runWithServer(normalServer{})
			So(err, ShouldBeNil)
		})

		PatchConvey("SignalQuit-StopSuccess", func() {
			// 使用 SIGUSR1 避免干扰测试框架的 SIGINT/SIGTERM 处理
			MockValue(&quitSignals).To([]os.Signal{syscall.SIGUSR1})
			Mock(xutil.InfoIfEnableDebug).Return().Build()

			s := &blockingServer{}
			go func() {
				time.Sleep(50 * time.Millisecond)
				syscall.Kill(syscall.Getpid(), syscall.SIGUSR1)
			}()

			err := runWithServer(s)
			So(err, ShouldBeNil)
		})

		PatchConvey("SignalQuit-StopError", func() {
			MockValue(&quitSignals).To([]os.Signal{syscall.SIGUSR1})
			Mock(xutil.InfoIfEnableDebug).Return().Build()

			go func() {
				time.Sleep(50 * time.Millisecond)
				syscall.Kill(syscall.Getpid(), syscall.SIGUSR1)
			}()

			err := runWithServer(blockingErrStopServer{quit: make(chan struct{})})
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "stop err")
		})
	})
}

func TestSafeInvokeServerStop(t *testing.T) {
	PatchConvey("TestSafeInvokeServerStop", t, func() {
		PatchConvey("Panic-NilServer", func() {
			err := safeInvokeServerStop(nil)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "panic occurred")
		})

		PatchConvey("Panic-UserPanic", func() {
			err := safeInvokeServerStop(panicStopServer{})
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "stop panic")
		})

		PatchConvey("StopError", func() {
			err := safeInvokeServerStop(errStopServer{})
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "stop err")
		})

		PatchConvey("ExitWithNil", func() {
			err := safeInvokeServerStop(normalServer{})
			So(err, ShouldBeNil)
		})
	})
}

// ==================== blocking.go ====================

func TestBlockingServerRunAndStop(t *testing.T) {
	PatchConvey("TestBlockingServerRunAndStop", t, func() {
		s := &blockingServer{}

		done := make(chan error)
		go func() {
			done <- s.Run()
		}()

		time.Sleep(10 * time.Millisecond)

		err := s.Stop()
		So(err, ShouldBeNil)

		select {
		case err := <-done:
			So(err, ShouldBeNil)
		case <-time.After(time.Second):
			t.Fatal("Run did not complete after Stop")
		}

		// 幂等验证
		err = s.Stop()
		So(err, ShouldBeNil)
	})
}

func TestBlockingServerStopBeforeRun(t *testing.T) {
	PatchConvey("TestBlockingServerStopBeforeRun", t, func() {
		s := &blockingServer{}

		err := s.Stop()
		So(err, ShouldBeNil)

		done := make(chan error, 1)
		go func() {
			done <- s.Run()
		}()

		select {
		case err := <-done:
			So(err, ShouldBeNil)
		case <-time.After(time.Second):
			t.Fatal("Run did not complete after Stop")
		}
	})
}

// ==================== 启动失败回滚 / Run 返回后调用 Stop ====================

// stopRecordServer 记录 Stop 是否被调用，以及被调用的次数
type stopRecordServer struct {
	runErr    error
	stopTimes int32
	quit      chan struct{}
}

func (s *stopRecordServer) Run() error {
	if s.quit != nil {
		<-s.quit
	}
	return s.runErr
}

func (s *stopRecordServer) Stop() error {
	atomic.AddInt32(&s.stopTimes, 1)
	if s.quit != nil {
		close(s.quit)
	}
	return nil
}

func TestRunInternal_StartHookFailRollback(t *testing.T) {
	PatchConvey("TestRunInternal-StartHookFailRollback", t, func() {
		PatchConvey("BeforeStart 失败时执行 BeforeStop 回滚", func() {
			// hook 正序执行，失败点之前的模块都已初始化完成，不回滚它们就不会被关闭
			stopCalled := false
			Mock(xhook.InvokeBeforeStartHook).Return(errors.New("hook failed")).Build()
			Mock(xhook.InvokeBeforeStopHook).To(func() error {
				stopCalled = true
				return nil
			}).Build()

			err := run(normalServer{})
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "hook failed")
			So(stopCalled, ShouldBeTrue)
		})

		PatchConvey("回滚本身出错时两个错误都返回", func() {
			Mock(xutil.ErrorIfEnableDebug).Return().Build()
			Mock(xhook.InvokeBeforeStartHook).Return(errors.New("start failed")).Build()
			Mock(xhook.InvokeBeforeStopHook).Return(errors.New("rollback failed")).Build()

			err := run(normalServer{})
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "start failed")
			So(err.Error(), ShouldContainSubstring, "rollback failed")
		})
	})
}

func TestRunWithServer_RunReturnInvokesStop(t *testing.T) {
	PatchConvey("TestRunWithServer-RunReturnInvokesStop", t, func() {
		PatchConvey("Run 正常返回也调用 Stop", func() {
			// Server 接口约定清理逻辑放在 Stop 中，只在收到退出信号时才调用，
			// 会让端口占用等场景下用户的清理逻辑永远不执行
			Mock(xutil.InfoIfEnableDebug).Return().Build()
			s := &stopRecordServer{}
			So(runWithServer(s), ShouldBeNil)
			So(atomic.LoadInt32(&s.stopTimes), ShouldEqual, 1)
		})

		PatchConvey("Run 报错时也调用 Stop", func() {
			s := &stopRecordServer{runErr: errors.New("bind failed")}
			err := runWithServer(s)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "bind failed")
			So(atomic.LoadInt32(&s.stopTimes), ShouldEqual, 1)
		})

		PatchConvey("退出信号触发的 Stop 只执行一次", func() {
			// 信号触发 Stop，Stop 让 Run 返回，Run 返回不应再触发第二次 Stop
			MockValue(&quitSignals).To([]os.Signal{syscall.SIGUSR1})
			Mock(xutil.InfoIfEnableDebug).Return().Build()

			s := &stopRecordServer{quit: make(chan struct{})}
			go func() {
				time.Sleep(50 * time.Millisecond)
				_ = syscall.Kill(syscall.Getpid(), syscall.SIGUSR1)
			}()

			So(runWithServer(s), ShouldBeNil)
			So(atomic.LoadInt32(&s.stopTimes), ShouldEqual, 1)
		})
	})
}

func TestWaitRunExitTimeout(t *testing.T) {
	PatchConvey("TestWaitRunExitTimeout", t, func() {
		origin := getWaitRunExitTimeout()
		defer SetWaitRunExitTimeout(origin)

		PatchConvey("正数生效", func() {
			SetWaitRunExitTimeout(5 * time.Second)
			So(getWaitRunExitTimeout(), ShouldEqual, 5*time.Second)
		})

		PatchConvey("非正数忽略", func() {
			SetWaitRunExitTimeout(7 * time.Second)
			SetWaitRunExitTimeout(0)
			SetWaitRunExitTimeout(-1)
			So(getWaitRunExitTimeout(), ShouldEqual, 7*time.Second)
		})
	})
}
