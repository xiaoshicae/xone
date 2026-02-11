package xserver

import (
	"errors"
	"os"
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
